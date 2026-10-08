"""Real-channel deadline, sanitized failures and close behavior."""

import grpc
import pytest

from scyg_agent.adapters.blog_grpc import (
    ArticleId,
    BlogFailure,
    BlogGrpcClient,
    BlogUserId,
    FailureKind,
    GetArticle,
)

from .support import FakeBlog, dispatch_call
from .test_client import identity


def command() -> GetArticle:
    return GetArticle(identity(), BlogUserId("admin"), ArticleId(1))


@pytest.mark.anyio
async def test_deadline_maps_retryable_failure(blog_server: tuple[str, FakeBlog]) -> None:
    target, fake = blog_server
    fake.block = True
    async with BlogGrpcClient(target, 0.01) as client:
        assert await client.invoke(
            "get_article",
            "v1",
            command(),
            dispatch=dispatch_call,
        ) == BlogFailure(FailureKind.DEADLINE_EXCEEDED, retryable=True)


@pytest.mark.anyio
async def test_remote_failure_is_sanitized(blog_server: tuple[str, FakeBlog]) -> None:
    target, fake = blog_server
    fake.status = grpc.StatusCode.PERMISSION_DENIED
    async with BlogGrpcClient(target, 1.0) as client:
        result = await client.invoke("get_article", "v1", command(), dispatch=dispatch_call)
    assert result == BlogFailure(FailureKind.AUTHORIZATION, retryable=False)
    assert "server-secret-token" not in repr(result)


@pytest.mark.anyio
async def test_malformed_article_is_protocol_failure(blog_server: tuple[str, FakeBlog]) -> None:
    target, fake = blog_server
    fake.malformed = True
    async with BlogGrpcClient(target, 1.0) as client:
        assert await client.invoke(
            "get_article",
            "v1",
            command(),
            dispatch=dispatch_call,
        ) == BlogFailure(FailureKind.INVALID_RESPONSE, retryable=False)


@pytest.mark.anyio
async def test_closed_client_never_calls_server(blog_server: tuple[str, FakeBlog]) -> None:
    target, fake = blog_server
    client = BlogGrpcClient(target, 1.0)
    await client.close()
    await client.close()
    assert await client.invoke(
        "get_article",
        "v1",
        command(),
        dispatch=dispatch_call,
    ) == BlogFailure(FailureKind.CHANNEL_CLOSED, retryable=False)
    assert fake.calls == []
