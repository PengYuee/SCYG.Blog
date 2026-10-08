"""Frozen BlogContent commands and complete management projections."""

from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from enum import StrEnum
from typing import NewType

from scyg_agent.domain.runs import OperationId, RunId, ToolCallId

RequestId = NewType("RequestId", str)
CorrelationId = NewType("CorrelationId", str)
ArticleId = NewType("ArticleId", int)
TagId = NewType("TagId", int)
BlogUserId = NewType("BlogUserId", str)
type RpcDispatch = Callable[[Callable[[], Awaitable[object]]], Awaitable[object]]


@dataclass(frozen=True, slots=True)
class RequestIdentity:
    """Internal tracing identity; never forwarded as arbitrary metadata."""

    request_id: RequestId
    correlation_id: CorrelationId
    run_id: RunId
    tool_call_id: ToolCallId
    causation_id: str | None = None


@dataclass(frozen=True, slots=True)
class GetArticle:
    """Read one article through current management authorization."""

    identity: RequestIdentity
    user_id: BlogUserId
    article_id: ArticleId


@dataclass(frozen=True, slots=True)
class SearchArticles:
    """Search management articles using explicit page-based filters."""

    identity: RequestIdentity
    user_id: BlogUserId
    query: str = ""
    page: int = 1
    page_size: int = 20
    status: int = 0
    article_type_id: int = 0
    tag_id: int = 0
    sort: str = ""


@dataclass(frozen=True, slots=True)
class ListTags:
    """List existing management tags with database pagination."""

    identity: RequestIdentity
    user_id: BlogUserId
    page: int = 1
    page_size: int = 20
    query: str = ""
    sort: str = ""


@dataclass(frozen=True, slots=True)
class ListArticleTypes:
    """List existing article classifications with database pagination."""

    identity: RequestIdentity
    user_id: BlogUserId
    page: int = 1
    page_size: int = 20
    query: str = ""
    sort: str = ""


@dataclass(frozen=True, slots=True)
class CreateArticle:
    """Create using a caller-persisted UUIDv4 write intent identity."""

    identity: RequestIdentity
    user_id: BlogUserId
    operation_id: OperationId
    status: int
    article_type_id: int
    title: str
    slug: str
    digest: str
    content: str
    tag_ids: tuple[TagId, ...] = ()


@dataclass(frozen=True, slots=True)
class UpdateArticle:
    """Apply optional patches; None means absent, empty means replacement."""

    identity: RequestIdentity
    user_id: BlogUserId
    operation_id: OperationId
    article_id: ArticleId
    expected_version: int
    article_type_id: int | None = None
    title: str | None = None
    slug: str | None = None
    digest: str | None = None
    content: str | None = None
    tag_ids: tuple[TagId, ...] | None = None


@dataclass(frozen=True, slots=True)
class PublishArticle:
    """Publish under optimistic version checking and a stable UUIDv4."""

    identity: RequestIdentity
    user_id: BlogUserId
    operation_id: OperationId
    article_id: ArticleId
    expected_version: int


@dataclass(frozen=True, slots=True)
class ArchiveArticle:
    """Archive under optimistic version checking and a stable UUIDv4."""

    identity: RequestIdentity
    user_id: BlogUserId
    operation_id: OperationId
    article_id: ArticleId
    expected_version: int


type BlogCommand = (
    GetArticle
    | SearchArticles
    | ListTags
    | ListArticleTypes
    | CreateArticle
    | UpdateArticle
    | PublishArticle
    | ArchiveArticle
)


@dataclass(frozen=True, slots=True)
class TaxonomyResult:
    """Project a numeric taxonomy identity and its current name."""

    id: int
    name: str


@dataclass(frozen=True, slots=True)
class ArticleResult:
    """Preserve the complete management article projection."""

    article_id: ArticleId
    title: str
    content: str
    digest: str
    status: int
    version: int
    article_type_id: int
    slug: str
    tags: tuple[TaxonomyResult, ...]
    created_at: str
    updated_at: str
    published_at: str | None
    support: int
    comment: int
    visited: int


@dataclass(frozen=True, slots=True)
class ArticleSucceeded:
    """Return one successfully decoded article projection."""

    article: ArticleResult


@dataclass(frozen=True, slots=True)
class SearchSucceeded:
    """Return validated article projections and their pagination fields."""

    articles: tuple[ArticleResult, ...]
    page: int
    page_size: int
    total_items: int
    total_pages: int


@dataclass(frozen=True, slots=True)
class TagsSucceeded:
    """Current taxonomy page, not an opaque cursor."""

    tags: tuple[TaxonomyResult, ...]
    page: int
    page_size: int
    total_items: int
    total_pages: int


@dataclass(frozen=True, slots=True)
class ArticleTypesSucceeded:
    """Current classification page, not an opaque cursor."""

    article_types: tuple[TaxonomyResult, ...]
    page: int
    page_size: int
    total_items: int
    total_pages: int


class FailureKind(StrEnum):
    """Classify public client failures without exposing infrastructure details."""

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


@dataclass(frozen=True, slots=True)
class BlogFailure:
    """Carry a sanitized failure and the existing retry decision."""

    kind: FailureKind
    retryable: bool


type BlogResult = (
    ArticleSucceeded | SearchSucceeded | TagsSucceeded | ArticleTypesSucceeded | BlogFailure
)
