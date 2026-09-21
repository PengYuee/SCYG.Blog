"""公开静态允许列表 Blog gRPC 客户端契约。."""

from .client import BlogGrpcClient
from .contracts import (
    AddArticleTags,
    ArticleId,
    ArticleResult,
    ArticleSucceeded,
    BlogFailure,
    BlogResult,
    BlogUserId,
    CorrelationId,
    CreateArticleDraft,
    FailureKind,
    GetPublishedArticle,
    RequestId,
    RequestIdentity,
    SearchArticles,
    SearchHit,
    SearchSucceeded,
    TagId,
    UpdateArticleDraft,
)
from .security import InvalidAuthorizationError, InvalidDeadlineError

__all__ = [
    "AddArticleTags",
    "ArticleId",
    "ArticleResult",
    "ArticleSucceeded",
    "BlogFailure",
    "BlogGrpcClient",
    "BlogResult",
    "BlogUserId",
    "CorrelationId",
    "CreateArticleDraft",
    "FailureKind",
    "GetPublishedArticle",
    "InvalidAuthorizationError",
    "InvalidDeadlineError",
    "RequestId",
    "RequestIdentity",
    "SearchArticles",
    "SearchHit",
    "SearchSucceeded",
    "TagId",
    "UpdateArticleDraft",
]
