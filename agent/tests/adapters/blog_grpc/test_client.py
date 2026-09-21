"""Blog gRPC 五项操作、允许列表与状态测试。"""

import math

import grpc
import pytest

from scyg_agent.adapters.blog_grpc import (
    AddArticleTags,
    ArticleId,
    ArticleSucceeded,
    BlogFailure,
    BlogGrpcClient,
    BlogUserId,
    CorrelationId,
    CreateArticleDraft,
    FailureKind,
    GetPublishedArticle,
    InvalidDeadlineError,
    RequestId,
    RequestIdentity,
    SearchArticles,
    SearchSucceeded,
    TagId,
    UpdateArticleDraft,
)
from scyg_agent.adapters.blog_grpc.status import map_rpc_status
from scyg_agent.domain.runs import OperationId, RunId, RuntimeKind, RuntimeSelection, ToolCallId
from scyg_agent.runtimes.profiles import (
    COMPOSE_DEEP_V1_PROFILE,
    RuntimeProfile,
    ToolPermission,
)

from .support import FakeBlog


def identity(suffix: str = "1") -> RequestIdentity:
    """构造合法且可区分的调用身份。"""
    return RequestIdentity(
        RequestId(f"request-{suffix}"),
        CorrelationId("correlation-1"),
        RunId("run_abcdefgh"),
        ToolCallId(f"tool_abcdefg{suffix}"),
    )


def all_tools_profile() -> RuntimeProfile:
    """复用 T14 闭集构造覆盖五项已实际授权工具的测试画像。"""
    return RuntimeProfile(
        RuntimeSelection(RuntimeKind.DEEP, "v1"),
        COMPOSE_DEEP_V1_PROFILE.capabilities,
        COMPOSE_DEEP_V1_PROFILE.bounds,
        frozenset(
            {
                ToolPermission.GET_PUBLISHED_ARTICLE,
                ToolPermission.SEARCH_ARTICLES,
                ToolPermission.CREATE_ARTICLE_DRAFT,
                ToolPermission.UPDATE_ARTICLE_DRAFT,
                ToolPermission.ADD_ARTICLE_TAGS,
            }
        ),
    )


