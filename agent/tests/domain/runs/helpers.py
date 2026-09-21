"""Deterministic Run domain test values."""

from datetime import UTC, datetime
from typing import assert_never

from scyg_agent.domain.runs import (
    CancelRun,
    ClaimRun,
    CommandId,
    CommandKind,
    EventId,
    ExecutionOwnerId,
    FailRun,
    InteractionId,
    ReleaseExecution,
    RequestInput,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    SubmitInput,
    SucceedRun,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.commands import RunCommand

NOW = datetime(2026, 7, 11, 10, 0, tzinfo=UTC)
LATER = datetime(2026, 7, 11, 10, 1, tzinfo=UTC)


def make_run(status: RunStatus, *, revision: int = 5) -> Run:
    """Build one valid aggregate snapshot for a matrix status."""
    execution_owner = ExecutionOwnerId("worker_12345678") if status is RunStatus.RUNNING else None
    pending_interaction_id = (
        InteractionId("int_12345678") if status is RunStatus.WAITING_INPUT else None
    )
    return Run(
        id=RunId("run_12345678"),
        owner_user_id=UserId("user-1"),
        task_type=TaskType.SUMMARY,
        runtime=RuntimeSelection(RuntimeKind.SIMPLE, "v1"),
        revision=revision,
        status=status,
        created_at=NOW,
        updated_at=NOW,
        attempt=1,
        execution_owner=execution_owner,
        pending_interaction_id=pending_interaction_id,
    )


def command_for(  # noqa: PLR0911
    kind: CommandKind,
    *,
    expected_revision: int = 5,
    identity: str = "12345678",
) -> RunCommand:
    """Build a deterministic command variant for matrix tests."""
    command_id = CommandId(f"cmd_{identity}")
    event_id = EventId(f"evt_{identity}")
    if kind is CommandKind.CLAIM:
        return ClaimRun(
            command_id,
            event_id,
            expected_revision,
            LATER,
            ExecutionOwnerId("worker_abcdefgh"),
        )
    if kind is CommandKind.REQUEST_INPUT:
        return RequestInput(
            command_id, event_id, expected_revision, LATER, InteractionId("int_12345678")
        )
    if kind is CommandKind.SUBMIT_INPUT:
        return SubmitInput(
            command_id, event_id, expected_revision, LATER, InteractionId("int_12345678")
        )
    if kind is CommandKind.SUCCEED:
        return SucceedRun(command_id, event_id, expected_revision, LATER)
    if kind is CommandKind.FAIL:
        return FailRun(command_id, event_id, expected_revision, LATER)
    if kind is CommandKind.CANCEL:
        return CancelRun(command_id, event_id, expected_revision, LATER)
    if kind is CommandKind.RELEASE_EXECUTION:
        return ReleaseExecution(command_id, event_id, expected_revision, LATER)
    assert_never(kind)
