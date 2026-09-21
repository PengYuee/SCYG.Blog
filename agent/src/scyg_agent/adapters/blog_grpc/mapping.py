"""将外部 protobuf 响应解析为冻结结果。."""

from scyg_agent.generated.scyg.blog.v1 import blog_tool_service_pb2 as service_pb2
from scyg_agent.generated.scyg.blog.v1 import common_pb2

from .contracts import (
    ArticleId,
    ArticleResult,
    ArticleSucceeded,
    BlogFailure,
    BlogResult,
    FailureKind,
    SearchHit,
    SearchSucceeded,
)

type ArticleResponse = (
    service_pb2.GetPublishedArticleResponse
    | service_pb2.CreateArticleDraftResponse
    | service_pb2.UpdateArticleDraftResponse
    | service_pb2.AddArticleTagsResponse
)


def article_response(response: ArticleResponse) -> BlogResult:
    """拒绝缺失必需字段或无效状态的文章响应。."""
    if not response.HasField("article"):
        return _invalid_response()
    article = response.article
    if (
        not article.HasField("id")
        or not article.id.value
        or article.status == common_pb2.ARTICLE_STATUS_UNSPECIFIED
        or article.version < 1
    ):
        return _invalid_response()
    return ArticleSucceeded(
        ArticleResult(
            ArticleId(article.id.value),
            article.title,
            article.body_markdown,
            article.summary,
            article.status,
            article.version,
        )
    )


def search_response(response: service_pb2.SearchArticlesResponse) -> BlogResult:
    """拒绝含缺失身份字段的搜索响应。."""
    if any(not hit.HasField("id") or not hit.id.value for hit in response.articles):
        return _invalid_response()
    return SearchSucceeded(
        tuple(
            SearchHit(ArticleId(hit.id.value), hit.title, hit.summary) for hit in response.articles
        ),
        response.next_page_token if response.HasField("next_page_token") else None,
    )


def _invalid_response() -> BlogFailure:
    """构造稳定且不包含 protobuf 内容的协议失败。."""
    return BlogFailure(FailureKind.INVALID_RESPONSE, retryable=False)
