"""Production writing HITL checkpoints and mandatory lease-backed Agent admission."""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING, Required, TypedDict, cast

import pytest
from langgraph.checkpoint.memory import InMemorySaver
from langgraph.graph import END, START, StateGraph
from langgraph.graph.state import CompiledStateGraph
from langgraph.runtime import Runtime
from langgraph.types import Command, Interrupt, interrupt
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.run_records import InteractionRecord, RunRecord
from scyg_agent.agents import (
    ApprovalRequest,
    ArticleDraft,
    Capability,
    RecipeId,
    WritingInput,
    input_digest,
    input_payload,
)
from scyg_agent.agents.context import AgentRequestContext, Quality
from scyg_agent.agents.execution import PostgreSQLExecutionDispatch
from scyg_agent.agents.production import (
    AgentGraph,
    AgentRunnerConfig,
    ApprovalRejectedError,
    LangChainAgentRunner,
    RecipeGraph,
    WritingApproval,
)
from scyg_agent.agents.recipes import default_recipe_registry
from scyg_agent.agents.runner import AgentFailed, AgentSucceeded, AgentWaitingForApproval
from scyg_agent.domain.runs import RunId
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.domain.runs.repository import LeaseGuard
from tests.adapters.database.scripted_session_support import FakeResult, ScriptedSession, session
from tests.worker.test_executor import OWNER, RUN_ID, lease, running_run

if TYPE_CHECKING:
    from langchain.agents.middleware.types import AgentState
    from langchain_core.runnables import RunnableConfig

DRAFT = ArticleDraft(
    title="Checkpoint title",
    outline="Checkpoint outline",
    markdown="# Preserved draft",
)


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class ApprovalState(TypedDict, total=False):
    structured_response: Required[ArticleDraft]
    approved: bool


class ApprovalObservation(ApprovalState, total=False):
    """Observe SDK-returned metadata without registering a reserved graph channel."""

    __interrupt__: tuple[Interrupt, ...]


def approval_graph() -> CompiledStateGraph[
    ApprovalState,
    AgentRequestContext,
    ApprovalState,
    ApprovalState,
]:
    middleware = WritingApproval()

    async def node(state: ApprovalState, runtime: Runtime[AgentRequestContext]) -> dict[str, bool]:
        agent_state: AgentState[object] = {
            "messages": [],
            "structured_response": state["structured_response"],
        }
        await middleware.aafter_agent(agent_state, runtime)
        return {"approved": True}

    builder: StateGraph[
        ApprovalState,
        AgentRequestContext,
        ApprovalState,
        ApprovalState,
    ] = StateGraph(ApprovalState, context_schema=AgentRequestContext)
    _ = builder.add_node("approval", node)
    _ = builder.add_edge(START, "approval")
    _ = builder.add_edge("approval", END)
    return builder.compile(checkpointer=InMemorySaver())


class Inputs:
    def __init__(self) -> None:
        self.calls: int = 0

    async def get_input(self, run_id: RunId) -> RunInput:
        self.calls += 1
        value = WritingInput(topic="Checkpoint article", reference_article_ids=(1,))
        return RunInput(
            "Checkpoint article",
            "",
            capability="write",
            recipe_id="writing-v1",
            recipe_version="v1",
            input_schema_version="v1",
            input_payload=input_payload(value),
            input_digest=input_digest(value),
            locale="und",
            thread_id=str(run_id),
            quality="standard",
            state_schema_version="v1",
        )


class Sessions:
    def __init__(self, value: AsyncSession) -> None:
        self.value: AsyncSession = value

    @asynccontextmanager
    async def begin(self) -> AsyncIterator[AsyncSession]:
        yield self.value

    @asynccontextmanager
    async def __call__(self) -> AsyncIterator[AsyncSession]:
        yield self.value

    def factory(self) -> async_sessionmaker[AsyncSession]:
        return cast("async_sessionmaker[AsyncSession]", cast("object", self))


