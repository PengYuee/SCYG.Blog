from scyg_agent.generated.buf.validate import validate_pb2 as _validate_pb2
from scyg_agent.generated.scyg.blog.v1 import article_pb2 as _article_pb2
from scyg_agent.generated.scyg.blog.v1 import common_pb2 as _common_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class GetPublishedArticleRequest(_message.Message):
    __slots__ = ("metadata", "article_id")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.ToolRequestMetadata
    article_id: _common_pb2.ArticleId
    def __init__(self, metadata: _Optional[_Union[_common_pb2.ToolRequestMetadata, _Mapping]] = ..., article_id: _Optional[_Union[_common_pb2.ArticleId, _Mapping]] = ...) -> None: ...

class GetPublishedArticleResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: _article_pb2.Article
    def __init__(self, article: _Optional[_Union[_article_pb2.Article, _Mapping]] = ...) -> None: ...

class SearchArticlesRequest(_message.Message):
    __slots__ = ("metadata", "query", "page_size", "page_token")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    QUERY_FIELD_NUMBER: _ClassVar[int]
    PAGE_SIZE_FIELD_NUMBER: _ClassVar[int]
    PAGE_TOKEN_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.ToolRequestMetadata
    query: str
    page_size: int
    page_token: str
    def __init__(self, metadata: _Optional[_Union[_common_pb2.ToolRequestMetadata, _Mapping]] = ..., query: _Optional[str] = ..., page_size: _Optional[int] = ..., page_token: _Optional[str] = ...) -> None: ...

class SearchArticlesResponse(_message.Message):
    __slots__ = ("articles", "next_page_token")
    ARTICLES_FIELD_NUMBER: _ClassVar[int]
    NEXT_PAGE_TOKEN_FIELD_NUMBER: _ClassVar[int]
    articles: _containers.RepeatedCompositeFieldContainer[_article_pb2.ArticleSearchHit]
    next_page_token: str
    def __init__(self, articles: _Optional[_Iterable[_Union[_article_pb2.ArticleSearchHit, _Mapping]]] = ..., next_page_token: _Optional[str] = ...) -> None: ...

class CreateArticleDraftRequest(_message.Message):
    __slots__ = ("metadata", "author_user_id", "title", "body_markdown", "summary")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    AUTHOR_USER_ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    BODY_MARKDOWN_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.WriteOperationMetadata
    author_user_id: _common_pb2.UserId
    title: str
    body_markdown: str
    summary: str
    def __init__(self, metadata: _Optional[_Union[_common_pb2.WriteOperationMetadata, _Mapping]] = ..., author_user_id: _Optional[_Union[_common_pb2.UserId, _Mapping]] = ..., title: _Optional[str] = ..., body_markdown: _Optional[str] = ..., summary: _Optional[str] = ...) -> None: ...

class CreateArticleDraftResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: _article_pb2.Article
    def __init__(self, article: _Optional[_Union[_article_pb2.Article, _Mapping]] = ...) -> None: ...

class UpdateArticleDraftRequest(_message.Message):
    __slots__ = ("metadata", "article_id", "expected_version", "title", "body_markdown", "summary")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_VERSION_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    BODY_MARKDOWN_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.WriteOperationMetadata
    article_id: _common_pb2.ArticleId
    expected_version: int
    title: str
    body_markdown: str
    summary: str
    def __init__(self, metadata: _Optional[_Union[_common_pb2.WriteOperationMetadata, _Mapping]] = ..., article_id: _Optional[_Union[_common_pb2.ArticleId, _Mapping]] = ..., expected_version: _Optional[int] = ..., title: _Optional[str] = ..., body_markdown: _Optional[str] = ..., summary: _Optional[str] = ...) -> None: ...

class UpdateArticleDraftResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: _article_pb2.Article
    def __init__(self, article: _Optional[_Union[_article_pb2.Article, _Mapping]] = ...) -> None: ...

class PublishArticleRequest(_message.Message):
    __slots__ = ("metadata", "article_id", "expected_version")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_VERSION_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.WriteOperationMetadata
    article_id: _common_pb2.ArticleId
    expected_version: int
    def __init__(self, metadata: _Optional[_Union[_common_pb2.WriteOperationMetadata, _Mapping]] = ..., article_id: _Optional[_Union[_common_pb2.ArticleId, _Mapping]] = ..., expected_version: _Optional[int] = ...) -> None: ...

class PublishArticleResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: _article_pb2.Article
    def __init__(self, article: _Optional[_Union[_article_pb2.Article, _Mapping]] = ...) -> None: ...

class AddArticleTagsRequest(_message.Message):
    __slots__ = ("metadata", "article_id", "expected_version", "tag_ids")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_VERSION_FIELD_NUMBER: _ClassVar[int]
    TAG_IDS_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.WriteOperationMetadata
    article_id: _common_pb2.ArticleId
    expected_version: int
    tag_ids: _containers.RepeatedCompositeFieldContainer[_common_pb2.TagId]
    def __init__(self, metadata: _Optional[_Union[_common_pb2.WriteOperationMetadata, _Mapping]] = ..., article_id: _Optional[_Union[_common_pb2.ArticleId, _Mapping]] = ..., expected_version: _Optional[int] = ..., tag_ids: _Optional[_Iterable[_Union[_common_pb2.TagId, _Mapping]]] = ...) -> None: ...

class AddArticleTagsResponse(_message.Message):
    __slots__ = ("article",)
    ARTICLE_FIELD_NUMBER: _ClassVar[int]
    article: _article_pb2.Article
    def __init__(self, article: _Optional[_Union[_article_pb2.Article, _Mapping]] = ...) -> None: ...
