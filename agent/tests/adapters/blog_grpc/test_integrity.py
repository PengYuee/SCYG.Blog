"""Blog gRPC 授权和截止期快照完整性测试。"""

from dataclasses import dataclass

import pytest

from scyg_agent.adapters.blog_grpc import (
    AddArticleTags,
    ArticleId,
    ArticleSucceeded,
    BlogFailure,
    BlogGrpcClient,
    BlogUserId,
    CreateArticleDraft,
    FailureKind,
    GetPublishedArticle,
    InvalidAuthorizationError,
    InvalidDeadlineError,
    SearchArticles,
    TagId,
    UpdateArticleDraft,
)
from scyg_agent.domain.runs import OperationId, RuntimeKind, RuntimeSelection
from scyg_agent.runtimes.profiles import (
    COMPOSE_DEEP_V1_PROFILE,
    RESEARCH_DEEP_V1_PROFILE,
    RuntimeProfile,
)

from .support import FakeBlog
from .test_client import all_tools_profile, identity

PROFILE_ATTRIBUTE = "_profile"
AUTHORIZATION_ATTRIBUTE = "_authorization"
SECURITY_ATTRIBUTE = "_security"
DEADLINE_ATTRIBUTE = "_deadline"
DEADLINE_SECONDS_ATTRIBUTE = "_deadline_seconds"


def rebind(client: BlogGrpcClient, attribute: str, value: RuntimeProfile | float) -> None:
    """通过普通 Python 属性写入尝试篡改客户端。"""
    setattr(client, attribute, value)


@dataclass(frozen=True, slots=True)
class RuntimeProfileImpostor(RuntimeProfile):
    """模拟值合法但具体类型不属于闭集的画像。"""


def create_command() -> CreateArticleDraft:
    """构造会暴露授权升级的真实写命令。"""
    return CreateArticleDraft(
        identity(),
        OperationId("integrity-operation"),
        BlogUserId("user-1"),
        "标题",
        "正文",
        "摘要",
    )


def test_profile_impostor_is_rejected_before_channel_creation() -> None:
    impostor = RuntimeProfileImpostor(
        RESEARCH_DEEP_V1_PROFILE.selection,
        RESEARCH_DEEP_V1_PROFILE.capabilities,
        RESEARCH_DEEP_V1_PROFILE.bounds,
        RESEARCH_DEEP_V1_PROFILE.tool_permissions,
    )
    with pytest.raises(InvalidAuthorizationError):
        _ = BlogGrpcClient("127.0.0.1:1", impostor, 1.0)


@pytest.mark.anyio
@pytest.mark.parametrize(
    "replacement",
    [
        COMPOSE_DEEP_V1_PROFILE,
        RuntimeProfileImpostor(
            RESEARCH_DEEP_V1_PROFILE.selection,
            RESEARCH_DEEP_V1_PROFILE.capabilities,
            RESEARCH_DEEP_V1_PROFILE.bounds,
            RESEARCH_DEEP_V1_PROFILE.tool_permissions,
        ),
    ],
)
async def test_profile_replacement_is_drift_before_network(
    blog_server: tuple[str, FakeBlog], replacement: RuntimeProfile
) -> None:
    target, fake = blog_server
    async with BlogGrpcClient(target, RESEARCH_DEEP_V1_PROFILE, 1.0) as client:
        with pytest.raises(TypeError):
            _ = vars(client)
        with pytest.raises(AttributeError):
            rebind(client, PROFILE_ATTRIBUTE, replacement)
        result = await client.invoke("create_article_draft", "v1", create_command())
    assert result == BlogFailure(FailureKind.INVALID_TOOL, retryable=False)
    assert fake.calls == []


