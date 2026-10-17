"""Reclaimed public checkpoints reauthorize evidence before real SDK tool execution."""

from dataclasses import replace
from datetime import UTC, datetime, timedelta
from typing import cast, override

import pytest
from langchain_core.messages import AIMessage, ToolMessage
from langgraph.checkpoint.memory import InMemorySaver
from langgraph.graph import END, START, StateGraph
from langgraph.prebuilt import ToolNode
from langgraph.runtime import Runtime

from scyg_agent.adapters.blog_grpc.contracts import (
    ArticleSucceeded,
    BlogCommand,
    BlogResult,
    GetArticle,
    RpcDispatch,
)
from scyg_agent.agents import (
    FailureKind,
    RecipeId,
    SearchInput,
    SearchResponse,
    input_digest,
    input_payload,
)
from scyg_agent.agents.context import AgentRequestContext
from scyg_agent.agents.production import (
    AgentRunnerConfig,
    LangChainAgentRunner,
    PublicAgentState,
    RecipeGraph,
    SDKAgentGraph,
)
from scyg_agent.agents.recipes import default_recipe_registry
from scyg_agent.agents.runner import AgentFailed, AgentSucceeded
from scyg_agent.agents.tool_gateway import BlogReadGateway
from scyg_agent.config import PublicSearchSettings
from scyg_agent.domain.runs import ContentScope, RunId
from scyg_agent.domain.runs.input import RunInput
from tests.adapters.database.scripted_session_support import FakeResult, ScriptedSession, session
from tests.worker.test_executor import RUN_ID, lease, running_run

from .test_production import Inputs, Sessions, active_record
from .test_tool_gateway import SeededBlog, article


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class SearchInputs(Inputs):
    @override
    async def get_input(self, run_id: RunId) -> RunInput:
        value = SearchInput(query="Published only")
        return RunInput(
            value.query,
            "",
            capability="search",
            recipe_id="search-v1",
            recipe_version="v1",
            input_schema_version="v1",
            input_payload=input_payload(value),
            input_digest=input_digest(value),
            locale="und",
            thread_id=str(run_id),
            quality="standard",
            state_schema_version="v1",
        )


class RecoveryBlog(SeededBlog):
    def __init__(self, *, private: bool, large_reads: bool) -> None:
        super().__init__()
        self.private: bool = private
        self.large_reads: bool = large_reads

    @override
    async def invoke(
        self, tool_name: str, version: str, command: BlogCommand, *, dispatch: RpcDispatch
    ) -> BlogResult:
        assert version == "v1"
        assert isinstance(command, GetArticle)
        assert command.content_scope is ContentScope.PUBLIC

        async def result() -> BlogResult:
            self.calls.append((tool_name, command))
            article_id = int(command.article_id)
            # Visibility RPCs must not serialize/charge these fresh bodies. Only
            # the six new model reads (IDs >= 8) deliver actual model evidence.
            return ArticleSucceeded(
                replace(
                    article(article_id),
                    status=1 if self.private else 2,
                    content=(
                        "fresh body not sent to model" * 2_000
                        if article_id <= 7
                        else "public" * (1_000 if self.large_reads else 1)
                    ),
                )
            )

        return cast("BlogResult", await dispatch(result))


@pytest.mark.anyio
@pytest.mark.parametrize(
    "case",
    [
        "six_reads",
        "seventh_read",
        "read_limit",
        "checkpoint_limit",
        "private",
        "cancelled",
        "expired",
    ],
)
async def test_public_checkpoint_reauthorization_precedes_sdk_resume(
    monkeypatch: pytest.MonkeyPatch, case: str
) -> None:
    registry = default_recipe_registry()
    recipe = registry.resolve(RecipeId.SEARCH_V1, "v1")
    assert recipe is not None
    blog = RecoveryBlog(private=case == "private", large_reads=case == "read_limit")
    gateway = BlogReadGateway(blog, registry)
    completed: list[AgentRequestContext] = []

    async def seed(_state: PublicAgentState) -> dict[str, object]:
        return {}

    async def finish(
        state: PublicAgentState, runtime: Runtime[AgentRequestContext]
    ) -> dict[str, object]:
        completed.append(runtime.context)
        # Reauthorization never appends the newly fetched historical bodies.
        assert all(
            "fresh body not sent to model" not in str(message.content)
            for message in state["messages"]
        )
        return {
            "structured_response": SearchResponse(
                query="Published only", conclusion="Public evidence"
            )
        }

    builder = StateGraph(PublicAgentState, context_schema=AgentRequestContext)
    _ = builder.add_node("seed", seed)
    _ = builder.add_node(
        "tools",
        ToolNode(
            gateway.tools_for(recipe),
            handle_tool_errors=False,
            awrap_tool_call=gateway.middleware.awrap_tool_call,
        ),
    )
    _ = builder.add_node("finish", finish)
    _ = builder.add_edge(START, "seed")
    _ = builder.add_edge("seed", "tools")
    _ = builder.add_edge("tools", "finish")
    _ = builder.add_edge("finish", END)
    graph = builder.compile(checkpointer=InMemorySaver())
    count = 7 if case == "seventh_read" else 6
    _ = await graph.aupdate_state(
        {"configurable": {"thread_id": str(RUN_ID)}},
        {
            "authorized_article_ids": list(range(1, 8)),
            "messages": [
                ToolMessage(content="prior public evidence", tool_call_id="prior"),
                AIMessage(
                    content="",
                    tool_calls=[
                        {"name": "get_article", "args": {"article_id": index}, "id": f"new_{index}"}
                        for index in range(8, 8 + count)
                    ],
                ),
            ],
        },
        as_node="seed",
    )
    # Every external read uses the real database lease fence. Cancellation at
    # the first visibility read must prevent both the RPC and checkpoint resume.
    records = [active_record() for _ in range(2 + 7 + min(count, 6))]
    if case == "cancelled":
        records[2].cancellation_requested_at = datetime.now(UTC)
    script = ScriptedSession([FakeResult(record) for record in records])
    runner = LangChainAgentRunner(
        AgentRunnerConfig(
            SearchInputs(),
            registry,
            tuple(RecipeGraph(item, SDKAgentGraph(graph)) for item in registry.recipes),
            Sessions(session(monkeypatch, script)).factory(),
            public_search=PublicSearchSettings(
                max_read_chars_per_attempt=20 if case == "checkpoint_limit" else 24_000
            ),
            gateway=gateway,
        )
    )
    run = replace(
        running_run(),
        content_scope=ContentScope.PUBLIC,
        public_execution_deadline=datetime.now(UTC)
        + timedelta(seconds=-1 if case == "expired" else 180),
    )
    outcome = await runner.execute(run, lease())
    if case == "six_reads":
        assert isinstance(outcome, AgentSucceeded)
        assert len(blog.calls) == 13
        assert len(completed) == 1
        budget = completed[0].public_budget
        assert budget is not None
        assert budget.tool_calls == 6
        assert 0 < budget.read_chars < budget.settings.max_read_chars_per_attempt
        assert completed[0].authorized_article_ids == set(range(1, 14))
    else:
        assert isinstance(outcome, AgentFailed)
        assert not completed
        if case in {"seventh_read", "read_limit"}:
            assert outcome.failure.kind is FailureKind.POLICY_LIMIT
            assert 7 < len(blog.calls) <= 13
        elif case == "private":
            assert outcome.failure.kind is FailureKind.FORBIDDEN
            assert len(blog.calls) == 1
        else:
            assert not blog.calls
            if case in {"expired", "checkpoint_limit"}:
                assert outcome.failure.kind is FailureKind.POLICY_LIMIT
