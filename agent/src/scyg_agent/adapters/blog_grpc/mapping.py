"""Map complete Blog management projections without leaking raw responses."""

from scyg_agent.generated.proto.scyg.blog.v1 import article_pb2, common_pb2
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2 as service_pb2

from .contracts import (
    ArticleId,
    ArticleResult,
    ArticleSucceeded,
    ArticleTypesSucceeded,
    BlogFailure,
    BlogResult,
    FailureKind,
    SearchSucceeded,
    TagsSucceeded,
    TaxonomyResult,
)

type ArticleResponse = (
    service_pb2.GetArticleResponse
    | service_pb2.CreateArticleResponse
    | service_pb2.UpdateArticleResponse
    | service_pb2.PublishArticleResponse
    | service_pb2.ArchiveArticleResponse
)
MAX_PAGE_SIZE = 100


def _article(article: article_pb2.Article) -> ArticleResult | None:
    if (
        article.id <= 0
        or article.article_type_id <= 0
        or article.version < 1
        or article.status
        not in (
            common_pb2.ARTICLE_STATUS_DRAFT,
            common_pb2.ARTICLE_STATUS_PUBLISHED,
            common_pb2.ARTICLE_STATUS_ARCHIVED,
        )
        or not article.HasField("created_at")
        or not article.HasField("updated_at")
        or any(tag.id <= 0 or not tag.name for tag in article.tags)
    ):
        return None
    try:
        return ArticleResult(
            ArticleId(article.id),
            article.title,
            article.content,
            article.digest,
            article.status,
            article.version,
            article.article_type_id,
            article.slug,
            tuple(TaxonomyResult(tag.id, tag.name) for tag in article.tags),
            article.created_at.ToJsonString(),
            article.updated_at.ToJsonString(),
            article.published_at.ToJsonString() if article.HasField("published_at") else None,
            article.support,
            article.comment,
            article.visited,
        )
    except (ValueError, OverflowError):
        return None


def article_response(response: ArticleResponse) -> BlogResult:
    """Decode one present, valid article from a read or write response."""
    article = _article(response.article) if response.HasField("article") else None
    return ArticleSucceeded(article) if article is not None else _invalid_response()


def _valid_page(page: int, page_size: int, total_items: int) -> bool:
    return page >= 1 and 1 <= page_size <= MAX_PAGE_SIZE and total_items >= 0


def search_response(response: service_pb2.SearchArticlesResponse) -> BlogResult:
    """Decode a bounded article page without dropping its pagination metadata."""
    articles = tuple(_article(article) for article in response.articles)
    if not _valid_page(response.page, response.page_size, response.total_items) or any(
        article is None for article in articles
    ):
        return _invalid_response()
    return SearchSucceeded(
        tuple(article for article in articles if article is not None),
        response.page,
        response.page_size,
        response.total_items,
        response.total_pages,
    )


def tags_response(response: service_pb2.ListTagsResponse) -> BlogResult:
    """Map numeric tag IDs and all pagination fields."""
    if not _valid_page(response.page, response.page_size, response.total_items) or any(
        tag.id <= 0 or not tag.name for tag in response.tags
    ):
        return _invalid_response()
    return TagsSucceeded(
        tuple(TaxonomyResult(tag.id, tag.name) for tag in response.tags),
        response.page,
        response.page_size,
        response.total_items,
        response.total_pages,
    )


def article_types_response(response: service_pb2.ListArticleTypesResponse) -> BlogResult:
    """Map numeric classification IDs and all pagination fields."""
    if not _valid_page(response.page, response.page_size, response.total_items) or any(
        item.id <= 0 or not item.name for item in response.article_types
    ):
        return _invalid_response()
    return ArticleTypesSucceeded(
        tuple(TaxonomyResult(item.id, item.name) for item in response.article_types),
        response.page,
        response.page_size,
        response.total_items,
        response.total_pages,
    )


def _invalid_response() -> BlogFailure:
    return BlogFailure(FailureKind.INVALID_RESPONSE, retryable=False)
