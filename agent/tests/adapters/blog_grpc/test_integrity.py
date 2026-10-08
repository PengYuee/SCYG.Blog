"""Allowlist and synchronous final Call dispatch boundary."""

from collections.abc import Awaitable, Callable

import pytest

from scyg_agent.adapters.blog_grpc import (
    ArchiveArticle,
    ArticleId,
    ArticleSucceeded,
    BlogFailure,
    BlogGrpcClient,
    BlogUserId,
    CreateArticle,
    FailureKind,
    GetArticle,
    InvalidDeadlineError,
    PublishArticle,
    UpdateArticle,
)
from scyg_agent.domain.runs import OperationId

from .support import FakeBlog, dispatch_call
from .test_client import identity


@pytest.mark.anyio
async def test_rejected_commands_never_start_rpc(blog_server: tuple[str, FakeBlog]) -> None:
    target, fake = blog_server
    command = GetArticle(identity(), BlogUserId("admin"), ArticleId(1))
    async with BlogGrpcClient(target, 1.0) as client:
        assert await client.invoke(
            "unknown",
            "v1",
            command,
            dispatch=dispatch_call,
        ) == BlogFailure(FailureKind.INVALID_TOOL, retryable=False)
        assert await client.invoke(
            "get_article",
            "v2",
            command,
            dispatch=dispatch_call,
        ) == BlogFailure(FailureKind.INVALID_VERSION, retryable=False)
        assert await client.invoke(
            "search_articles",
            "v1",
            command,
            dispatch=dispatch_call,
        ) == BlogFailure(FailureKind.WRONG_COMMAND, retryable=False)
        for name in (
            "get_published_article",
            "create_article_draft",
            "update_article_draft",
            "add_article_tags",
        ):
            assert await client.invoke(
                name,
                "v1",
                command,
                dispatch=dispatch_call,
            ) == BlogFailure(FailureKind.INVALID_TOOL, retryable=False)
    assert fake.calls == []


@pytest.mark.anyio
async def test_call_is_created_synchronously_inside_dispatch_and_awaited_outside(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    lock_held = False
    seen: list[bool] = []

    async def dispatch(start: Callable[[], Awaitable[object]]) -> object:
        nonlocal lock_held
        lock_held = True
        call = start()
        # grpc.aio Call creation, not an async coroutine deferred until await.
        assert hasattr(call, "cancel")
        assert hasattr(call, "done")
        seen.append(lock_held)
        lock_held = False
        return await call

    async with BlogGrpcClient(target, 1.0) as client:
        result = await client.invoke(
            "get_article",
            "v1",
            GetArticle(identity(), BlogUserId("admin"), ArticleId(1)),
            dispatch=dispatch,
        )
    assert isinstance(result, ArticleSucceeded)
    assert seen == [True]
    assert not lock_held
    assert fake.calls == ["get"]


@pytest.mark.anyio
async def test_cancelled_dispatch_does_not_create_call(blog_server: tuple[str, FakeBlog]) -> None:
    target, fake = blog_server

    async def refused(_start: Callable[[], Awaitable[object]]) -> object:
        message = "cancelled Run"
        raise PermissionError(message)

    async with BlogGrpcClient(target, 1.0) as client:
        with pytest.raises(PermissionError):
            _ = await client.invoke(
                "get_article",
                "v1",
                GetArticle(identity(), BlogUserId("admin"), ArticleId(1)),
                dispatch=refused,
            )
    assert fake.calls == []


@pytest.mark.parametrize("deadline", [0.0, -1.0, float("inf"), float("nan"), 3600.1])
def test_invalid_deadline_rejected_before_channel_creation(deadline: float) -> None:
    with pytest.raises(InvalidDeadlineError):
        _ = BlogGrpcClient("127.0.0.1:1", deadline)


@pytest.mark.anyio
@pytest.mark.parametrize(
    "operation_id",
    [
        "op-obsolete",
        "00000000-0000-1000-8000-000000000000",
        "5BF6F8A2-70F9-47A7-A101-F2DA1C807C01",
    ],
)
async def test_all_writes_reject_noncanonical_uuidv4_before_rpc(
    blog_server: tuple[str, FakeBlog],
    operation_id: str,
) -> None:
    """Arbitrary historical IDs cannot silently enter Blog's new identity contract."""
    target, fake = blog_server
    operation, user = OperationId(operation_id), BlogUserId("admin")
    commands = (
        (
            "create_article",
            CreateArticle(identity(), user, operation, 1, 3, "Title", "slug", "digest", "content"),
        ),
        ("update_article", UpdateArticle(identity(), user, operation, ArticleId(1), 1)),
        ("publish_article", PublishArticle(identity(), user, operation, ArticleId(1), 1)),
        ("archive_article", ArchiveArticle(identity(), user, operation, ArticleId(1), 1)),
    )
    async with BlogGrpcClient(target, 1.0) as client:
        for name, command in commands:
            assert await client.invoke(name, "v1", command, dispatch=dispatch_call) == BlogFailure(
                FailureKind.INVALID_ARGUMENT,
                retryable=False,
            )
    assert fake.calls == []
