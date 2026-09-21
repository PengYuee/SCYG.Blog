"""原子 Run 命令应用的纯领域端口。."""

from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from typing import Protocol

from scyg_agent.domain.ports.event_store import StoredEvent
from scyg_agent.domain.runs import (
    CancelRun,
    CommandId,
    CommandKind,
    Run,
    RunCommand,
    RunId,
    SubmitInput,
)

from .idempotency import AuditMetadata, RequestDigest, ResultReference, require_utc


class CommandResultStatus(StrEnum):
    """限定持久化命令结果状态。."""

    SUCCEEDED = "succeeded"
    REJECTED = "rejected"


class ExternalCommandKind(StrEnum):
    """限定无租约公共命令平面允许的变体。."""

    SUBMIT_INPUT = "submit_input"
    CANCEL = "cancel"


def external_command_kind(command: RunCommand) -> ExternalCommandKind | None:
    """分类安全用户命令并拒绝 worker/internal 变体。."""
    if type(command) is SubmitInput:
        return ExternalCommandKind.SUBMIT_INPUT
    if type(command) is CancelRun:
        return ExternalCommandKind.CANCEL
    return None


@dataclass(frozen=True, slots=True)
class CommandSubmission:
    """携带一次确定性的 Run 命令提交。."""

    command_id: CommandId
    run_id: RunId
    expected_revision: int
    expected_sequence: int
    kind: str
    request_digest: RequestDigest
    submitted_at: datetime
    command: RunCommand
    audit_metadata: AuditMetadata

    def __post_init__(self) -> None:
        """保证所有身份和乐观锁字段内部一致。."""
        if self.expected_revision < 1 or self.expected_sequence < 0:
            raise InvalidCommandSubmissionError
        if (
            self.command.command_id != self.command_id
            or self.command.expected_revision != self.expected_revision
        ):
            raise InvalidCommandSubmissionError
        require_utc(self.submitted_at, "submitted_at")


@dataclass(frozen=True, slots=True)
class InvalidCommandSubmissionError(ValueError):
    """拒绝内部不一致的命令提交。."""


@dataclass(frozen=True, slots=True)
class CommandApplied:
    """返回首次提交或完全相同重试的原始成功结果。."""

    run: Run
    events: tuple[StoredEvent, ...]
    result_reference: ResultReference
    replayed: bool


@dataclass(frozen=True, slots=True)
class CommandRejected:
    """返回可确定重放的领域拒绝。."""

    code: str
    actual_revision: int
    actual_sequence: int
    replayed: bool


@dataclass(frozen=True, slots=True)
class IdempotencyConflict:
    """报告同一命令身份被用于不同不可变请求。."""

    command_id: CommandId


@dataclass(frozen=True, slots=True)
class CommandRunNotFound:
    """报告目标 Run 不存在。."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class UnsupportedCommand:
    """拒绝不能由无租约公共命令平面执行的变体。."""

    command_id: CommandId
    kind: CommandKind


@dataclass(frozen=True, slots=True)
class CommandDataIntegrity:
    """报告无法安全恢复的持久化命令数据。."""

    command_id: CommandId


type CommandApplyResult = (
    CommandApplied
    | CommandRejected
    | IdempotencyConflict
    | CommandRunNotFound
    | UnsupportedCommand
    | CommandDataIntegrity
)


class CommandStore(Protocol):
    """在一个短事务中应用并记录 Run 命令。."""

    async def apply(self, request: CommandSubmission) -> CommandApplyResult:
        """应用一次命令或重放原始结果。."""
        ...  # pragma: no cover
