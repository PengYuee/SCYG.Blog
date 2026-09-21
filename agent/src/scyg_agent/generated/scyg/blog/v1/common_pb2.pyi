from scyg_agent.generated.buf.validate import validate_pb2 as _validate_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ArticleStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ARTICLE_STATUS_UNSPECIFIED: _ClassVar[ArticleStatus]
    ARTICLE_STATUS_DRAFT: _ClassVar[ArticleStatus]
    ARTICLE_STATUS_PUBLISHED: _ClassVar[ArticleStatus]

class ErrorCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ERROR_CODE_UNSPECIFIED: _ClassVar[ErrorCode]
    ERROR_CODE_VALIDATION: _ClassVar[ErrorCode]
    ERROR_CODE_PERMISSION_DENIED: _ClassVar[ErrorCode]
    ERROR_CODE_NOT_FOUND: _ClassVar[ErrorCode]
    ERROR_CODE_ALREADY_EXISTS: _ClassVar[ErrorCode]
    ERROR_CODE_FAILED_PRECONDITION: _ClassVar[ErrorCode]
    ERROR_CODE_VERSION_REQUIRED: _ClassVar[ErrorCode]
    ERROR_CODE_STALE_VERSION: _ClassVar[ErrorCode]
    ERROR_CODE_INTERNAL: _ClassVar[ErrorCode]
ARTICLE_STATUS_UNSPECIFIED: ArticleStatus
ARTICLE_STATUS_DRAFT: ArticleStatus
ARTICLE_STATUS_PUBLISHED: ArticleStatus
ERROR_CODE_UNSPECIFIED: ErrorCode
ERROR_CODE_VALIDATION: ErrorCode
ERROR_CODE_PERMISSION_DENIED: ErrorCode
ERROR_CODE_NOT_FOUND: ErrorCode
ERROR_CODE_ALREADY_EXISTS: ErrorCode
ERROR_CODE_FAILED_PRECONDITION: ErrorCode
ERROR_CODE_VERSION_REQUIRED: ErrorCode
ERROR_CODE_STALE_VERSION: ErrorCode
ERROR_CODE_INTERNAL: ErrorCode

class ToolRequestMetadata(_message.Message):
    __slots__ = ("request_id", "correlation_id", "causation_id", "run_id", "tool_call_id")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    CORRELATION_ID_FIELD_NUMBER: _ClassVar[int]
    CAUSATION_ID_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    TOOL_CALL_ID_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    correlation_id: str
    causation_id: str
    run_id: str
    tool_call_id: str
    def __init__(self, request_id: _Optional[str] = ..., correlation_id: _Optional[str] = ..., causation_id: _Optional[str] = ..., run_id: _Optional[str] = ..., tool_call_id: _Optional[str] = ...) -> None: ...

class WriteOperationMetadata(_message.Message):
    __slots__ = ("request", "operation_id")
    REQUEST_FIELD_NUMBER: _ClassVar[int]
    OPERATION_ID_FIELD_NUMBER: _ClassVar[int]
    request: ToolRequestMetadata
    operation_id: str
    def __init__(self, request: _Optional[_Union[ToolRequestMetadata, _Mapping]] = ..., operation_id: _Optional[str] = ...) -> None: ...

class ArticleId(_message.Message):
    __slots__ = ("value",)
    VALUE_FIELD_NUMBER: _ClassVar[int]
    value: str
    def __init__(self, value: _Optional[str] = ...) -> None: ...

class TagId(_message.Message):
    __slots__ = ("value",)
    VALUE_FIELD_NUMBER: _ClassVar[int]
    value: str
    def __init__(self, value: _Optional[str] = ...) -> None: ...

class UserId(_message.Message):
    __slots__ = ("value",)
    VALUE_FIELD_NUMBER: _ClassVar[int]
    value: str
    def __init__(self, value: _Optional[str] = ...) -> None: ...

class FieldViolation(_message.Message):
    __slots__ = ("field", "description")
    FIELD_FIELD_NUMBER: _ClassVar[int]
    DESCRIPTION_FIELD_NUMBER: _ClassVar[int]
    field: str
    description: str
    def __init__(self, field: _Optional[str] = ..., description: _Optional[str] = ...) -> None: ...

class ErrorDetail(_message.Message):
    __slots__ = ("code", "retryable", "violations")
    CODE_FIELD_NUMBER: _ClassVar[int]
    RETRYABLE_FIELD_NUMBER: _ClassVar[int]
    VIOLATIONS_FIELD_NUMBER: _ClassVar[int]
    code: ErrorCode
    retryable: bool
    violations: _containers.RepeatedCompositeFieldContainer[FieldViolation]
    def __init__(self, code: _Optional[_Union[ErrorCode, str]] = ..., retryable: bool = ..., violations: _Optional[_Iterable[_Union[FieldViolation, _Mapping]]] = ...) -> None: ...
