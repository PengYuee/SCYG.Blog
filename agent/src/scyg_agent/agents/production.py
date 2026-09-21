"""Production LangChain AgentRunner composition and execution boundary."""

from __future__ import annotations

import json
from collections.abc import Mapping
from dataclasses import dataclass
from typing import TYPE_CHECKING, Protocol, cast, final, override

from langchain.agents import create_agent  # pyright: ignore[reportUnknownVariableType]
from langchain_openai import ChatOpenAI

from scyg_agent.adapters.database.run_request_source import PostgreSQLRunInputSource
from scyg_agent.config import ApplicationSettings, ModelProtocol
from scyg_agent.domain.runs.input import MissingRunInput

from .contracts import (
    AgentFailure,
    CapabilityOutput,
    FailureKind,
    InvalidCapabilityOutputError,
    validate_capability_output,
)
from .input import AgentInputSnapshot, InvalidAgentInputError, decode_agent_input
from .recipes import AgentRecipe, RecipeRegistry, default_recipe_registry
from .runner import AgentFailed, AgentRunner, AgentRunOutcome, AgentSucceeded

if TYPE_CHECKING:
    from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

    from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
    from scyg_agent.domain.ports.command_store import CommandSubmission
    from scyg_agent.domain.runs import Run
    from scyg_agent.domain.runs.input import RunInputSource
    from scyg_agent.domain.runs.repository_values import RunLease


class AgentGraph(Protocol):
    """Expose only the asynchronous invocation surface used by the Runner."""

    async def ainvoke(self, payload: object, config: object) -> object:
        """Execute one graph invocation and return its raw state."""
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

    @override
    async def execute(  # noqa: PLR0911 - closed input, output, and failure branches are explicit.
        self, run: Run, lease: RunLease
    ) -> AgentRunOutcome:
        """Load, validate, invoke, and strictly validate one capability Run."""
        if lease.run_id != run.id:
            return _failure(FailureKind.VALIDATION, "Agent Run 身份无效")
        try:
            persisted = await self._inputs.get_input(run.id)
            if isinstance(persisted, MissingRunInput):
                return _failure(FailureKind.VALIDATION, "Agent Run 输入无效")
            snapshot = decode_agent_input(persisted, self._registry)
            raw = await self._invoke(self._graph_for(snapshot), snapshot, run)
            output = _structured_output(raw, snapshot.recipe)
            checked = validate_capability_output(snapshot.capability, output)
            return AgentSucceeded(snapshot.capability, checked)
        except InvalidAgentInputError:
            return _failure(FailureKind.VALIDATION, "Agent Run 输入无效")
        except (InvalidCapabilityOutputError, TypeError):
            return _failure(FailureKind.RESULT_VALIDATION, "Agent 结果无效")
        except (TimeoutError, OSError):
            return _failure(FailureKind.DEPENDENCY, "Agent 模型调用失败", retryable=True)
        except Exception:  # noqa: BLE001 - framework errors become a stable Agent outcome.
            return _failure(FailureKind.INTERNAL, "Agent 执行失败")

    @override
    async def resume(
        self,
        run: Run,
        command: CommandSubmission,
        lease: RunLease,
    ) -> AgentRunOutcome:
        """Reject resume until a Recipe declares an approval/checkpoint contract."""
        _ = run, command, lease
        return _failure(FailureKind.VALIDATION, "Agent Run 恢复状态无效")

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

    async def _invoke(self, graph: AgentGraph, snapshot: AgentInputSnapshot, run: Run) -> object:
        """Invoke one graph with a stable thread and a bounded serialized input."""
        payload = json.dumps(
            snapshot.value.model_dump(mode="json"), ensure_ascii=False, sort_keys=True
        )
        message = f"locale={snapshot.locale}\ninput={payload}"
        return await graph.ainvoke(
            {"messages": [{"role": "user", "content": message}]},
            {"configurable": {"thread_id": str(run.id)}},
        )


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
) -> LangChainAgentRunner:
    """Construct all four structured Agents against the shared checkpoint pool."""
    registry = default_recipe_registry()
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
    except BaseException:
        await _close_models(tuple(models.values()))
        raise
    return LangChainAgentRunner(
        AgentRunnerConfig(PostgreSQLRunInputSource(sessions), registry, graphs),
        tuple(models.values()),
    )


def _create_models(settings: ApplicationSettings) -> dict[str, ChatOpenAI]:
    """Create server-owned model tiers with an explicit API protocol."""
    if settings.models is None:
        raise RuntimeError
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
