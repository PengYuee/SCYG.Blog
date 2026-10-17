"""Production LangChain AgentRunner composition and execution boundary."""

from __future__ import annotations

import json
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import dataclass, field
from datetime import UTC, datetime
from hashlib import sha256
from typing import TYPE_CHECKING, Annotated, Generic, Protocol, TypedDict, cast, final, override

from langchain.agents import (
    create_agent,  # pyright: ignore[reportUnknownVariableType]  # TYPE_IGNORE_OK - LangChain 动态工厂由本地 AgentGraph 协议收窄。
)
from langchain.agents.middleware import AgentMiddleware
from langchain.agents.middleware.types import AgentState, ModelRequest, ModelResponse
from langchain_core.messages import SystemMessage
from langchain_openai import ChatOpenAI
from langgraph.errors import GraphRecursionError
from langgraph.graph import END, START, StateGraph
from langgraph.runtime import Runtime  # noqa: TC002 - LangGraph evaluates node runtime hints.
from langgraph.types import Command, Interrupt, StateSnapshot, interrupt
from langgraph.typing import InputT, OutputT, StateT
from sqlalchemy import select

from scyg_agent.adapters.database.run_records import InteractionRecord
from scyg_agent.adapters.database.run_request_source import PostgreSQLRunInputSource
from scyg_agent.config import ApplicationSettings, ModelProtocol, PublicSearchSettings
from scyg_agent.domain.runs import ContentScope
from scyg_agent.domain.runs.input import MissingRunInput
from scyg_agent.domain.runs.repository import LeaseGuard

from .context import AgentRequestContext, Quality
from .contracts import (
    INTERACTION_REQUEST_ADAPTER,
    AgentFailure,
    ApprovalRequest,
    ArticleDraft,
    Capability,
    CapabilityOutput,
    FailureKind,
    InteractionRequest,
    InvalidCapabilityOutputError,
    SearchInput,
    SearchResponse,
    SelectionOption,
    SelectionRequest,
    TextInputRequest,
    WritingInput,
    input_payload,
    validate_capability_output,
    validate_interaction_response,
)
from .execution import PostgreSQLExecutionDispatch
from .input import AgentInputSnapshot, InvalidAgentInputError, decode_agent_input
from .public_policy import PolicyLimitError, PublicAttemptBudget
from .recipes import AgentRecipe, RecipeRegistry, default_recipe_registry
from .runner import (
    AgentFailed,
    AgentRunner,
    AgentRunOutcome,
    AgentSucceeded,
    AgentWaitingForApproval,
)
from .tool_gateway import BlogReadGateway, BlogToolFailureError, ToolAccessDeniedError

if TYPE_CHECKING:
    from langchain_core.runnables import RunnableConfig
    from langgraph.checkpoint.base import BaseCheckpointSaver
    from langgraph.graph.state import CompiledStateGraph
    from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

    from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
    from scyg_agent.domain.ports.command_store import CommandSubmission
    from scyg_agent.domain.runs import Run
    from scyg_agent.domain.runs.input import RunInputSource
    from scyg_agent.domain.runs.repository_values import RunLease

    from .tool_gateway import BlogReadClient


def _decode_interaction(value: object) -> InteractionRequest:
    try:
        return INTERACTION_REQUEST_ADAPTER.validate_python(value)
    except ValueError as error:
        raise InvalidAgentInputError from error


class AgentGraph(Protocol):
    """Expose only the asynchronous invocation surface used by the Runner."""

    async def ainvoke(
        self, payload: object, config: object, *, context: AgentRequestContext
    ) -> object:
        """Execute one graph invocation and return its raw state."""
        ...

    async def aget_state(self, config: object) -> StateSnapshot:
        """Read pending interrupts from the same persisted thread."""
        ...


class GeneratorGraph(Protocol):
    """Describe the generation-only invocation needed by writing preparation."""

    async def ainvoke(
        self, payload: object, config: object, /, *, context: AgentRequestContext
    ) -> object:
        """Generate a structured draft from the prepared request."""
        ...