def runner_for(graph: object, inputs: Inputs, sessions: Sessions) -> LangChainAgentRunner:
    registry = default_recipe_registry()
    graphs = tuple(RecipeGraph(recipe, cast("AgentGraph", graph)) for recipe in registry.recipes)
    return LangChainAgentRunner(AgentRunnerConfig(inputs, registry, graphs, sessions.factory()))


def active_record() -> RunRecord:
    return RunRecord(
        run_id=str(RUN_ID),
        status="running",
        revision=2,
        lease_owner=str(OWNER),
        lease_token=lease().token.value,
        lease_expires_at=datetime.now(UTC) + timedelta(minutes=5),
        cancellation_requested_at=None,
    )


@pytest.mark.anyio
@pytest.mark.parametrize("decision", ["approve", "reject"])
async def test_real_writing_after_agent_interrupt_preserves_complete_draft(
    monkeypatch: pytest.MonkeyPatch, decision: str
) -> None:
    graph = approval_graph()
    database = Sessions(session(monkeypatch, ScriptedSession([])))
    guard = PostgreSQLExecutionDispatch(
        database.factory(),
        LeaseGuard(RUN_ID, OWNER, lease().token, 2, datetime.now(UTC)),
        lambda: datetime.now(UTC),
    )
    context = AgentRequestContext(
        running_run().owner_user_id,
        str(RUN_ID),
        Capability.WRITE,
        RecipeId.WRITING_V1,
        "v1",
        "und",
        Quality.STANDARD,
        execution_guard=guard,
    )
    config: RunnableConfig = {"configurable": {"thread_id": str(RUN_ID)}}
    first = cast(
        "ApprovalObservation",
        await graph.ainvoke(
            {"structured_response": DRAFT},
            config,
            context=context,
        ),
    )
    assert "__interrupt__" in first
    pending = first["__interrupt__"][0]
    request = ApprovalRequest.model_validate(cast("object", pending.value))
    assert request.title == DRAFT.title
    assert request.outline == DRAFT.outline
    state = await graph.aget_state(config)
    values = cast("ApprovalState", cast("object", state.values))
    assert values["structured_response"] == DRAFT
    command: Command[object] = Command(resume={pending.id: {"decision": decision}})
    if decision == "reject":
        with pytest.raises(ApprovalRejectedError):
            _ = await graph.ainvoke(command, config, context=context)
    else:
        result = cast("ApprovalState", await graph.ainvoke(command, config, context=context))
        assert "approved" in result
        assert result["approved"] is True
        assert result["structured_response"] == DRAFT


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("present", "payload"),
    [(False, None), (True, None), (True, {"choice": 1})],
)
async def test_persisted_reply_preserves_payload_presence(
    monkeypatch: pytest.MonkeyPatch,
    payload: object,
    *,
    present: bool,
) -> None:
    request = ApprovalRequest(
        interaction_id="int_presence01",
        kind="selection",
        title="Choose",
        outline="Options",
    )
    replies: list[dict[str, object]] = []

    def node(_state: ApprovalState) -> dict[str, bool]:
        reply = cast("object", interrupt(request.model_dump(mode="json")))
        assert isinstance(reply, dict)
        replies.append(cast("dict[str, object]", reply))
        return {"approved": True}

    builder: StateGraph[
        ApprovalState,
        AgentRequestContext,
        ApprovalState,
        ApprovalState,
    ] = StateGraph(ApprovalState, context_schema=AgentRequestContext)
    _ = builder.add_node("selection", node)
    _ = builder.add_edge(START, "selection")
    _ = builder.add_edge("selection", END)
    graph = builder.compile(checkpointer=InMemorySaver())
    config: RunnableConfig = {"configurable": {"thread_id": str(RUN_ID)}}
    _ = await graph.ainvoke({"structured_response": DRAFT}, config)
    record = InteractionRecord(
        interaction_id=request.interaction_id,
        run_id=str(RUN_ID),
        status="resolved",
        kind=request.kind,
        decision="select",
        request_payload=request.model_dump(mode="json"),
        payload_present=present,
        response_payload=payload,
    )
    script = ScriptedSession(
        [
            FakeResult(active_record()),
            FakeResult(record),
            FakeResult(active_record()),
        ]
    )
    runner = runner_for(graph, Inputs(), Sessions(session(monkeypatch, script)))
    outcome = await runner.execute(running_run(), lease())
    assert isinstance(outcome, AgentSucceeded)
    assert len(replies) == 1
    reply = replies[0]
    assert reply["decision"] == "select"
    assert ("payload" in reply) is present
    if present:
        assert reply["payload"] == payload


