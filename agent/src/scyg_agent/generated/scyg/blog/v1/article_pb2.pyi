from scyg_agent.generated.buf.validate import validate_pb2 as _validate_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from scyg_agent.generated.scyg.blog.v1 import common_pb2 as _common_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Tag(_message.Message):
    __slots__ = ("id", "name")
    ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    id: _common_pb2.TagId
    name: str
    def __init__(self, id: _Optional[_Union[_common_pb2.TagId, _Mapping]] = ..., name: _Optional[str] = ...) -> None: ...

class Article(_message.Message):
    __slots__ = ("id", "title", "body_markdown", "summary", "status", "tags", "version", "created_at", "updated_at", "published_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    BODY_MARKDOWN_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    PUBLISHED_AT_FIELD_NUMBER: _ClassVar[int]
    id: _common_pb2.ArticleId
    title: str
    body_markdown: str
    summary: str
    status: _common_pb2.ArticleStatus
    tags: _containers.RepeatedCompositeFieldContainer[Tag]
    version: int
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    published_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[_Union[_common_pb2.ArticleId, _Mapping]] = ..., title: _Optional[str] = ..., body_markdown: _Optional[str] = ..., summary: _Optional[str] = ..., status: _Optional[_Union[_common_pb2.ArticleStatus, str]] = ..., tags: _Optional[_Iterable[_Union[Tag, _Mapping]]] = ..., version: _Optional[int] = ..., created_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., published_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ArticleSearchHit(_message.Message):
    __slots__ = ("id", "title", "summary", "updated_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    TITLE_FIELD_NUMBER: _ClassVar[int]
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    id: _common_pb2.ArticleId
    title: str
    summary: str
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[_Union[_common_pb2.ArticleId, _Mapping]] = ..., title: _Optional[str] = ..., summary: _Optional[str] = ..., updated_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...
