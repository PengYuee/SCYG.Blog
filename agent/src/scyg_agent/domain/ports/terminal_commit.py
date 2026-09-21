"""围栏 Run 终态原子提交的纯类型契约."""

from dataclasses import dataclass
from typing import Final

from scyg_agent.domain.ports.audit_store import AuditFact, StoredAuditFact
from scyg_agent.domain.ports.event_store import AppendRequest, StoredEvent
from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    EventId,
    Run,
    RunCancelled,
    RunFailed,
    RunId,
    RunStatus,
    RunSucceeded,
)
from scyg_agent.domain.runs.repository import CompletionRequest

MAX_ERROR_CODE_LENGTH: Final = 64
MAX_ERROR_MESSAGE_LENGTH: Final = 512


@dataclass(frozen=True, slots=True)
class TerminalResult:
    """Carry one validated capability result into the terminal transaction."""

    schema_version: str
    capability: str
    payload: dict[str, object]
    digest: str


@dataclass(frozen=True, slots=True)
class TerminalCommitRequest:
    """绑定终态围栏、事件、审计和可选结构化结果."""

    completion: CompletionRequest
    events: AppendRequest
    audit: AuditFact
    result: TerminalResult | None = None
    error_code: str | None = None
    error_message: str | None = None

    def __post_init__(self) -> None:
        """要求终态、结构化结果和稳定失败字段严格一致."""
        run_id = self.completion.guard.run_id
        if self.events.run_id != run_id or self.audit.run_id != run_id:
            raise InvalidTerminalCommitRequestError
        if self.result is not None and self.completion.status is not RunStatus.SUCCEEDED:
            raise InvalidTerminalCommitRequestError
        has_error = self.error_code is not None or self.error_message is not None
        if has_error and self.completion.status is not RunStatus.FAILED:
            raise InvalidTerminalCommitRequestError
        if (self.error_code is None) != (self.error_message is None):
            raise InvalidTerminalCommitRequestError
        if self.error_code is not None and (
            type(self.error_code) is not str
            or not 1 <= len(self.error_code) <= MAX_ERROR_CODE_LENGTH
        ):
            raise InvalidTerminalCommitRequestError
        if self.error_message is not None and (
            type(self.error_message) is not str
            or not 1 <= len(self.error_message) <= MAX_ERROR_MESSAGE_LENGTH
        ):
            raise InvalidTerminalCommitRequestError
        expected_revision = self.completion.guard.expected_revision + 1
        if any(event.revision != expected_revision for event in self.events.events):
            raise InvalidTerminalCommitRequestError
        terminal = self.events.events[-1]
        expected_type = _completion_event_type(self.completion.status)
        if type(terminal) is not expected_type:
            raise InvalidTerminalCommitRequestError


def _completion_event_type(
    status: RunStatus,
) -> type[RunSucceeded] | type[RunFailed] | type[RunCancelled] | type[ApprovalRequiredEvent]:
    """按完成状态返回必须位于批末尾的精确事件类型."""
    match status:  # noqa: RUF100  # noqa: MATCH_OK - 仅允许四个提交状态。
        case RunStatus.SUCCEEDED:
            return RunSucceeded
        case RunStatus.FAILED:
            return RunFailed
        case RunStatus.CANCELLED:
            return RunCancelled
        case RunStatus.WAITING_INPUT:
            return ApprovalRequiredEvent
        case _:
            raise InvalidTerminalCommitRequestError


class InvalidTerminalCommitRequestError(ValueError):
    """拒绝跨 Run 的原子提交请求."""


@dataclass(frozen=True, slots=True)
class TerminalCommitted:
    """返回一次提交产生的 Run、事件序列和审计序列."""

    run: Run
    events: tuple[StoredEvent, ...]
    audit: StoredAuditFact


@dataclass(frozen=True, slots=True)
class TerminalReplay:
    """报告相同终态已经提交且没有新增副作用."""

    run: Run


@dataclass(frozen=True, slots=True)
class TerminalLeaseLost:
    """报告 token、owner 或有效期不再拥有 Run."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class TerminalCancellationRequested:
    """报告用户取消优先于非取消终态."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class TerminalStateConflict:
    """报告 revision 或当前状态与请求终态冲突."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class TerminalEventConflict:
    """报告事件身份对应不可变内容不一致."""

    run_id: RunId
    event_id: EventId | None


type TerminalCommitResult = (
    TerminalCommitted
    | TerminalReplay
    | TerminalLeaseLost
    | TerminalCancellationRequested
    | TerminalStateConflict
    | TerminalEventConflict
)
