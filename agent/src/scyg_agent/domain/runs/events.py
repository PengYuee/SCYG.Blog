"""Payload-free typed facts emitted by Run transitions."""

from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum

from .errors import InvalidRunError, UnknownVariant
from .models import (
    CommandId,
    EventId,
    InteractionId,
    RunId,
    RunStatus,
    ToolCallId,
    validate_utc_timestamp,
)


class EventKind(StrEnum):
    """Discriminate stable Run domain event meaning."""

    STATUS_CHANGED = "status_changed"
    INPUT_REQUESTED = "input_requested"
    INPUT_RESOLVED = "input_resolved"
    RUN_SUCCEEDED = "run_succeeded"
    RUN_FAILED = "run_failed"
    RUN_CANCELLED = "run_cancelled"
    EXECUTION_RELEASED = "execution_released"
    TEXT_DELTA = "text_delta"
    TOKEN_USAGE = "token_usage"  # noqa: S105 - 事件种类不是凭据。
    APPROVAL_REQUIRED = "approval_required"
    APPROVAL_RESOLVED = "approval_resolved"
    TOOL_STARTED = "tool_started"
    TOOL_SUCCEEDED = "tool_succeeded"
    TOOL_FAILED = "tool_failed"
    TOOL_OUTCOME_UNKNOWN = "tool_outcome_unknown"


@dataclass(frozen=True, slots=True)
class DomainEventBase:
    """Carry identity and fencing shared by every domain event."""

    event_id: EventId
    command_id: CommandId
    occurred_at: datetime
    run_id: RunId
    revision: int

    def __post_init__(self) -> None:
        """Validate event revision and injected occurrence time."""
        if self.revision < 1:
            invariant = "event revision must be positive"
            raise InvalidRunError(invariant)
        validate_utc_timestamp(self.occurred_at, "occurred_at")


@dataclass(frozen=True, slots=True)
class StatusChanged(DomainEventBase):
    """Record a lifecycle status change without adapter payload."""

    previous: RunStatus
    current: RunStatus


@dataclass(frozen=True, slots=True)
class InputRequested(DomainEventBase):
    """Record that a durable interaction now blocks execution."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class InputResolved(DomainEventBase):
    """Record that a durable interaction queued resume."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class RunSucceeded(DomainEventBase):
    """Record terminal successful completion."""


@dataclass(frozen=True, slots=True)
class RunFailed(DomainEventBase):
    """Record terminal failed completion."""


@dataclass(frozen=True, slots=True)
class RunCancelled(DomainEventBase):
    """Record terminal user or application cancellation."""


@dataclass(frozen=True, slots=True)
class ExecutionReleased(DomainEventBase):
    """Record that running ownership returned to pending."""


@dataclass(frozen=True, slots=True)
class TextDelta(DomainEventBase):
    """记录一个保持原始顺序的非空文本增量."""

    content: str


@dataclass(frozen=True, slots=True)
class TokenUsage(DomainEventBase):
    """记录提供方已确认的完整令牌用量."""

    prompt_tokens: int
    completion_tokens: int
    total_tokens: int


@dataclass(frozen=True, slots=True)
class ApprovalRequiredEvent(DomainEventBase):
    """记录工具提案正等待指定交互审批."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class ApprovalResolvedEvent(DomainEventBase):
    """记录审批交互的无秘密决定."""

    interaction_id: InteractionId
    approved: bool


@dataclass(frozen=True, slots=True)
class ToolStarted(DomainEventBase):
    """记录逻辑工具调用已开始."""

    tool_call_id: ToolCallId


@dataclass(frozen=True, slots=True)
class ToolSucceeded(DomainEventBase):
    """记录逻辑工具调用成功."""

    tool_call_id: ToolCallId


@dataclass(frozen=True, slots=True)
class ToolFailed(DomainEventBase):
    """记录逻辑工具调用失败."""

    tool_call_id: ToolCallId


@dataclass(frozen=True, slots=True)
class ToolOutcomeUnknown(DomainEventBase):
    """记录外部工具结果无法安全确定."""

    tool_call_id: ToolCallId


type DomainEvent = (
    StatusChanged
    | InputRequested
    | InputResolved
    | RunSucceeded
    | RunFailed
    | RunCancelled
    | ExecutionReleased
    | TextDelta
    | TokenUsage
    | ApprovalRequiredEvent
    | ApprovalResolvedEvent
    | ToolStarted
    | ToolSucceeded
    | ToolFailed
    | ToolOutcomeUnknown
)


def event_kind(event: DomainEvent) -> EventKind | UnknownVariant:
    """Return the stable discriminator for a domain event variant."""
    kinds = {
        StatusChanged: EventKind.STATUS_CHANGED,
        InputRequested: EventKind.INPUT_REQUESTED,
        InputResolved: EventKind.INPUT_RESOLVED,
        RunSucceeded: EventKind.RUN_SUCCEEDED,
        RunFailed: EventKind.RUN_FAILED,
        RunCancelled: EventKind.RUN_CANCELLED,
        ExecutionReleased: EventKind.EXECUTION_RELEASED,
        TextDelta: EventKind.TEXT_DELTA,
        TokenUsage: EventKind.TOKEN_USAGE,
        ApprovalRequiredEvent: EventKind.APPROVAL_REQUIRED,
        ApprovalResolvedEvent: EventKind.APPROVAL_RESOLVED,
        ToolStarted: EventKind.TOOL_STARTED,
        ToolSucceeded: EventKind.TOOL_SUCCEEDED,
        ToolFailed: EventKind.TOOL_FAILED,
        ToolOutcomeUnknown: EventKind.TOOL_OUTCOME_UNKNOWN,
    }
    kind = kinds.get(type(event))
    if kind is None:
        return UnknownVariant("event", type(event).__name__)
    return kind