@pytest.mark.anyio
async def test_cancelled_admission_does_not_read_input_or_invoke_graph(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    record = active_record()
    record.status = "cancelled"
    inputs = Inputs()
    database = Sessions(session(monkeypatch, ScriptedSession([FakeResult(record)])))
    runner = runner_for(approval_graph(), inputs, database)
    outcome = await runner.execute(running_run(), lease())
    assert isinstance(outcome, AgentFailed)
    assert inputs.calls == 0


@pytest.mark.anyio
async def test_runner_resumes_matching_persisted_confirmation_without_restarting_graph(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = approval_graph()
    database = Sessions(session(monkeypatch, ScriptedSession([])))
    guard = PostgreSQLExecutionDispatch(
        database.factory(),
        LeaseGuard(RUN_ID, OWNER, lease().token, 2, datetime.now(UTC)),
        lambda: datetime.now(UTC),
    )
    context = AgentRequestContext(
        running_run().owner_user_id,
        str(RUN_ID),
        Capability.WRITE,
        RecipeId.WRITING_V1,
        "v1",
        "und",
        Quality.STANDARD,
        execution_guard=guard,
    )
    config: RunnableConfig = {"configurable": {"thread_id": str(RUN_ID)}}
    first = cast(
        "ApprovalObservation",
        await graph.ainvoke(
            {"structured_response": DRAFT},
            config,
            context=context,
        ),
    )
    assert "__interrupt__" in first
    request = ApprovalRequest.model_validate(cast("object", first["__interrupt__"][0].value))
    record = InteractionRecord(
        interaction_id=request.interaction_id,
        run_id=str(RUN_ID),
        status="resolved",
        kind=request.kind,
        decision="approve",
        request_payload=request.model_dump(mode="json"),
        payload_present=False,
        response_payload=None,
    )
    script = ScriptedSession(
        [FakeResult(active_record()), FakeResult(record), FakeResult(active_record())],
    )
    resumed_database = Sessions(session(monkeypatch, script))
    runner = runner_for(graph, Inputs(), resumed_database)
    outcome = await runner.execute(running_run(), lease())
    assert isinstance(outcome, AgentSucceeded)
    assert outcome.output == DRAFT
    assert script.results == []


@pytest.mark.anyio
async def test_runner_reports_unresolved_checkpoint_without_model_restart(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    graph = approval_graph()
    database = Sessions(session(monkeypatch, ScriptedSession([])))
    guard = PostgreSQLExecutionDispatch(
        database.factory(),
        LeaseGuard(RUN_ID, OWNER, lease().token, 2, datetime.now(UTC)),
        lambda: datetime.now(UTC),
    )
    context = AgentRequestContext(
        running_run().owner_user_id,
        str(RUN_ID),
        Capability.WRITE,
        RecipeId.WRITING_V1,
        "v1",
        "und",
        Quality.STANDARD,
        execution_guard=guard,
    )
    config: RunnableConfig = {"configurable": {"thread_id": str(RUN_ID)}}
    _ = await graph.ainvoke({"structured_response": DRAFT}, config, context=context)
    script = ScriptedSession([FakeResult(active_record()), FakeResult(None)])
    runner = runner_for(graph, Inputs(), Sessions(session(monkeypatch, script)))
    outcome = await runner.execute(running_run(), lease())
    assert isinstance(outcome, AgentWaitingForApproval)
    assert outcome.request.title == DRAFT.title
    assert script.results == []
