"""Blog gRPC 边界的冻结命令与封闭结果。."""

from dataclasses import dataclass
from enum import StrEnum
from typing import NewType

from scyg_agent.domain.runs import OperationId, RunId, ToolCallId

RequestId = NewType("RequestId", str)
CorrelationId = NewType("CorrelationId", str)
ArticleId = NewType("ArticleId", str)
TagId = NewType("TagId", str)
BlogUserId = NewType("BlogUserId", str)


@dataclass(frozen=True, slots=True)
class RequestIdentity:
    """携带一次工具调用的审计与追踪身份。."""

    request_id: RequestId
    correlation_id: CorrelationId
    run_id: RunId
    tool_call_id: ToolCallId
    causation_id: str | None = None


@dataclass(frozen=True, slots=True)
class GetPublishedArticle:
    """读取一个已发布文章。."""

    identity: RequestIdentity
    article_id: ArticleId


@dataclass(frozen=True, slots=True)
class SearchArticles:
    """搜索一个有界文章页。."""

    identity: RequestIdentity
    query: str
    page_size: int
    page_token: str | None = None


@dataclass(frozen=True, slots=True)
class CreateArticleDraft:
    """以稳定操作身份创建草稿。."""

    identity: RequestIdentity
    operation_id: OperationId
    author_user_id: BlogUserId
    title: str
    body_markdown: str
    summary: str


@dataclass(frozen=True, slots=True)
class UpdateArticleDraft:
    """以稳定操作身份和预期版本更新草稿。."""

    identity: RequestIdentity
    operation_id: OperationId
    article_id: ArticleId
    expected_version: int
    title: str
    body_markdown: str
    summary: str


@dataclass(frozen=True, slots=True)
class AddArticleTags:
    """以稳定操作身份添加既有标签。."""

    identity: RequestIdentity
    operation_id: OperationId
    article_id: ArticleId
    expected_version: int
    tag_ids: tuple[TagId, ...]


type BlogCommand = (
    GetPublishedArticle | SearchArticles | CreateArticleDraft | UpdateArticleDraft | AddArticleTags
)


@dataclass(frozen=True, slots=True)
class ArticleResult:
    """返回经边界解析的文章标量投影。."""

    article_id: ArticleId
    title: str
    body_markdown: str
    summary: str
    status: int
    version: int


@dataclass(frozen=True, slots=True)
class SearchHit:
    """返回经边界解析的搜索命中。."""

    article_id: ArticleId
    title: str
    summary: str


@dataclass(frozen=True, slots=True)
class ArticleSucceeded:
    """表示文章操作成功。."""

    article: ArticleResult


@dataclass(frozen=True, slots=True)
class SearchSucceeded:
    """表示搜索成功。."""

    articles: tuple[SearchHit, ...]
    next_page_token: str | None


class FailureKind(StrEnum):
    """关闭所有允许暴露给上层的 gRPC 失败类别。."""

    INVALID_TOOL = "invalid_tool"
    INVALID_VERSION = "invalid_version"
    WRONG_COMMAND = "wrong_command"
    INVALID_ARGUMENT = "invalid_argument"
    NOT_FOUND = "not_found"
    DUPLICATE = "duplicate"
    FAILED_PRECONDITION = "failed_precondition"
    AUTHORIZATION = "authorization"
    RESOURCE_EXHAUSTED = "resource_exhausted"
    UNAVAILABLE = "unavailable"
    DEADLINE_EXCEEDED = "deadline_exceeded"
    CANCELLED = "cancelled"
    INTERNAL = "internal"
    INVALID_RESPONSE = "invalid_response"
    CHANNEL_CLOSED = "channel_closed"
    SECURITY_DRIFT = "security_drift"


@dataclass(frozen=True, slots=True)
class BlogFailure:
    """返回无服务端详情、无请求内容的稳定失败。."""

    kind: FailureKind
    retryable: bool


type BlogResult = ArticleSucceeded | SearchSucceeded | BlogFailure
