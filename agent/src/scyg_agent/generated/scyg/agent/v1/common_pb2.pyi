from scyg_agent.generated.buf.validate import validate_pb2 as _validate_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class AgentCapability(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    AGENT_CAPABILITY_UNSPECIFIED: _ClassVar[AgentCapability]
    AGENT_CAPABILITY_SEARCH: _ClassVar[AgentCapability]
    AGENT_CAPABILITY_WRITE: _ClassVar[AgentCapability]
    AGENT_CAPABILITY_POLISH: _ClassVar[AgentCapability]
    AGENT_CAPABILITY_CHAT: _ClassVar[AgentCapability]

class TaskType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    TASK_TYPE_UNSPECIFIED: _ClassVar[TaskType]
    TASK_TYPE_SUMMARY: _ClassVar[TaskType]
    TASK_TYPE_QUESTION: _ClassVar[TaskType]
    TASK_TYPE_POLISH: _ClassVar[TaskType]
    TASK_TYPE_COMPOSE: _ClassVar[TaskType]
    TASK_TYPE_RESEARCH: _ClassVar[TaskType]
    TASK_TYPE_REVISE: _ClassVar[TaskType]

class RuntimeKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RUNTIME_KIND_UNSPECIFIED: _ClassVar[RuntimeKind]
    RUNTIME_KIND_SIMPLE: _ClassVar[RuntimeKind]
    RUNTIME_KIND_DEEP: _ClassVar[RuntimeKind]

class RunStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    RUN_STATUS_UNSPECIFIED: _ClassVar[RunStatus]
    RUN_STATUS_PENDING: _ClassVar[RunStatus]
    RUN_STATUS_RUNNING: _ClassVar[RunStatus]
    RUN_STATUS_WAITING_INPUT: _ClassVar[RunStatus]
    RUN_STATUS_PENDING_RESUME: _ClassVar[RunStatus]
    RUN_STATUS_SUCCEEDED: _ClassVar[RunStatus]
    RUN_STATUS_FAILED: _ClassVar[RunStatus]
    RUN_STATUS_CANCELLED: _ClassVar[RunStatus]

class InteractionType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    INTERACTION_TYPE_UNSPECIFIED: _ClassVar[InteractionType]
    INTERACTION_TYPE_CONFIRMATION: _ClassVar[InteractionType]
    INTERACTION_TYPE_SELECTION: _ClassVar[InteractionType]
    INTERACTION_TYPE_TEXT_INPUT: _ClassVar[InteractionType]

class ErrorCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ERROR_CODE_UNSPECIFIED: _ClassVar[ErrorCode]
    ERROR_CODE_VALIDATION: _ClassVar[ErrorCode]
    ERROR_CODE_PERMISSION_DENIED: _ClassVar[ErrorCode]
    ERROR_CODE_NOT_FOUND: _ClassVar[ErrorCode]
    ERROR_CODE_FAILED_PRECONDITION: _ClassVar[ErrorCode]
    ERROR_CODE_CONFLICT: _ClassVar[ErrorCode]
    ERROR_CODE_DEPENDENCY_UNAVAILABLE: _ClassVar[ErrorCode]
    ERROR_CODE_INTERNAL: _ClassVar[ErrorCode]
AGENT_CAPABILITY_UNSPECIFIED: AgentCapability
AGENT_CAPABILITY_SEARCH: AgentCapability
AGENT_CAPABILITY_WRITE: AgentCapability
AGENT_CAPABILITY_POLISH: AgentCapability
AGENT_CAPABILITY_CHAT: AgentCapability
TASK_TYPE_UNSPECIFIED: TaskType
TASK_TYPE_SUMMARY: TaskType
TASK_TYPE_QUESTION: TaskType
TASK_TYPE_POLISH: TaskType
TASK_TYPE_COMPOSE: TaskType
TASK_TYPE_RESEARCH: TaskType
TASK_TYPE_REVISE: TaskType
RUNTIME_KIND_UNSPECIFIED: RuntimeKind
RUNTIME_KIND_SIMPLE: RuntimeKind
RUNTIME_KIND_DEEP: RuntimeKind
RUN_STATUS_UNSPECIFIED: RunStatus
RUN_STATUS_PENDING: RunStatus
RUN_STATUS_RUNNING: RunStatus
RUN_STATUS_WAITING_INPUT: RunStatus
RUN_STATUS_PENDING_RESUME: RunStatus
RUN_STATUS_SUCCEEDED: RunStatus
RUN_STATUS_FAILED: RunStatus
RUN_STATUS_CANCELLED: RunStatus
INTERACTION_TYPE_UNSPECIFIED: InteractionType
INTERACTION_TYPE_CONFIRMATION: InteractionType
INTERACTION_TYPE_SELECTION: InteractionType
INTERACTION_TYPE_TEXT_INPUT: InteractionType
ERROR_CODE_UNSPECIFIED: ErrorCode
ERROR_CODE_VALIDATION: ErrorCode
ERROR_CODE_PERMISSION_DENIED: ErrorCode
ERROR_CODE_NOT_FOUND: ErrorCode
ERROR_CODE_FAILED_PRECONDITION: ErrorCode
ERROR_CODE_CONFLICT: ErrorCode
ERROR_CODE_DEPENDENCY_UNAVAILABLE: ErrorCode
ERROR_CODE_INTERNAL: ErrorCode

class RequestMetadata(_message.Message):
    __slots__ = ("request_id", "correlation_id", "causation_id")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    CORRELATION_ID_FIELD_NUMBER: _ClassVar[int]
    CAUSATION_ID_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    correlation_id: str
    causation_id: str
    def __init__(self, request_id: _Optional[str] = ..., correlation_id: _Optional[str] = ..., causation_id: _Optional[str] = ...) -> None: ...

class RunId(_message.Message):
    __slots__ = ("value",)
    VALUE_FIELD_NUMBER: _ClassVar[int]
    value: str
    def __init__(self, value: _Optional[str] = ...) -> None: ...

class UserId(_message.Message):
    __slots__ = ("value",)
    VALUE_FIELD_NUMBER: _ClassVar[int]
    value: str
    def __init__(self, value: _Optional[str] = ...) -> None: ...

class ArticleId(_message.Message):
    __slots__ = ("value",)
    VALUE_FIELD_NUMBER: _ClassVar[int]
    value: str
    def __init__(self, value: _Optional[str] = ...) -> None: ...

class SearchInput(_message.Message):
    __slots__ = ("query", "max_results")
    QUERY_FIELD_NUMBER: _ClassVar[int]
    MAX_RESULTS_FIELD_NUMBER: _ClassVar[int]
    query: str
    max_results: int
    def __init__(self, query: _Optional[str] = ..., max_results: _Optional[int] = ...) -> None: ...

class WritingInput(_message.Message):
    __slots__ = ("topic", "requirements", "reference_article_ids")
    TOPIC_FIELD_NUMBER: _ClassVar[int]
    REQUIREMENTS_FIELD_NUMBER: _ClassVar[int]
    REFERENCE_ARTICLE_IDS_FIELD_NUMBER: _ClassVar[int]
    topic: str
    requirements: str
    reference_article_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, topic: _Optional[str] = ..., requirements: _Optional[str] = ..., reference_article_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class PolishInput(_message.Message):
    __slots__ = ("content", "requirements")
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    REQUIREMENTS_FIELD_NUMBER: _ClassVar[int]
    content: str
    requirements: str
    def __init__(self, content: _Optional[str] = ..., requirements: _Optional[str] = ...) -> None: ...

class ChatInput(_message.Message):
    __slots__ = ("message",)
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    message: str
    def __init__(self, message: _Optional[str] = ...) -> None: ...

class RuntimeSelection(_message.Message):
    __slots__ = ("kind", "version")
    KIND_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    kind: RuntimeKind
    version: str
    def __init__(self, kind: _Optional[_Union[RuntimeKind, str]] = ..., version: _Optional[str] = ...) -> None: ...

class ErrorDetail(_message.Message):
    __slots__ = ("code", "message", "retryable")
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    RETRYABLE_FIELD_NUMBER: _ClassVar[int]
    code: ErrorCode
    message: str
    retryable: bool
    def __init__(self, code: _Optional[_Union[ErrorCode, str]] = ..., message: _Optional[str] = ..., retryable: bool = ...) -> None: ...

class Run(_message.Message):
    __slots__ = ("id", "owner_user_id", "task_type", "runtime", "status", "article_id", "created_at", "updated_at", "revision", "failure")
    ID_FIELD_NUMBER: _ClassVar[int]
    OWNER_USER_ID_FIELD_NUMBER: _ClassVar[int]
    TASK_TYPE_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    REVISION_FIELD_NUMBER: _ClassVar[int]
    FAILURE_FIELD_NUMBER: _ClassVar[int]
    id: RunId
    owner_user_id: UserId
    task_type: TaskType
    runtime: RuntimeSelection
    status: RunStatus
    article_id: ArticleId
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    revision: int
    failure: ErrorDetail
    def __init__(self, id: _Optional[_Union[RunId, _Mapping]] = ..., owner_user_id: _Optional[_Union[UserId, _Mapping]] = ..., task_type: _Optional[_Union[TaskType, str]] = ..., runtime: _Optional[_Union[RuntimeSelection, _Mapping]] = ..., status: _Optional[_Union[RunStatus, str]] = ..., article_id: _Optional[_Union[ArticleId, _Mapping]] = ..., created_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[_timestamp_pb2.Timestamp, _Mapping]] = ..., revision: _Optional[int] = ..., failure: _Optional[_Union[ErrorDetail, _Mapping]] = ...) -> None: ...
