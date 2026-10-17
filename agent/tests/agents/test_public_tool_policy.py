"""Real SDK ToolNode must abort instead of feeding policy failures to a model."""

from dataclasses import replace
from datetime import UTC, datetime, timedelta
from typing import override

import pytest
from langchain_core.messages import AIMessage
from langgraph.graph import END, START, MessagesState, StateGraph
from langgraph.prebuilt import ToolNode

from scyg_agent.adapters.blog_grpc.contracts import (
    ArticleSucceeded,
    BlogCommand,
    BlogResult,
    GetArticle,
    RpcDispatch,
)
from scyg_agent.agents.public_policy import PolicyLimitError, PublicAttemptBudget
from scyg_agent.agents.recipes import default_recipe_registry
from scyg_agent.agents.tool_catalog import ReadToolName
from scyg_agent.agents.tool_gateway import BlogReadGateway
from scyg_agent.config import PublicSearchSettings
from scyg_agent.domain.runs import ContentScope

from .test_tool_gateway import CONFIG, Guard, SeededBlog, article, context


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class PublishedBlog(SeededBlog):
    @override
    async def invoke(
        self, tool_name: str, version: str, command: BlogCommand, *, dispatch: RpcDispatch
    ) -> BlogResult:
        async def result() -> BlogResult:
            self.calls.append((tool_name, command))
            assert isinstance(command, GetArticle)
            return ArticleSucceeded(replace(article(int(command.article_id)), status=2))

        observed = await dispatch(result)
        assert isinstance(observed, ArticleSucceeded)
        return observed


@pytest.mark.anyio
async def test_parallel_public_calls_reserve_before_rpc_and_policy_error_bubbles() -> None:
    blog = PublishedBlog()
    registry = default_recipe_registry()
    gateway = BlogReadGateway(blog, registry)
    request_context = replace(
        context(Guard()),
        content_scope=ContentScope.PUBLIC,
        public_budget=PublicAttemptBudget(
            PublicSearchSettings(max_tool_calls_per_attempt=1),
            datetime.now(UTC) + timedelta(minutes=1),
        ),
    )
    recipe = registry.resolve(request_context.recipe_id, "v1")
    assert recipe is not None
    builder = StateGraph(MessagesState, context_schema=type(request_context))
    # Default SDK error handling is deliberately enabled: policy must escape it.
    _ = builder.add_node(
        "tools",
        ToolNode(gateway.tools_for(recipe), awrap_tool_call=gateway.middleware.awrap_tool_call),
    )
    _ = builder.add_edge(START, "tools")
    _ = builder.add_edge("tools", END)
    calls = [
        {
            "name": ReadToolName.GET_ARTICLE.value,
            "args": {"article_id": index},
            "id": f"call_{index}",
            "type": "tool_call",
        }
        for index in (1, 2)
    ]
    with pytest.raises(PolicyLimitError):
        _ = await builder.compile().ainvoke(
            {"messages": [AIMessage(content="", tool_calls=calls)]}, CONFIG, context=request_context
        )
    assert len(blog.calls) <= 1
    for _, command in blog.calls:
        assert isinstance(command, GetArticle)
        assert command.content_scope is ContentScope.PUBLIC


@pytest.mark.anyio
async def test_over_budget_whole_result_never_becomes_authorized_evidence() -> None:
    blog = PublishedBlog()
    gateway = BlogReadGateway(blog, default_recipe_registry())
    request_context = replace(
        context(Guard()),
        content_scope=ContentScope.PUBLIC,
        public_budget=PublicAttemptBudget(
            PublicSearchSettings(max_read_chars_per_attempt=1),
            datetime.now(UTC) + timedelta(minutes=1),
        ),
    )
    with pytest.raises(PolicyLimitError):
        _ = await gateway.read_reference(request_context, 1)
    assert len(blog.calls) == 1
    assert not request_context.authorized_article_ids