class WritingReferenceGateway(Protocol):
    """Expose the two authorized reference operations used by preparation."""

    async def search_references(
        self, context: AgentRequestContext, query: str, /
    ) -> list[dict[str, object]]:
        """Search frozen reference metadata."""
        ...

    async def read_reference(
        self, context: AgentRequestContext, article_id: int, /
    ) -> dict[str, object]:
        """Read one authorized article."""
        ...


@dataclass(frozen=True, slots=True)
class SDKAgentGraph(Generic[StateT, InputT, OutputT]):  # noqa: UP046 - 复用 SDK 公开 TypeVar 和默认值以免复制私有状态边界
    """Adapt runner objects to the SDK's actual mapping and command inputs."""

    graph: CompiledStateGraph[StateT, AgentRequestContext, InputT, OutputT]

    async def ainvoke(
        self, payload: object, config: object, *, context: AgentRequestContext
    ) -> object:
        """Validate the runner input shape and invoke the compiled graph."""
        if payload is not None and not isinstance(payload, (dict, Command)):
            raise InvalidAgentInputError
        # SDK v1 output is dynamic; the runner validates its concrete output schema.
        return await self.graph.ainvoke(
            cast("InputT | Command[object] | None", payload),
            cast("RunnableConfig", config),
            context=context,
        )

    async def aget_state(self, config: object) -> StateSnapshot:
        """Read a checkpoint using the runner's generated configuration."""
        return await self.graph.aget_state(cast("RunnableConfig", config))


class AsyncClose(Protocol):
    """Describe the async close operation exposed by model clients."""

    async def close(self) -> None:
        """Close the underlying HTTP client."""
        ...


@dataclass(frozen=True, slots=True)
class RecipeGraph:
    """Bind one server-owned Recipe to one precompiled graph."""

    recipe: AgentRecipe
    graph: AgentGraph


class InvalidAgentGraphCatalogError(ValueError):
    """Reject a graph catalog that is incomplete or ambiguous."""


@dataclass(frozen=True, slots=True)
class AgentRunnerConfig:
    """Carry the immutable input and graph catalog used by a Runner."""

    inputs: RunInputSource
    registry: RecipeRegistry
    graphs: tuple[RecipeGraph, ...]
    sessions: async_sessionmaker[AsyncSession]
    public_search: PublicSearchSettings = field(default_factory=PublicSearchSettings)
    gateway: BlogReadGateway | None = None

    def __post_init__(self) -> None:
        """Reject graph bindings that do not exactly cover registry recipe objects."""
        expected = {(recipe.recipe_id, recipe.version): recipe for recipe in self.registry.recipes}
        actual = {
            (binding.recipe.recipe_id, binding.recipe.version): binding for binding in self.graphs
        }
        if set(actual) != set(expected) or len(actual) != len(self.graphs):
            raise InvalidAgentGraphCatalogError
        if any(actual[key].recipe is not expected[key] for key in expected):
            raise InvalidAgentGraphCatalogError


class ApprovalRejectedError(ValueError):
    """A persisted confirmation rejected the generated draft."""


class WritingApproval(AgentMiddleware[AgentState[object], AgentRequestContext, object]):
    """Checkpoint the complete draft before exposing a one-time confirmation."""

    @override
    async def aafter_agent(
        self, state: AgentState[object], runtime: Runtime[AgentRequestContext]
    ) -> None:
        draft = state.get("structured_response")
        if not isinstance(draft, ArticleDraft):
            raise InvalidCapabilityOutputError
        identity = sha256(f"{runtime.context.run_id}:writing-confirmation".encode()).hexdigest()[
            :32
        ]
        request = ApprovalRequest(
            interaction_id=f"int_{identity}", title=draft.title, outline=draft.outline
        )
        response = cast("object", interrupt(request.model_dump(mode="json")))
        if not isinstance(response, Mapping):
            raise InvalidAgentInputError
        decision = cast("Mapping[str, object]", response).get("decision")
        if decision == "reject":
            raise ApprovalRejectedError
        if decision != "approve":
            raise InvalidAgentInputError


