"""Real ToolNode authorization, management reads and restored-call fencing."""

from collections.abc import Awaitable, Callable
from typing import cast

import pytest
from langchain_core.messages import AIMessage
from langchain_core.runnables import RunnableConfig
from langgraph.graph import END, START, MessagesState, StateGraph
from langgraph.graph.state import CompiledStateGraph
from langgraph.prebuilt import ToolNode

from scyg_agent.adapters.blog_grpc.contracts import (
    ArticleId,
    ArticleResult,
    ArticleSucceeded,
    BlogCommand,
    BlogFailure,
    BlogResult,
    FailureKind,
    GetArticle,
    RpcDispatch,
    SearchArticles,
    SearchSucceeded,
    TaxonomyResult,
)
from scyg_agent.agents.context import AgentRequestContext, Quality
from scyg_agent.agents.contracts import Capability, RecipeId
from scyg_agent.agents.recipes import default_recipe_registry
from scyg_agent.agents.tool_gateway import (
    BlogReadGateway,
    BlogToolFailureError,
    ToolAccessDeniedError,
)
from scyg_agent.domain.runs import UserId

RUN_ID = "run_gateway001"
CONFIG: RunnableConfig = {"configurable": {"thread_id": RUN_ID}}


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class Guard:
    def __init__(self) -> None:
        self.calls: int = 0
        self.cancelled: bool = False

    async def dispatch[T](self, start: Callable[[], Awaitable[T]]) -> T:
        if self.cancelled:
            message = "cancelled"
            raise PermissionError(message)
        self.calls += 1
        call = start()
        return await call


def article(article_id: int = 1) -> ArticleResult:
    return ArticleResult(
        ArticleId(article_id),
        "Title",
        "draft content",
        "Digest",
        1,
        1,
        3,
        "slug",
        (TaxonomyResult(2, "Tag"),),
        "1970-01-01T00:00:01Z",
        "1970-01-01T00:00:02Z",
        None,
        0,
        0,
        0,
    )


class SeededBlog:
    def __init__(self) -> None:
        self.calls: list[tuple[str, BlogCommand]] = []
        self.failure: BlogFailure | None = None

    async def invoke(
        self,
        tool_name: str,
        version: str,
        command: BlogCommand,
        *,
        dispatch: RpcDispatch,
    ) -> BlogResult:
        assert version == "v1"

        async def response() -> BlogResult:
            self.calls.append((tool_name, command))
            if self.failure is not None:
                return self.failure
            if isinstance(command, SearchArticles):
                return SearchSucceeded((article(),), command.page, command.page_size, 1, 1)
            if isinstance(command, GetArticle):
                return ArticleSucceeded(article(command.article_id))
            message = "unexpected command"
            raise AssertionError(message)

        return cast("BlogResult", await dispatch(response))


def context(guard: Guard) -> AgentRequestContext:
    return AgentRequestContext(
        UserId("admin"),
        RUN_ID,
        Capability.SEARCH,
        RecipeId.SEARCH_V1,
        "v1",
        "zh-CN",
        Quality.STANDARD,
        execution_guard=guard,
    )


def graph_for(
    blog: SeededBlog,
) -> tuple[
    CompiledStateGraph[MessagesState, AgentRequestContext, MessagesState, MessagesState],
    BlogReadGateway,
]:
    registry = default_recipe_registry()
    recipe = registry.resolve(RecipeId.SEARCH_V1, "v1")
    assert recipe is not None
    gateway = BlogReadGateway(blog, registry)
    builder: StateGraph[
        MessagesState,
        AgentRequestContext,
        MessagesState,
        MessagesState,
    ] = StateGraph(MessagesState, context_schema=AgentRequestContext)
    _ = builder.add_node(
        "tools",
        ToolNode(
            gateway.tools_for(recipe),
            handle_tool_errors=False,
            awrap_tool_call=gateway.middleware.awrap_tool_call,
        ),
    )
    _ = builder.add_edge(START, "tools")
    _ = builder.add_edge("tools", END)
    return builder.compile(), gateway


def message(name: str, args: dict[str, object]) -> AIMessage:
    return AIMessage(content="", tool_calls=[{"name": name, "args": args, "id": "sdk-call-001"}])


@pytest.mark.anyio
async def test_all_management_reads_use_owner_numeric_ids_and_fence() -> None:
    blog, guard = SeededBlog(), Guard()
    graph, _ = graph_for(blog)
    cases: tuple[tuple[str, dict[str, object]], ...] = (
        (
            "search_articles",
            {
                "query": "draft",
                "page": 2,
                "page_size": 10,
                "status": 1,
                "article_type_id": 3,
                "tag_id": 2,
                "sort": "title",
            },
        ),
        ("get_article", {"article_id": 1}),
    )
    for name, args in cases:
        state: MessagesState = {"messages": [message(name, args)]}
        output = cast("MessagesState", await graph.ainvoke(state, CONFIG, context=context(guard)))
        assert output["messages"][-1].content
    assert guard.calls == 2
    assert all(command.user_id == "admin" for _, command in blog.calls)
    search = blog.calls[0][1]
    assert isinstance(search, SearchArticles)
    assert (
        search.page,
        search.page_size,
        search.status,
        search.article_type_id,
        search.tag_id,
    ) == (2, 10, 1, 3, 2)
    assert isinstance(blog.calls[1][1], GetArticle)
    assert blog.calls[1][1].article_id == 1


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("name", "args"),
    [
        ("get_published_article", {"article_id": 1}),
        ("create_article", {}),
        ("archive_article", {}),
        ("unknown", {}),
        ("get_article", {"article_id": "1"}),
        ("search_articles", {"page_token": "old"}),
        ("search_articles", {"page": 0}),
        ("list_tags", {}),
        ("list_article_types", {}),
    ],
)
async def test_unapproved_or_invalid_restored_calls_never_reach_blog(
    name: str,
    args: dict[str, object],
) -> None:
    blog = SeededBlog()
    graph, _ = graph_for(blog)
    with pytest.raises(ToolAccessDeniedError):
        _ = await graph.ainvoke(
            {"messages": [message(name, args)]},
            CONFIG,
            context=context(Guard()),
        )
    assert blog.calls == []


@pytest.mark.anyio
async def test_cancelled_guard_never_dispatches() -> None:
    blog = SeededBlog()
    graph, _ = graph_for(blog)
    guard = Guard()
    guard.cancelled = True
    with pytest.raises(BlogToolFailureError):
        _ = await graph.ainvoke(
            {"messages": [message("get_article", {"article_id": 1})]},
            CONFIG,
            context=context(guard),
        )
    assert blog.calls == []


@pytest.mark.anyio
async def test_blog_failure_remains_sanitized() -> None:
    blog = SeededBlog()
    blog.failure = BlogFailure(FailureKind.UNAVAILABLE, retryable=True)
    graph, _ = graph_for(blog)
    with pytest.raises(BlogToolFailureError) as failure:
        _ = await graph.ainvoke(
            {"messages": [message("get_article", {"article_id": 1})]},
            CONFIG,
            context=context(Guard()),
        )
    assert failure.value.retryable
