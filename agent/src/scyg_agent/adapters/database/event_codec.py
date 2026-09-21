"""领域事件持久化的严格版本分派编解码边界."""

from dataclasses import dataclass
from datetime import datetime
from typing import Literal, override

from pydantic import ValidationError

from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    ApprovalResolvedEvent,
    CommandId,
    DomainEvent,
    EventId,
    EventKind,
    ExecutionReleased,
    InputRequested,
    InputResolved,
    InteractionId,
    RunCancelled,
    RunFailed,
    RunId,
    RunStatus,
    RunSucceeded,
    StatusChanged,
    TextDelta,
    TokenUsage,
    ToolCallId,
    ToolFailed,
    ToolOutcomeUnknown,
    ToolStarted,
    ToolSucceeded,
)

from .event_payloads import (
    ApprovalPayload,
    EmptyPayload,
    InteractionPayload,
    PersistedPayload,
    StatusPayload,
    TextPayload,
    ToolPayload,
    UsagePayload,
    parse_payload,
)
from .journal_records import EventRecord


@dataclass(frozen=True, slots=True)
class MalformedEventRecordError(ValueError):
    """报告不反射持久化值的稳定 codec 失败."""

    reason: str

    @override
    def __str__(self) -> str:
        return f"事件记录格式无效: {self.reason}"


def serialize_event(  # noqa: C901, PLR0912
    event: DomainEvent,
) -> tuple[EventKind, dict[str, str]]:
    """把精确领域变体编码为唯一严格 v1 载荷."""
    allowed_types = (
        StatusChanged,
        InputRequested,
        InputResolved,
        RunSucceeded,
        RunFailed,
        RunCancelled,
        ExecutionReleased,
        TextDelta,
        TokenUsage,
        ApprovalRequiredEvent,
        ApprovalResolvedEvent,
        ToolStarted,
        ToolSucceeded,
        ToolFailed,
        ToolOutcomeUnknown,
    )
    if type(event) not in allowed_types:
        reason = "事件类型不受支持"
        raise MalformedEventRecordError(reason)
    command_id = str(event.command_id)
    match event:  # noqa: RUF100  # noqa: MATCH_OK - 前置精确类型闭集已完备。
        case StatusChanged(previous=previous, current=current):
            kind, payload = (
                EventKind.STATUS_CHANGED,
                StatusPayload(
                    command_id=command_id, previous=previous.value, current=current.value
                ),
            )
        case InputRequested(interaction_id=value):
            kind, payload = (
                EventKind.INPUT_REQUESTED,
                InteractionPayload(command_id=command_id, interaction_id=str(value)),
            )
        case InputResolved(interaction_id=value):
            kind, payload = (
                EventKind.INPUT_RESOLVED,
                InteractionPayload(command_id=command_id, interaction_id=str(value)),
            )
        case RunSucceeded():
            kind, payload = EventKind.RUN_SUCCEEDED, EmptyPayload(command_id=command_id)
        case RunFailed():
            kind, payload = EventKind.RUN_FAILED, EmptyPayload(command_id=command_id)
        case RunCancelled():
            kind, payload = EventKind.RUN_CANCELLED, EmptyPayload(command_id=command_id)
        case ExecutionReleased():
            kind, payload = EventKind.EXECUTION_RELEASED, EmptyPayload(command_id=command_id)
        case TextDelta(content=content):
            kind, payload = (
                EventKind.TEXT_DELTA,
                TextPayload(command_id=command_id, content=content),
            )
        case TokenUsage(prompt_tokens=prompt, completion_tokens=completion, total_tokens=total):
            kind, payload = (
                EventKind.TOKEN_USAGE,
                UsagePayload(
                    command_id=command_id,
                    prompt_tokens=str(prompt),
                    completion_tokens=str(completion),
                    total_tokens=str(total),
                ),
            )
        case ApprovalRequiredEvent(interaction_id=value):
            kind, payload = (
                EventKind.APPROVAL_REQUIRED,
                InteractionPayload(command_id=command_id, interaction_id=str(value)),
            )
        case ApprovalResolvedEvent(interaction_id=value, approved=approved):
            approved_value: Literal["true", "false"] = "true" if approved else "false"
            kind, payload = (
                EventKind.APPROVAL_RESOLVED,
                ApprovalPayload(
                    command_id=command_id,
                    interaction_id=str(value),
                    approved=approved_value,
                ),
            )
        case ToolStarted(tool_call_id=value):
            kind, payload = (
                EventKind.TOOL_STARTED,
                ToolPayload(command_id=command_id, tool_call_id=str(value)),
            )
        case ToolSucceeded(tool_call_id=value):
            kind, payload = (
                EventKind.TOOL_SUCCEEDED,
                ToolPayload(command_id=command_id, tool_call_id=str(value)),
            )
        case ToolFailed(tool_call_id=value):
            kind, payload = (
                EventKind.TOOL_FAILED,
                ToolPayload(command_id=command_id, tool_call_id=str(value)),
            )
        case ToolOutcomeUnknown(tool_call_id=value):
            kind, payload = (
                EventKind.TOOL_OUTCOME_UNKNOWN,
                ToolPayload(command_id=command_id, tool_call_id=str(value)),
            )
    return kind, payload.model_dump()


