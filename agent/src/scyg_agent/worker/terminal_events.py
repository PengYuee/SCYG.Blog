"""Worker 终态事件的纯映射与取消构造."""

from datetime import datetime

from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    CommandId,
    DomainEvent,
    Run,
    RunCancelled,
    RunFailed,
    RunStatus,
    RunSucceeded,
)
from scyg_agent.domain.runs.repository import RunLease

from .identity import event_id


def completion_status(event: DomainEvent) -> RunStatus | None:
    """把批末事件收窄为可原子提交的状态."""
    statuses: dict[type[DomainEvent], RunStatus] = {
        RunSucceeded: RunStatus.SUCCEEDED,
        RunFailed: RunStatus.FAILED,
        RunCancelled: RunStatus.CANCELLED,
        ApprovalRequiredEvent: RunStatus.WAITING_INPUT,
    }
    return statuses.get(type(event))


def cancel_events(
    run: Run, lease: RunLease, command_id: CommandId, now: datetime
) -> tuple[DomainEvent, ...]:
    """构造确定性的唯一取消终态."""
    return (RunCancelled(event_id(run, "cancelled"), command_id, now, run.id, lease.revision + 1),)
