"""Blog gRPC 取消、并发、关闭和畸形响应测试。"""

import anyio
import grpc
import pytest

from scyg_agent.adapters.blog_grpc import (
    ArticleId,
    ArticleSucceeded,
    BlogFailure,
    BlogGrpcClient,
    FailureKind,
    GetPublishedArticle,
    SearchArticles,
)

from .support import FakeBlog
from .test_client import all_tools_profile, identity


@pytest.mark.anyio
async def test_deadline_and_remote_details_map_without_secret(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    fake.block = True
    command = GetPublishedArticle(identity(), ArticleId("article-1"))
    async with BlogGrpcClient(target, all_tools_profile(), 0.01) as client:
        result = await client.invoke("get_published_article", "v1", command)
    assert result == BlogFailure(FailureKind.DEADLINE_EXCEEDED, retryable=True)
    assert "secret" not in str(result)
    with anyio.fail_after(1):
        await fake.drained.wait()


@pytest.mark.anyio
async def test_remote_cancelled_is_typed_failure(blog_server: tuple[str, FakeBlog]) -> None:
    target, fake = blog_server
    fake.status = grpc.StatusCode.CANCELLED
    async with BlogGrpcClient(target, all_tools_profile(), 1.0) as client:
        result = await client.invoke(
            "get_published_article", "v1", GetPublishedArticle(identity(), ArticleId("a"))
        )
    assert result == BlogFailure(FailureKind.CANCELLED, retryable=False)


@pytest.mark.anyio
async def test_caller_cancellation_propagates_and_drains_server(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    fake.block = True
    cancelled = anyio.Event()

    async def invoke() -> None:
        try:
            async with BlogGrpcClient(target, all_tools_profile(), 5.0) as client:
                _ = await client.invoke(
                    "get_published_article",
                    "v1",
                    GetPublishedArticle(identity(), ArticleId("a")),
                )
        except anyio.get_cancelled_exc_class():
            cancelled.set()
            raise

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(invoke)
        await fake.started.wait()
        tasks.cancel_scope.cancel()
    assert cancelled.is_set()
    with anyio.fail_after(1):
        await fake.drained.wait()


@pytest.mark.anyio
async def test_channel_shutdown_cancels_active_call_and_rejects_next_call(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    fake.block = True
    client = BlogGrpcClient(target, all_tools_profile(), 5.0)
    cancelled = anyio.Event()

    async def invoke() -> None:
        try:
            _ = await client.invoke(
                "get_published_article",
                "v1",
                GetPublishedArticle(identity(), ArticleId("a")),
            )
        except anyio.get_cancelled_exc_class():
            cancelled.set()
            raise

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(invoke)
        await fake.started.wait()
        await client.close()
    assert cancelled.is_set()
    result = await client.invoke(
        "get_published_article", "v1", GetPublishedArticle(identity(), ArticleId("a"))
    )
    assert result == BlogFailure(FailureKind.CHANNEL_CLOSED, retryable=False)


@pytest.mark.anyio
async def test_concurrent_calls_share_channel_without_response_crosstalk(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, _fake = blog_server
    results: list[ArticleSucceeded | BlogFailure] = []

    async def invoke(client: BlogGrpcClient, suffix: str) -> None:
        result = await client.invoke(
            "get_published_article",
            "v1",
            GetPublishedArticle(identity(suffix), ArticleId(f"article-{suffix}")),
        )
        if isinstance(result, ArticleSucceeded | BlogFailure):
            results.append(result)

    async with (
        BlogGrpcClient(target, all_tools_profile(), 1.0) as client,
        anyio.create_task_group() as tasks,
    ):
        for suffix in ("1", "2", "3", "4"):
            _ = tasks.start_soon(invoke, client, suffix)
    article_ids = {
        result.article.article_id for result in results if isinstance(result, ArticleSucceeded)
    }
    assert article_ids == {ArticleId(f"article-{suffix}") for suffix in ("1", "2", "3", "4")}


@pytest.mark.anyio
@pytest.mark.parametrize("response_kind", ["article", "search"])
async def test_semantically_malformed_response_is_rejected(
    blog_server: tuple[str, FakeBlog], response_kind: str
) -> None:
    target, fake = blog_server
    fake.malformed = True
    search = response_kind == "search"
    command = (
        SearchArticles(identity(), "查询", 10)
        if search
        else GetPublishedArticle(identity(), ArticleId("article-1"))
    )
    tool_name = "search_articles" if search else "get_published_article"
    async with BlogGrpcClient(target, all_tools_profile(), 1.0) as client:
        result = await client.invoke(tool_name, "v1", command)
    assert result == BlogFailure(FailureKind.INVALID_RESPONSE, retryable=False)