@pytest.mark.anyio
async def test_all_five_operations_use_exact_contract_and_finite_deadline(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    commands = (
        ("get_published_article", GetPublishedArticle(identity(), ArticleId("article-1"))),
        ("search_articles", SearchArticles(identity("2"), "查询", 10)),
        (
            "create_article_draft",
            CreateArticleDraft(
                identity("3"), OperationId("op-create"), BlogUserId("user-1"), "题", "文", "摘"
            ),
        ),
        (
            "update_article_draft",
            UpdateArticleDraft(
                identity("4"), OperationId("op-update"), ArticleId("article-1"), 1, "题", "文", "摘"
            ),
        ),
        (
            "add_article_tags",
            AddArticleTags(
                identity("5"),
                OperationId("op-tags"),
                ArticleId("article-1"),
                1,
                (TagId("tag-1"),),
            ),
        ),
    )
    async with BlogGrpcClient(target, all_tools_profile(), 1.5) as client:
        results = [await client.invoke(name, "v1", command) for name, command in commands]
    assert isinstance(results[0], ArticleSucceeded)
    assert isinstance(results[1], SearchSucceeded)
    assert all(isinstance(result, ArticleSucceeded) for result in results[2:])
    assert fake.calls == ["get", "search", "create", "update", "tags"]
    assert fake.operation_ids == ["op-create", "op-update", "op-tags"]
    assert all(0 < deadline <= 2 for deadline in fake.deadlines)
    assert [metadata.request_id for metadata in fake.request_metadata] == [
        "request-1",
        "request-2",
        "request-3",
        "request-4",
        "request-5",
    ]


@pytest.mark.anyio
async def test_duplicate_write_preserves_semantic_operation_identity(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    command = CreateArticleDraft(
        identity(), OperationId("operation-1"), BlogUserId("user-1"), "标题", "正文", "摘要"
    )
    async with BlogGrpcClient(target, COMPOSE_DEEP_V1_PROFILE, 1.0) as client:
        first = await client.invoke("create_article_draft", "v1", command)
        second = await client.invoke("create_article_draft", "v1", command)
    assert first == second
    assert fake.operation_ids == ["operation-1", "operation-1"]


@pytest.mark.anyio
async def test_invalid_selection_and_wrong_variant_fail_before_network(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    command = CreateArticleDraft(
        identity(), OperationId("operation-1"), BlogUserId("user-1"), "标题", "正文", "摘要"
    )
    async with BlogGrpcClient(target, COMPOSE_DEEP_V1_PROFILE, 1.0) as client:
        results = (
            await client.invoke("delete_everything", "v1", command),
            await client.invoke("create_article_draft", "v2", command),
            await client.invoke("get_published_article", "v1", command),
            await client.invoke("publish_article", "v1", command),
        )
    assert results == (
        BlogFailure(FailureKind.INVALID_TOOL, retryable=False),
        BlogFailure(FailureKind.INVALID_VERSION, retryable=False),
        BlogFailure(FailureKind.WRONG_COMMAND, retryable=False),
        BlogFailure(FailureKind.INVALID_TOOL, retryable=False),
    )
    assert fake.calls == []


@pytest.mark.parametrize("deadline", [0, -1, math.inf, math.nan])
def test_deadline_must_be_positive_and_finite(deadline: float) -> None:
    with pytest.raises(InvalidDeadlineError):
        _ = BlogGrpcClient("127.0.0.1:1", COMPOSE_DEEP_V1_PROFILE, deadline)


@pytest.mark.parametrize(
    ("status", "expected"),
    [
        (
            grpc.StatusCode.INVALID_ARGUMENT,
            BlogFailure(FailureKind.INVALID_ARGUMENT, retryable=False),
        ),
        (grpc.StatusCode.NOT_FOUND, BlogFailure(FailureKind.NOT_FOUND, retryable=False)),
        (grpc.StatusCode.ALREADY_EXISTS, BlogFailure(FailureKind.DUPLICATE, retryable=False)),
        (
            grpc.StatusCode.FAILED_PRECONDITION,
            BlogFailure(FailureKind.FAILED_PRECONDITION, retryable=False),
        ),
        (grpc.StatusCode.ABORTED, BlogFailure(FailureKind.FAILED_PRECONDITION, retryable=False)),
        (
            grpc.StatusCode.PERMISSION_DENIED,
            BlogFailure(FailureKind.AUTHORIZATION, retryable=False),
        ),
        (grpc.StatusCode.UNAUTHENTICATED, BlogFailure(FailureKind.AUTHORIZATION, retryable=False)),
        (
            grpc.StatusCode.RESOURCE_EXHAUSTED,
            BlogFailure(FailureKind.RESOURCE_EXHAUSTED, retryable=True),
        ),
        (grpc.StatusCode.UNAVAILABLE, BlogFailure(FailureKind.UNAVAILABLE, retryable=True)),
        (
            grpc.StatusCode.DEADLINE_EXCEEDED,
            BlogFailure(FailureKind.DEADLINE_EXCEEDED, retryable=True),
        ),
        (grpc.StatusCode.CANCELLED, BlogFailure(FailureKind.CANCELLED, retryable=False)),
        (grpc.StatusCode.UNKNOWN, BlogFailure(FailureKind.INTERNAL, retryable=False)),
        (grpc.StatusCode.UNIMPLEMENTED, BlogFailure(FailureKind.INTERNAL, retryable=False)),
        (grpc.StatusCode.INTERNAL, BlogFailure(FailureKind.INTERNAL, retryable=False)),
        (grpc.StatusCode.DATA_LOSS, BlogFailure(FailureKind.INTERNAL, retryable=False)),
        (grpc.StatusCode.OUT_OF_RANGE, BlogFailure(FailureKind.INTERNAL, retryable=False)),
        (grpc.StatusCode.OK, BlogFailure(FailureKind.INTERNAL, retryable=False)),
    ],
)
def test_status_mapping_is_closed_and_sanitized(
    status: grpc.StatusCode,
    expected: BlogFailure,
) -> None:
    assert map_rpc_status(status) == expected
