from scyg_agent.generated.buf.validate import validate_pb2 as _validate_pb2
from scyg_agent.generated.scyg.agent.v1 import common_pb2 as _common_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class CreateRunRequest(_message.Message):
    __slots__ = ("metadata", "operation_id", "owner_user_id", "task_type", "runtime", "initial_message", "article_id")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    OPERATION_ID_FIELD_NUMBER: _ClassVar[int]
    OWNER_USER_ID_FIELD_NUMBER: _ClassVar[int]
    TASK_TYPE_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_FIELD_NUMBER: _ClassVar[int]
    INITIAL_MESSAGE_FIELD_NUMBER: _ClassVar[int]
    ARTICLE_ID_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.RequestMetadata
    operation_id: str
    owner_user_id: _common_pb2.UserId
    task_type: _common_pb2.TaskType
    runtime: _common_pb2.RuntimeSelection
    initial_message: str
    article_id: _common_pb2.ArticleId
    def __init__(self, metadata: _Optional[_Union[_common_pb2.RequestMetadata, _Mapping]] = ..., operation_id: _Optional[str] = ..., owner_user_id: _Optional[_Union[_common_pb2.UserId, _Mapping]] = ..., task_type: _Optional[_Union[_common_pb2.TaskType, str]] = ..., runtime: _Optional[_Union[_common_pb2.RuntimeSelection, _Mapping]] = ..., initial_message: _Optional[str] = ..., article_id: _Optional[_Union[_common_pb2.ArticleId, _Mapping]] = ...) -> None: ...

class CreateRunResponse(_message.Message):
    __slots__ = ("run",)
    RUN_FIELD_NUMBER: _ClassVar[int]
    run: _common_pb2.Run
    def __init__(self, run: _Optional[_Union[_common_pb2.Run, _Mapping]] = ...) -> None: ...

class CreateAgentRunRequest(_message.Message):
    __slots__ = ("metadata", "idempotency_key", "user_id", "capability", "locale", "search", "writing", "polish", "chat")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    IDEMPOTENCY_KEY_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    CAPABILITY_FIELD_NUMBER: _ClassVar[int]
    LOCALE_FIELD_NUMBER: _ClassVar[int]
    SEARCH_FIELD_NUMBER: _ClassVar[int]
    WRITING_FIELD_NUMBER: _ClassVar[int]
    POLISH_FIELD_NUMBER: _ClassVar[int]
    CHAT_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.RequestMetadata
    idempotency_key: str
    user_id: _common_pb2.UserId
    capability: _common_pb2.AgentCapability
    locale: str
    search: _common_pb2.SearchInput
    writing: _common_pb2.WritingInput
    polish: _common_pb2.PolishInput
    chat: _common_pb2.ChatInput
    def __init__(self, metadata: _Optional[_Union[_common_pb2.RequestMetadata, _Mapping]] = ..., idempotency_key: _Optional[str] = ..., user_id: _Optional[_Union[_common_pb2.UserId, _Mapping]] = ..., capability: _Optional[_Union[_common_pb2.AgentCapability, str]] = ..., locale: _Optional[str] = ..., search: _Optional[_Union[_common_pb2.SearchInput, _Mapping]] = ..., writing: _Optional[_Union[_common_pb2.WritingInput, _Mapping]] = ..., polish: _Optional[_Union[_common_pb2.PolishInput, _Mapping]] = ..., chat: _Optional[_Union[_common_pb2.ChatInput, _Mapping]] = ...) -> None: ...

class CreateAgentRunResponse(_message.Message):
    __slots__ = ("run",)
    RUN_FIELD_NUMBER: _ClassVar[int]
    run: _common_pb2.Run
    def __init__(self, run: _Optional[_Union[_common_pb2.Run, _Mapping]] = ...) -> None: ...

class GetRunRequest(_message.Message):
    __slots__ = ("metadata", "run_id", "owner_user_id")
    METADATA_FIELD_NUMBER: _ClassVar[int]
    RUN_ID_FIELD_NUMBER: _ClassVar[int]
    OWNER_USER_ID_FIELD_NUMBER: _ClassVar[int]
    metadata: _common_pb2.RequestMetadata
    run_id: _common_pb2.RunId
    owner_user_id: _common_pb2.UserId
    def __init__(self, metadata: _Optional[_Union[_common_pb2.RequestMetadata, _Mapping]] = ..., run_id: _Optional[_Union[_common_pb2.RunId, _Mapping]] = ..., owner_user_id: _Optional[_Union[_common_pb2.UserId, _Mapping]] = ...) -> None: ...

class GetRunResponse(_message.Message):
    __slots__ = ("run",)
    RUN_FIELD_NUMBER: _ClassVar[int]
    run: _common_pb2.Run
    def __init__(self, run: _Optional[_Union[_common_pb2.Run, _Mapping]] = ...) -> None: ...
