"""AgentControl protobuf 与领域类型的严格双向转换."""

import hashlib
import re
from typing import Final, cast

from google.protobuf.timestamp_pb2 import Timestamp

from scyg_agent.agents.contracts import (
    Capability,
    CapabilityInput,
    ChatInput,
    PolishInput,
    SearchInput,
    WritingInput,
    validate_capability_input,
)
from scyg_agent.domain.runs import (
    OperationId,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import common_pb2

MAX_INITIAL_MESSAGE_CHARACTERS: Final = 16_000
MAX_ARTICLE_ID_CHARACTERS: Final = 128
METADATA_ID_PATTERN: Final = re.compile(r"[A-Za-z0-9][A-Za-z0-9._:-]{0,127}")
MAX_LOCALE_CHARACTERS: Final = 32
TASK_TYPES: Final = {
    common_pb2.TASK_TYPE_SUMMARY: TaskType.SUMMARY,
    common_pb2.TASK_TYPE_QUESTION: TaskType.QUESTION,
    common_pb2.TASK_TYPE_POLISH: TaskType.POLISH,
    common_pb2.TASK_TYPE_COMPOSE: TaskType.COMPOSE,
    common_pb2.TASK_TYPE_RESEARCH: TaskType.RESEARCH,
    common_pb2.TASK_TYPE_REVISE: TaskType.REVISE,
}
CAPABILITIES: Final = {
    common_pb2.AGENT_CAPABILITY_SEARCH: Capability.SEARCH,
    common_pb2.AGENT_CAPABILITY_WRITE: Capability.WRITE,
    common_pb2.AGENT_CAPABILITY_POLISH: Capability.POLISH,
    common_pb2.AGENT_CAPABILITY_CHAT: Capability.CHAT,
}
RUNTIME_KINDS: Final = {
    common_pb2.RUNTIME_KIND_SIMPLE: RuntimeKind.SIMPLE,
    common_pb2.RUNTIME_KIND_DEEP: RuntimeKind.DEEP,
}
TASK_TYPE_PROTO: Final = {value: key for key, value in TASK_TYPES.items()}
RUNTIME_KIND_PROTO: Final = {value: key for key, value in RUNTIME_KINDS.items()}
RUN_STATUS_PROTO: Final = {
    RunStatus.PENDING: common_pb2.RUN_STATUS_PENDING,
    RunStatus.RUNNING: common_pb2.RUN_STATUS_RUNNING,
    RunStatus.WAITING_INPUT: common_pb2.RUN_STATUS_WAITING_INPUT,
    RunStatus.PENDING_RESUME: common_pb2.RUN_STATUS_PENDING_RESUME,
    RunStatus.SUCCEEDED: common_pb2.RUN_STATUS_SUCCEEDED,
    RunStatus.FAILED: common_pb2.RUN_STATUS_FAILED,
    RunStatus.CANCELLED: common_pb2.RUN_STATUS_CANCELLED,
}


class InvalidGrpcRequestError(ValueError):
    """表示 protobuf 结构存在不允许进入应用层的语义."""


def parse_create_request(
    request: service_pb2.CreateRunRequest,
) -> tuple[UserId, OperationId, RunId, TaskType, RuntimeSelection, str, str]:
    """严格解析创建请求的全部公开业务字段."""
    _validate_metadata(request)
    message = request.initial_message
    article_id = request.article_id.value if request.HasField("article_id") else ""
    if not message or len(message) > MAX_INITIAL_MESSAGE_CHARACTERS:
        raise InvalidGrpcRequestError
    if request.HasField("article_id") and (
        not article_id or len(article_id) > MAX_ARTICLE_ID_CHARACTERS
    ):
        raise InvalidGrpcRequestError
    try:
        return (
            UserId(request.owner_user_id.value),
            OperationId(request.operation_id),
            RunId(_run_id_from_operation(request.operation_id)),
            TASK_TYPES[request.task_type],
            RuntimeSelection(RUNTIME_KINDS[request.runtime.kind], request.runtime.version),
            article_id,
            message,
        )
    except (KeyError, ValueError):
        raise InvalidGrpcRequestError from None


def parse_create_agent_run_request(
    request: service_pb2.CreateAgentRunRequest,
) -> tuple[UserId, OperationId, RunId, Capability, CapabilityInput, str]:
    """Parse the public capability request without accepting server-owned choices."""
    _validate_metadata(request)
    try:
        capability = CAPABILITIES[request.capability]
        input_name = cast("str | None", request.WhichOneof("input"))
        if input_name is None:
            raise InvalidGrpcRequestError
        value = _parse_capability_input(input_name, request)
        _ = validate_capability_input(capability, value)
        user = UserId(request.user_id.value)
        operation = OperationId(request.idempotency_key)
        locale = request.locale
        if not locale or len(locale) > MAX_LOCALE_CHARACTERS:
            raise InvalidGrpcRequestError
        return (
            user,
            operation,
            RunId(_run_id_from_operation(f"{user}:{operation}")),
            capability,
            value,
            locale,
        )
    except (KeyError, ValueError, TypeError):
        raise InvalidGrpcRequestError from None


def _parse_capability_input(
    name: str, request: service_pb2.CreateAgentRunRequest
) -> CapabilityInput:
    """Convert exactly one protobuf input into its strict application contract."""
    if name == "search":
        message = request.search
        return SearchInput(query=message.query, max_results=message.max_results or 5)
    if name == "writing":
        message = request.writing
        return WritingInput(
            topic=message.topic,
            requirements=message.requirements,
            reference_article_ids=tuple(message.reference_article_ids),
        )
    if name == "polish":
        message = request.polish
        return PolishInput(content=message.content, requirements=message.requirements)
    if name == "chat":
        return ChatInput(message=request.chat.message)
    raise InvalidGrpcRequestError


def parse_get_request(request: service_pb2.GetRunRequest) -> tuple[UserId, RunId]:
    """严格解析读取请求的所有权与 Run 标识."""
    _validate_metadata(request)
    try:
        return UserId(request.owner_user_id.value), RunId(request.run_id.value)
    except ValueError:
        raise InvalidGrpcRequestError from None


def run_to_proto(run: Run) -> common_pb2.Run:
    """只序列化稳定快照字段并排除租约、检查点和内部状态."""
    created_at, updated_at = Timestamp(), Timestamp()
    created_at.FromDatetime(run.created_at)
    updated_at.FromDatetime(run.updated_at)
    return common_pb2.Run(
        id=common_pb2.RunId(value=str(run.id)),
        owner_user_id=common_pb2.UserId(value=str(run.owner_user_id)),
        task_type=TASK_TYPE_PROTO[run.task_type],
        runtime=common_pb2.RuntimeSelection(
            kind=RUNTIME_KIND_PROTO[run.runtime.kind], version=run.runtime.version
        ),
        status=RUN_STATUS_PROTO[run.status],
        created_at=created_at,
        updated_at=updated_at,
        revision=run.revision,
    )


def _validate_metadata(
    request: (
        service_pb2.CreateRunRequest | service_pb2.CreateAgentRunRequest | service_pb2.GetRunRequest
    ),
) -> None:
    """要求消息存在且请求链标识遵守发布契约."""
    if not request.HasField("metadata"):
        raise InvalidGrpcRequestError
    metadata = request.metadata
    values = (metadata.request_id, metadata.correlation_id)
    if any(METADATA_ID_PATTERN.fullmatch(value) is None for value in values):
        raise InvalidGrpcRequestError
    if metadata.HasField("causation_id") and (
        METADATA_ID_PATTERN.fullmatch(metadata.causation_id) is None
    ):
        raise InvalidGrpcRequestError


def _run_id_from_operation(operation_id: str) -> str:
    """从幂等操作标识确定性派生规范 Run 标识."""
    digest = hashlib.sha256(operation_id.encode()).hexdigest()[:24]
    return f"run_{digest}"