def deserialize_event(record: EventRecord) -> DomainEvent:
    """严格解析种类、版本、字段集合与标量类型后构造领域事件."""
    try:
        kind = EventKind(record.kind)
        payload = parse_payload(kind, record.payload)
        common = (
            EventId(record.event_id),
            CommandId(payload.command_id),
            record.occurred_at,
            RunId(record.run_id),
            record.revision,
        )
        return _construct(kind, payload, common)
    except (ValueError, ValidationError):
        reason = "种类、版本或载荷不符合协议"
        raise MalformedEventRecordError(reason) from None


def _construct(  # noqa: C901, PLR0911, PLR0912
    kind: EventKind,
    payload: PersistedPayload,
    common: tuple[EventId, CommandId, datetime, RunId, int],
) -> DomainEvent:
    """按种类和已解析载荷穷尽构造领域事件."""
    match kind:  # noqa: RUF100  # noqa: MATCH_OK - 默认分支拒绝模型错配。
        case EventKind.STATUS_CHANGED if type(payload) is StatusPayload:
            return StatusChanged(*common, RunStatus(payload.previous), RunStatus(payload.current))
        case EventKind.INPUT_REQUESTED if type(payload) is InteractionPayload:
            return InputRequested(*common, InteractionId(payload.interaction_id))
        case EventKind.INPUT_RESOLVED if type(payload) is InteractionPayload:
            return InputResolved(*common, InteractionId(payload.interaction_id))
        case EventKind.RUN_SUCCEEDED if type(payload) is EmptyPayload:
            return RunSucceeded(*common)
        case EventKind.RUN_FAILED if type(payload) is EmptyPayload:
            return RunFailed(*common)
        case EventKind.RUN_CANCELLED if type(payload) is EmptyPayload:
            return RunCancelled(*common)
        case EventKind.EXECUTION_RELEASED if type(payload) is EmptyPayload:
            return ExecutionReleased(*common)
        case EventKind.TEXT_DELTA if type(payload) is TextPayload:
            return TextDelta(*common, payload.content)
        case EventKind.TOKEN_USAGE if type(payload) is UsagePayload:
            return TokenUsage(
                *common,
                int(payload.prompt_tokens),
                int(payload.completion_tokens),
                int(payload.total_tokens),
            )
        case EventKind.APPROVAL_REQUIRED if type(payload) is InteractionPayload:
            return ApprovalRequiredEvent(*common, InteractionId(payload.interaction_id))
        case EventKind.APPROVAL_RESOLVED if type(payload) is ApprovalPayload:
            return ApprovalResolvedEvent(
                *common, InteractionId(payload.interaction_id), payload.approved == "true"
            )
        case EventKind.TOOL_STARTED if type(payload) is ToolPayload:
            return ToolStarted(*common, ToolCallId(payload.tool_call_id))
        case EventKind.TOOL_SUCCEEDED if type(payload) is ToolPayload:
            return ToolSucceeded(*common, ToolCallId(payload.tool_call_id))
        case EventKind.TOOL_FAILED if type(payload) is ToolPayload:
            return ToolFailed(*common, ToolCallId(payload.tool_call_id))
        case EventKind.TOOL_OUTCOME_UNKNOWN if type(payload) is ToolPayload:
            return ToolOutcomeUnknown(*common, ToolCallId(payload.tool_call_id))
        case _:
            reason = "事件种类与载荷模型不匹配"
            raise MalformedEventRecordError(reason)


def record_matches_event(record: EventRecord, event: DomainEvent) -> bool:
    """比较持久化身份与全部不可变事件内容."""
    kind, payload = serialize_event(event)
    existing = record.payload
    if "schema_version" not in existing:
        existing = {**existing, "schema_version": "1"}
    return (
        record.run_id == str(event.run_id)
        and record.revision == event.revision
        and record.kind == kind.value
        and record.occurred_at == event.occurred_at
        and existing == payload
    )
