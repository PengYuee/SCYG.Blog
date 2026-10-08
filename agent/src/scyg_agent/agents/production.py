"""Production LangChain AgentRunner composition and execution boundary."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from hashlib import sha256
from typing import TYPE_CHECKING, Protocol, cast, final, override

from langchain.agents import (
    create_agent,  # pyright: ignore[reportUnknownVariableType]  # TYPE_IGNORE_OK - LangChain 动态工厂由本地 AgentGraph 协议收窄。
)
from langchain.agents.middleware import AgentMiddleware
from langchain.agents.middleware.types import AgentState
from langchain_openai import ChatOpenAI
from langgraph.types import Command, Interrupt, StateSnapshot, interrupt
from sqlalchemy import select

from scyg_agent.adapters.database.run_records import InteractionRecord
from scyg_agent.adapters.database.run_request_source import PostgreSQLRunInputSource
from scyg_agent.config import ApplicationSettings, ModelProtocol
from scyg_agent.domain.runs.input import MissingRunInput
from scyg_agent.domain.runs.repository import LeaseGuard

from .context import AgentRequestContext, Quality
from .contracts import (
    AgentFailure,
    ApprovalRequest,
    ArticleDraft,
    Capability,
    CapabilityOutput,
    FailureKind,
    InvalidCapabilityOutputError,
    validate_capability_output,
)
from .execution import PostgreSQLExecutionDispatch
from .input import AgentInputSnapshot, InvalidAgentInputError, decode_agent_input
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
    from langgraph.runtime import Runtime
    from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

    from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
    from scyg_agent.domain.ports.command_store import CommandSubmission
    from scyg_agent.domain.runs import Run
    from scyg_agent.domain.runs.input import RunInputSource
    from scyg_agent.domain.runs.repository_values import RunLease

    from .tool_gateway import BlogReadClient


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

    @override
    async def execute(  # noqa: PLR0911 - closed input, output, and failure branches are explicit.
        self, run: Run, lease: RunLease
    ) -> AgentRunOutcome:
        """Load, validate, invoke, and strictly validate one capability Run."""
        if lease.run_id != run.id:
            return _failure(FailureKind.VALIDATION, "Agent Run 身份无效")
        try:
            return await self._execute_run(run, lease)
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

    async def _invoke(
        self,
        graph: AgentGraph,
        snapshot: AgentInputSnapshot,
        run: Run,
        execution_guard: PostgreSQLExecutionDispatch,
    ) -> object:
        """Invoke one graph with a stable thread and a bounded serialized input."""
        payload = json.dumps(
            snapshot.value.model_dump(mode="json"), ensure_ascii=False, sort_keys=True
        )
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
        )
        graph_input: object = {"messages": [{"role": "user", "content": message}]}
        config = {"configurable": {"thread_id": str(run.id)}}
        state = await graph.aget_state(config)
        interrupts = tuple(item for task in state.tasks for item in task.interrupts)
        if interrupts:
            if len(interrupts) != 1:
                raise InvalidAgentInputError
            request = ApprovalRequest.model_validate(cast("object", interrupts[0].value))
            response = await self._resolved_reply(run, request)
            if response is None:
                return {"__interrupt__": interrupts}
            graph_input = Command(resume={interrupts[0].id: response})
        elif state.values:
            # A reclaimed lease continues its checkpoint without duplicating input.
            graph_input = None
        await execution_guard.check()
        return await graph.ainvoke(
            graph_input,
            {"configurable": {"thread_id": str(run.id)}},
            context=context,
        )

    async def _resolved_reply(self, run: Run, request: ApprovalRequest) -> dict[str, object] | None:
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
            response: dict[str, object] = {"decision": reply.decision}
            if reply.payload_present:
                response["payload"] = reply.response_payload
            return response


def _approval_interrupt(raw: object) -> ApprovalRequest | None:
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
    return ApprovalRequest.model_validate(cast("object", pending[0].value))


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
            graphs = tuple(
                RecipeGraph(
                    recipe,
                    cast(
                        "AgentGraph",
                        cast(
                            "object",
                            create_agent(
                                models[recipe.model_tier],
                                tools=gateway.tools_for(recipe),
                                middleware=(
                                    [gateway.middleware, WritingApproval()]
                                    if recipe.capability is Capability.WRITE
                                    else [gateway.middleware]
                                ),
                                context_schema=AgentRequestContext,
                                system_prompt=recipe.prompt,
                                response_format=recipe.output_schema,
                                checkpointer=saver,
                                name=f"scyg-{recipe.recipe_id.value}",
                            ),
                        ),
                    ),
                )
                for recipe in registry.recipes
            )
    except BaseException:  # noqa: RUF100  # noqa: BROAD_EXCEPT_OK - 构造失败时必须释放模型并原样传播取消。
        await _close_models(tuple(models.values()))
        raise
    return LangChainAgentRunner(
        AgentRunnerConfig(PostgreSQLRunInputSource(sessions), registry, graphs, sessions),
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