@final
class LangChainAgentRunner(AgentRunner):
    """Execute precompiled structured-output Agents under a Worker lease."""

    def __init__(self, config: AgentRunnerConfig, models: tuple[ChatOpenAI, ...] = ()) -> None:
        """Bind already-constructed graphs without request-path framework construction."""
        self._inputs: RunInputSource = config.inputs
        self._registry: RecipeRegistry = config.registry
        self._graphs: tuple[RecipeGraph, ...] = config.graphs
        self._models: tuple[ChatOpenAI, ...] = models
        self._closed: bool = False
        self._sessions = config.sessions
        self._public_search = config.public_search
        self._gateway = config.gateway

    @override
    async def execute(  # noqa: PLR0911 - closed input, output, and failure branches are explicit.
        self, run: Run, lease: RunLease
    ) -> AgentRunOutcome:
        """Load, validate, invoke, and strictly validate one capability Run."""
        if lease.run_id != run.id:
            return _failure(FailureKind.VALIDATION, "Agent Run 身份无效")
        try:
            return await self._execute_run(run, lease)
        except (PolicyLimitError, GraphRecursionError):
            return _failure(FailureKind.POLICY_LIMIT, "公共搜索执行超出策略限制")
        except ApprovalRejectedError:
            return _failure(FailureKind.APPROVAL_REJECTED, "文章草稿审批被拒绝")
        except InvalidAgentInputError:
            return _failure(FailureKind.VALIDATION, "Agent Run 输入无效")
        except ToolAccessDeniedError:
            return _failure(FailureKind.FORBIDDEN, "Agent 工具访问被拒绝")
        except BlogToolFailureError as error:
            return _failure(
                FailureKind.DEPENDENCY, "Agent Blog 工具调用失败", retryable=error.retryable
            )
        except (InvalidCapabilityOutputError, TypeError):
            return _failure(FailureKind.RESULT_VALIDATION, "Agent 结果无效")
        except (TimeoutError, OSError):
            return _failure(FailureKind.DEPENDENCY, "Agent 模型调用失败", retryable=True)
        except Exception:  # noqa: BLE001, RUF100  # noqa: BROAD_EXCEPT_OK - 框架边界必须把未知第三方异常映射为稳定失败。
            return _failure(FailureKind.INTERNAL, "Agent 执行失败")

    async def _execute_run(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        """Admit the live lease and process the persisted input and graph outcome."""
        execution_guard = PostgreSQLExecutionDispatch(
            self._sessions,
            LeaseGuard(run.id, lease.owner, lease.token, lease.revision, datetime.now(UTC)),
            lambda: datetime.now(UTC),
        )
        await execution_guard.check()
        persisted = await self._inputs.get_input(run.id)
        if isinstance(persisted, MissingRunInput):
            return _failure(FailureKind.VALIDATION, "Agent Run 输入无效")
        snapshot = decode_agent_input(persisted, self._registry)
        raw = await self._invoke(self._graph_for(snapshot), snapshot, run, execution_guard)
        waiting = _approval_interrupt(raw)
        if waiting is not None:
            return AgentWaitingForApproval(waiting)
        output = _structured_output(raw, snapshot.recipe)
        checked = validate_capability_output(snapshot.capability, output)
        return AgentSucceeded(snapshot.capability, checked)

    @override
    async def resume(
        self,
        run: Run,
        command: CommandSubmission,
        lease: RunLease,
    ) -> AgentRunOutcome:
        """Consume the server-persisted reply under the claimed Worker lease."""
        if command.run_id != run.id:
            return _failure(FailureKind.VALIDATION, "Agent Run 恢复身份无效")
        return await self.execute(run, lease)

    async def close(self) -> None:
        """Close model HTTP clients exactly once after Worker drain."""
        if self._closed:
            return
        self._closed = True
        await _close_models(self._models)

    def _graph_for(self, snapshot: AgentInputSnapshot) -> AgentGraph:
        """Resolve the graph by the frozen Recipe identity, never by client input."""
        for binding in self._graphs:
            if binding.recipe is snapshot.recipe:
                return binding.graph
        raise InvalidAgentInputError

    def _public_budget(self, snapshot: AgentInputSnapshot, run: Run) -> PublicAttemptBudget | None:
        """Validate frozen public input before constructing the attempt budget."""
        if run.content_scope is not ContentScope.PUBLIC:
            return None
        deadline = run.public_execution_deadline
        if snapshot.capability is not Capability.SEARCH or deadline is None:
            reason = "public execution context"
            raise PolicyLimitError(reason)
        value = snapshot.value
        if not isinstance(value, SearchInput) or (
            len(value.query) > self._public_search.query_max_chars
            or value.max_results > self._public_search.max_results
        ):
            reason = "public input"
            raise PolicyLimitError(reason)
        return PublicAttemptBudget(self._public_search, deadline)

    async def _invoke(
        self,
        graph: AgentGraph,
        snapshot: AgentInputSnapshot,
        run: Run,
        execution_guard: PostgreSQLExecutionDispatch,
    ) -> object:
        """Invoke one graph with a stable thread and a bounded serialized input."""
        budget = self._public_budget(snapshot, run)
        payload = json.dumps(input_payload(snapshot.value), ensure_ascii=False, sort_keys=True)
        message = f"locale={snapshot.locale}\ninput={payload}"
        context = AgentRequestContext(
            owner_user_id=run.owner_user_id,
            run_id=str(run.id),
            capability=snapshot.recipe.capability,
            recipe_id=snapshot.recipe.recipe_id,
            recipe_version=snapshot.recipe.version,
            locale=snapshot.locale,
            quality=Quality(snapshot.recipe.model_tier),
            execution_guard=execution_guard,
            content_scope=run.content_scope,
            public_budget=budget,
        )
        graph_input: object = {"messages": [{"role": "user", "content": message}]}
        preparation = isinstance(snapshot.value, WritingInput) and (
            snapshot.value.request_requirements or bool(snapshot.value.reference_query.strip())
        )
        if preparation:
            graph_input = {
                "writing_input": input_payload(snapshot.value),
                "locale": snapshot.locale,
            }
        configured: dict[str, object] = {"thread_id": str(run.id)}
        if preparation:
            configured["writing_preparation"] = True
        config: dict[str, object] = {"configurable": configured}
        if context.public_budget is not None:
            context.public_budget.check_deadline()
            config["recursion_limit"] = self._public_search.graph_recursion_limit
        state = await graph.aget_state(config)
        checkpoint_values = cast("object", state.values)
        prior_sources: set[int] = set()
        if context.public_budget is not None and isinstance(checkpoint_values, Mapping):
            prior_sources = _authorized_sources(
                cast("Mapping[str, object]", checkpoint_values),
                context.public_budget.settings.max_read_chars_per_attempt,
            )
        graph_input, waiting = await self._checkpoint_input(graph_input, state, run)
        if waiting is not None:
            return waiting
        await execution_guard.check()
        if prior_sources:
            if self._gateway is None:
                raise InvalidCapabilityOutputError
            await self._gateway.reauthorize_sources(context, prior_sources)
        raw = await graph.ainvoke(graph_input, config, context=context)
        if context.public_budget is not None:
            context.public_budget.check_deadline()
            _validate_public_output(raw, snapshot.recipe, context, self._public_search)
        return raw

    async def _checkpoint_input(
        self, graph_input: object, state: StateSnapshot, run: Run
    ) -> tuple[object, dict[str, object] | None]:
        """Resume only the persisted reply, or continue a reclaimed checkpoint."""
        interrupts = tuple(item for task in state.tasks for item in task.interrupts)
        if interrupts:
            if len(interrupts) != 1:
                raise InvalidAgentInputError
            request = _decode_interaction(cast("object", interrupts[0].value))
            response = await self._resolved_reply(run, request)
            if response is None:
                return graph_input, {"__interrupt__": interrupts}
            return Command(resume={interrupts[0].id: response}), None
        if cast("object", state.values):
            # A reclaimed lease continues its checkpoint without duplicating input.
            return None, None
        return graph_input, None

    async def _resolved_reply(
        self, run: Run, request: InteractionRequest
    ) -> dict[str, object] | None:
        """Read only the matching interaction, preserving absent versus JSON null."""
        async with self._sessions() as session:
            reply = (
                await session.execute(
                    select(InteractionRecord).where(
                        InteractionRecord.run_id == str(run.id),
                        InteractionRecord.interaction_id == request.interaction_id,
                    )
                )
            ).scalar_one_or_none()
            if reply is None or reply.status == "pending":
                return None
            allowed = {
                "confirmation": {"approve", "reject"},
                "selection": {"select"},
                "text_input": {"submit"},
            }
            if (
                reply.status != "resolved"
                or reply.kind != request.kind
                or reply.decision not in allowed[request.kind]
                or reply.request_payload != request.model_dump(mode="json")
            ):
                raise InvalidAgentInputError
            try:
                _ = validate_interaction_response(
                    request,
                    reply.decision,
                    payload_present=reply.payload_present,
                    payload=reply.response_payload,
                )
            except ValueError as error:
                raise InvalidAgentInputError from error
            response: dict[str, object] = {"decision": reply.decision}
            if reply.payload_present:
                response["payload"] = reply.response_payload
            return response


def _approval_interrupt(raw: object) -> InteractionRequest | None:
    """Expose only the declared, bounded approval payload from a graph interrupt."""
    if not isinstance(raw, Mapping):
        return None
    state = cast("Mapping[str, object]", raw)
    interrupts = state.get("__interrupt__")
    if not interrupts:
        return None
    if not isinstance(interrupts, (tuple, list)):
        raise InvalidAgentInputError
    pending = cast("tuple[object, ...] | list[object]", interrupts)
    if len(pending) != 1 or not isinstance(pending[0], Interrupt):
        raise InvalidAgentInputError
    return _decode_interaction(cast("object", pending[0].value))


def _structured_output(raw: object, recipe: AgentRecipe) -> CapabilityOutput:
    """Extract one exact structured response from LangChain graph state."""
    if not isinstance(raw, Mapping):
        raise TypeError
    state = cast("Mapping[str, object]", raw)
    value = state.get("structured_response")
    if type(value) is not recipe.output_schema:
        raise TypeError
    return cast("CapabilityOutput", value)


def _failure(kind: FailureKind, message: str, *, retryable: bool = False) -> AgentFailed:
    """Construct a secret-free stable failure outcome."""
    return AgentFailed(AgentFailure(kind=kind, message=message, retryable=retryable))


def _merge_source_ids(previous: list[int], current: list[int]) -> list[int]:
    return sorted(set(previous) | set(current))


class PublicAgentState(AgentState[object], total=False):
    """Persist only successfully authorized public source identities."""

    authorized_article_ids: Annotated[list[int], _merge_source_ids]


class PublicModelPolicy(AgentMiddleware[AgentState[object], AgentRequestContext, object]):
    """Apply public limits to each request without mutating shared model clients."""

    state_schema: type[AgentState[object]] = PublicAgentState

    @override
    async def abefore_model(
        self, state: AgentState[object], runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object] | None:
        if runtime.context.public_budget is None:
            return None
        ids = (
            _authorized_sources(
                state, runtime.context.public_budget.settings.max_read_chars_per_attempt
            )
            | runtime.context.authorized_article_ids
        )
        return {"authorized_article_ids": sorted(ids)}

    @override
    async def aafter_agent(
        self, state: AgentState[object], runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object] | None:
        return await self.abefore_model(state, runtime)

    def __init__(self, settings: ApplicationSettings) -> None:
        """Borrow immutable process settings without modifying model clients."""
        self.settings: ApplicationSettings = settings

    @override
    async def awrap_model_call(
        self,
        request: ModelRequest[AgentRequestContext],
        handler: Callable[[ModelRequest[AgentRequestContext]], Awaitable[ModelResponse[object]]],
    ) -> ModelResponse[object]:
        context = request.runtime.context
        if context.content_scope is not ContentScope.PUBLIC:
            return await handler(request)
        if context.capability is not Capability.SEARCH or context.public_budget is None:
            reason = "公共搜索策略上下文无效"
            raise PolicyLimitError(reason)
        context.public_budget.check_deadline()
        if not isinstance(request.model, ChatOpenAI):
            reason = "模型不支持公共输出限制"
            raise PolicyLimitError(reason)
        responses = self.settings.models.protocol is ModelProtocol.RESPONSES
        if request.model.use_responses_api is not responses:
            reason = "模型协议与公共策略不匹配"
            raise PolicyLimitError(reason)
        parameters = dict(request.model_settings)
        parameters.pop("max_tokens", None)
        parameters.pop("max_completion_tokens", None)
        parameters.pop("max_output_tokens", None)
        parameters["max_output_tokens" if responses else "max_completion_tokens"] = (
            self.settings.public_search.model_max_output_tokens
        )
        return await handler(
            request.override(
                model_settings=parameters,
                system_message=SystemMessage(
                    content=(
                        "Search only published public Blog articles "
                        "using the registered read tools. "
                        "Never request or disclose drafts, archived, deleted or private content. "
                        "Cite only successfully authorized sources from this run, at most "
                        f"{self.settings.public_search.max_results} articles. "
                        "Never invent evidence. "
                        "Return a concise structured result."
                    )
                ),
            )
        )


_MAX_CANDIDATES = 5
_MIN_SELECTION_CANDIDATES = 2


def _required[T](value: T | None) -> T:
    """Reject an absent checkpoint field before accessing its typed value."""
    if value is None:
        raise InvalidAgentInputError
    return value


def _authorized_sources(state: Mapping[str, object], max_ids: int) -> set[int]:
    """Bound persisted identities before allocating or authorizing checkpoint values."""
    value = state.get("authorized_article_ids", ())
    if not isinstance(value, (list, tuple)):
        raise InvalidCapabilityOutputError
    identities = cast("list[object] | tuple[object, ...]", value)
    if len(identities) > max_ids:
        raise InvalidCapabilityOutputError
    sources: set[int] = set()
    for article_id in identities:
        if type(article_id) is not int or not 1 <= article_id <= 2**63 - 1:
            raise InvalidCapabilityOutputError
        sources.add(article_id)
    return sources


def _validate_public_output(
    raw: object, recipe: AgentRecipe, context: AgentRequestContext, settings: PublicSearchSettings
) -> None:
    """Enforce result bounds and successfully authorized public sources."""
    output = _structured_output(raw, recipe)
    if not isinstance(output, SearchResponse) or (
        len(output.articles) > settings.max_results
        or any(
            article.visibility != "published"
            or article.article_id not in context.authorized_article_ids
            for article in output.articles
        )
    ):
        raise InvalidCapabilityOutputError


class WritingState(TypedDict, total=False):
    """Describe optional channels populated one preparation stage at a time."""

    writing_input: dict[str, object]
    locale: str
    effective_requirements: str
    candidates: list[dict[str, object]]
    selected_ids: tuple[int, ...]
    references: list[dict[str, object]]
    structured_response: ArticleDraft


def _interaction_id(context: AgentRequestContext, stage: str) -> str:
    return "int_" + sha256(f"{context.run_id}:writing-{stage}".encode()).hexdigest()[:32]


def _consume(request: InteractionRequest) -> str | tuple[int, ...] | None:
    response = cast("object", interrupt(request.model_dump(mode="json")))
    if not isinstance(response, Mapping):
        raise InvalidAgentInputError
    values = cast("Mapping[str, object]", response)
    decision = values.get("decision")
    if not isinstance(decision, str):
        raise InvalidAgentInputError
    try:
        return validate_interaction_response(
            request, decision, payload_present="payload" in values, payload=values.get("payload")
        )
    except ValueError as error:
        raise InvalidAgentInputError from error


@dataclass(frozen=True, slots=True)
class _WritingPreparation:
    generator: GeneratorGraph
    gateway: WritingReferenceGateway

    async def requirements(
        self, state: WritingState, runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object]:
        """Resolve required prose before any reference search or generation."""
        value = WritingInput.model_validate(_required(state.get("writing_input")))
        text = value.requirements
        if value.request_requirements:
            text = _consume(
                TextInputRequest(
                    interaction_id=_interaction_id(runtime.context, "requirements"),
                    title="补充写作要求",
                    prompt="请补充读者、重点、风格或篇幅;不必逐项填写。",
                )
            )
            if not isinstance(text, str):
                raise InvalidAgentInputError
        return {"effective_requirements": text}

    async def candidates(
        self, state: WritingState, runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object]:
        """Freeze bounded candidate metadata before asking for a selection."""
        value = WritingInput.model_validate(_required(state.get("writing_input")))
        items = (
            await self.gateway.search_references(runtime.context, value.reference_query.strip())
            if value.reference_query.strip()
            else []
        )
        # Validation freezes metadata only, never any unselected body returned upstream.
        options = [
            SelectionOption.model_validate(
                {key: item[key] for key in ("article_id", "title", "digest", "status")}
            )
            for item in items
        ]
        if len(options) > _MAX_CANDIDATES or len({item.article_id for item in options}) != len(
            options
        ):
            raise InvalidAgentInputError
        return {"candidates": [item.model_dump(mode="json") for item in options]}

    async def selection(
        self, state: WritingState, runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object]:
        """Consume only IDs from the frozen candidate set."""
        options = tuple(
            SelectionOption.model_validate(item) for item in _required(state.get("candidates"))
        )
        selected = tuple(item.article_id for item in options)
        if len(options) >= _MIN_SELECTION_CANDIDATES:
            response = _consume(
                SelectionRequest(
                    interaction_id=_interaction_id(runtime.context, "references"),
                    title="选择写作参考",
                    prompt="只读取所选文章的正文片段;也可不选任何文章继续。",
                    options=options,
                    max_selections=len(options),
                )
            )
            if not isinstance(response, tuple):
                raise InvalidAgentInputError
            selected = response
        return {"selected_ids": selected}

    async def read(
        self, state: WritingState, runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object]:
        """Read only selected bodies and persist bounded fragments."""
        references: list[dict[str, object]] = []
        for article_id in _required(state.get("selected_ids")):
            article = await self.gateway.read_reference(runtime.context, article_id)
            content = article.get("content")
            if article.get("article_id") != article_id or not isinstance(content, str):
                raise InvalidCapabilityOutputError
            references.append(
                {
                    "article_id": article_id,
                    "title": article["title"],
                    "fragment": content[:4000],
                    "is_fragment": True,
                }
            )
        return {"references": references}

    async def generate(
        self, state: WritingState, runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object]:
        """Generate once from persisted requirements and selected fragments."""
        value = WritingInput.model_validate(_required(state.get("writing_input")))
        data = {
            "topic": value.topic,
            "requirements": _required(state.get("effective_requirements")),
            "reference_article_ids": list(value.reference_article_ids),
            "reference_fragments": _required(state.get("references")),
        }
        raw = await self.generator.ainvoke(
            {
                "messages": [
                    {
                        "role": "user",
                        "content": f"locale={_required(state.get('locale'))}\ninput="
                        + json.dumps(data, ensure_ascii=False),
                    }
                ]
            },
            {},
            context=runtime.context,
        )
        if not isinstance(raw, Mapping):
            raise InvalidCapabilityOutputError
        draft = cast("Mapping[str, object]", raw).get("structured_response")
        if not isinstance(draft, ArticleDraft):
            raise InvalidCapabilityOutputError
        if value.reference_query.strip() and not set(draft.source_article_ids) <= {
            item["article_id"] for item in _required(state.get("references"))
        }:
            raise InvalidCapabilityOutputError
        return {"structured_response": draft}

    async def confirmation(
        self, state: WritingState, runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object]:
        """Checkpoint the complete draft before consuming confirmation."""
        draft = _required(state.get("structured_response"))
        request = ApprovalRequest(
            interaction_id=_interaction_id(runtime.context, "confirmation"),
            title=draft.title,
            outline=draft.outline,
        )
        response = cast("object", interrupt(request.model_dump(mode="json")))
        if not isinstance(response, Mapping):
            raise InvalidAgentInputError
        decision = cast("Mapping[str, object]", response).get("decision")
        if decision not in {"approve", "reject"}:
            raise InvalidAgentInputError
        if decision == "reject":
            raise ApprovalRejectedError
        return {}


def prepared_writing_graph(
    generator: GeneratorGraph,
    gateway: WritingReferenceGateway,
    saver: BaseCheckpointSaver[int] | BaseCheckpointSaver[float] | BaseCheckpointSaver[str],
) -> AgentGraph:
    """Persist every external read before a separate replay-safe interrupt node."""
    preparation = _WritingPreparation(generator, gateway)
    builder = StateGraph(WritingState, context_schema=AgentRequestContext)
    nodes = [
        ("requirements", preparation.requirements),
        ("prepare_references", preparation.candidates),
        ("select_references", preparation.selection),
        ("read_references", preparation.read),
        ("generate", preparation.generate),
        ("confirmation", preparation.confirmation),
    ]
    previous = START
    for name, node in nodes:
        _ = builder.add_node(name, node)
        _ = builder.add_edge(previous, name)
        previous = name
    _ = builder.add_edge(previous, END)
    return SDKAgentGraph(builder.compile(checkpointer=saver))


@dataclass(frozen=True, slots=True)
class WritingGraph:
    """Keep original default-writing checkpoints while routing opted-in preparation."""

    legacy: AgentGraph
    prepared: AgentGraph

    def _select(self, config: object) -> AgentGraph:
        values = cast("dict[str, object]", config)
        configured = cast("dict[str, object]", values["configurable"])
        return self.prepared if configured.get("writing_preparation") else self.legacy

    async def ainvoke(
        self, payload: object, config: object, *, context: AgentRequestContext
    ) -> object:
        """Invoke the selected graph in its existing checkpoint namespace."""
        return await self._select(config).ainvoke(payload, config, context=context)

    async def aget_state(self, config: object) -> StateSnapshot:
        """Read interrupts from the selected persisted graph."""
        return await self._select(config).aget_state(config)


async def create_langchain_agent_runner(
    settings: ApplicationSettings,
    sessions: async_sessionmaker[AsyncSession],
    checkpoints: CheckpointStore,
    blog: BlogReadClient,
) -> LangChainAgentRunner:
    """Construct all four structured Agents against the shared checkpoint pool."""
    registry = default_recipe_registry()
    gateway = BlogReadGateway(blog, registry)
    models = _create_models(settings)
    try:
        async with checkpoints.saver() as saver:
            graphs_list: list[RecipeGraph] = []
            for recipe in registry.recipes:
                middleware = [gateway.middleware, PublicModelPolicy(settings)]
                graph: AgentGraph = SDKAgentGraph(
                    create_agent(
                        models[recipe.model_tier],
                        tools=[]
                        if recipe.capability is Capability.WRITE
                        else gateway.tools_for(recipe),
                        middleware=middleware
                        + ([WritingApproval()] if recipe.capability is Capability.WRITE else []),
                        context_schema=AgentRequestContext,
                        system_prompt=recipe.prompt,
                        response_format=recipe.output_schema,
                        checkpointer=saver,
                        name=f"scyg-{recipe.recipe_id.value}",
                    ),
                )
                if recipe.capability is Capability.WRITE:
                    generator = SDKAgentGraph(
                        create_agent(
                            models[recipe.model_tier],
                            tools=[],
                            middleware=middleware,
                            context_schema=AgentRequestContext,
                            system_prompt=recipe.prompt,
                            response_format=recipe.output_schema,
                        ),
                    )
                    graph = WritingGraph(graph, prepared_writing_graph(generator, gateway, saver))
                graphs_list.append(RecipeGraph(recipe, graph))
            graphs = tuple(graphs_list)
    except BaseException:  # noqa: RUF100  # noqa: BROAD_EXCEPT_OK - 构造失败时必须释放模型并原样传播取消。
        await _close_models(tuple(models.values()))
        raise
    return LangChainAgentRunner(
        AgentRunnerConfig(
            PostgreSQLRunInputSource(sessions),
            registry,
            graphs,
            sessions,
            settings.public_search,
            gateway,
        ),
        tuple(models.values()),
    )


def _create_models(settings: ApplicationSettings) -> dict[str, ChatOpenAI]:
    """Create server-owned model tiers with an explicit API protocol."""
    use_responses = settings.models.protocol is ModelProtocol.RESPONSES
    return {
        name: ChatOpenAI(
            model=tier.model,
            base_url=str(tier.base_url),
            api_key=tier.api_key,
            timeout=tier.timeout_seconds,
            max_retries=0,
            use_responses_api=use_responses,
        )
        for name, tier in (
            ("fast", settings.models.fast),
            ("standard", settings.models.standard),
            ("strong", settings.models.strong),
        )
    }


async def _close_models(models: tuple[ChatOpenAI, ...]) -> None:
    """Close partially constructed model clients after graph construction fails."""
    for model in models:
        client = cast("AsyncClose", model.root_async_client)
        await client.close()