@pytest.mark.anyio
async def test_profile_field_mutation_cannot_change_authorization_snapshot(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    profile = RuntimeProfile(
        RuntimeSelection(RuntimeKind.DEEP, "v1"),
        RESEARCH_DEEP_V1_PROFILE.capabilities,
        RESEARCH_DEEP_V1_PROFILE.bounds,
        RESEARCH_DEEP_V1_PROFILE.tool_permissions,
    )
    async with BlogGrpcClient(target, profile, 1.0) as client:
        object.__setattr__(profile, "tool_permissions", COMPOSE_DEEP_V1_PROFILE.tool_permissions)
        result = await client.invoke("create_article_draft", "v1", create_command())
    assert result == BlogFailure(FailureKind.INVALID_TOOL, retryable=False)
    assert fake.calls == []


@pytest.mark.anyio
async def test_coordinated_authorization_replacement_cannot_grant_create(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    async with BlogGrpcClient(target, RESEARCH_DEEP_V1_PROFILE, 1.0) as client:
        with pytest.raises(TypeError):
            _ = vars(client)
        with pytest.raises(AttributeError):
            rebind(client, PROFILE_ATTRIBUTE, COMPOSE_DEEP_V1_PROFILE)
        with pytest.raises(AttributeError):
            rebind(client, AUTHORIZATION_ATTRIBUTE, 2.0)
        result = await client.invoke("create_article_draft", "v1", create_command())
    assert result == BlogFailure(FailureKind.INVALID_TOOL, retryable=False)
    assert fake.calls == []


@pytest.mark.anyio
async def test_security_fields_have_no_dict_and_cannot_be_rebound_or_readded(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, _fake = blog_server
    async with BlogGrpcClient(target, RESEARCH_DEEP_V1_PROFILE, 1.0) as client:
        with pytest.raises(TypeError):
            _ = vars(client)
        with pytest.raises(AttributeError):
            rebind(client, SECURITY_ATTRIBUTE, 2.0)
        with pytest.raises(AttributeError):
            delattr(client, SECURITY_ATTRIBUTE)
        with pytest.raises(AttributeError):
            rebind(client, PROFILE_ATTRIBUTE, COMPOSE_DEEP_V1_PROFILE)


@pytest.mark.parametrize("deadline", [True, False, 0.0, -1.0, float("nan"), float("inf"), 1e308])
def test_invalid_deadline_is_rejected_before_channel_creation(deadline: float) -> None:
    with pytest.raises(InvalidDeadlineError):
        _ = BlogGrpcClient("127.0.0.1:1", RESEARCH_DEEP_V1_PROFILE, deadline)


@pytest.mark.anyio
@pytest.mark.parametrize("replacement", [True, 0.0, -1.0, 2.0, float("nan"), float("inf"), 1e308])
async def test_deadline_replacement_is_drift_before_network(
    blog_server: tuple[str, FakeBlog], replacement: float
) -> None:
    target, fake = blog_server
    async with BlogGrpcClient(target, RESEARCH_DEEP_V1_PROFILE, 1.0) as client:
        with pytest.raises(AttributeError):
            rebind(client, DEADLINE_SECONDS_ATTRIBUTE, replacement)
        result = await client.invoke(
            "get_published_article",
            "v1",
            GetPublishedArticle(identity(), ArticleId("article-1")),
        )
    assert isinstance(result, ArticleSucceeded)
    assert fake.calls == ["get"]


@pytest.mark.anyio
async def test_coordinated_deadline_replacement_cannot_change_all_rpc_timeouts(
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
        with pytest.raises(TypeError):
            _ = vars(client)
        with pytest.raises(AttributeError):
            rebind(client, DEADLINE_ATTRIBUTE, 2.0)
        with pytest.raises(AttributeError):
            rebind(client, DEADLINE_SECONDS_ATTRIBUTE, 2.0)
        for name, command in commands:
            _ = await client.invoke(name, "v1", command)
    assert fake.calls == ["get", "search", "create", "update", "tags"]
    assert all(0.5 < deadline < 1.8 for deadline in fake.deadlines)
