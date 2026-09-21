"""Typed command handlers used after matrix and fencing validation."""

from dataclasses import replace

from .commands import (
    CancelRun,
    ClaimRun,
    FailRun,
    ReleaseExecution,
    RequestInput,
    RunCommand,
    SubmitInput,
    SucceedRun,
)
from .errors import UnknownVariant
from .events import (
    ExecutionReleased,
    InputRequested,
    InputResolved,
    RunCancelled,
    RunFailed,
    RunSucceeded,
    StatusChanged,
)
from .models import Run, RunStatus
from .outcomes import Transitioned

type ProgressCommand = ClaimRun | RequestInput | SubmitInput
type FinishCommand = SucceedRun | FailRun | CancelRun | ReleaseExecution


def apply_transition(run: Run, command: RunCommand) -> Transitioned | UnknownVariant:
    """Dispatch a command already proven legal by the transition matrix."""
    if type(command) not in {
        ClaimRun,
        RequestInput,
        SubmitInput,
        SucceedRun,
        FailRun,
        CancelRun,
        ReleaseExecution,
    }:
        return UnknownVariant("command", type(command).__name__)
    if isinstance(command, ClaimRun | RequestInput | SubmitInput):
        return _apply_progress(run, command)
    return _apply_finish(run, command)


def _apply_progress(run: Run, command: ProgressCommand) -> Transitioned | UnknownVariant:
    if type(command) not in {ClaimRun, RequestInput, SubmitInput}:
        return UnknownVariant("command", type(command).__name__)
    revision = run.revision + 1
    if isinstance(command, ClaimRun):
        updated = replace(
            run,
            status=RunStatus.RUNNING,
            revision=revision,
            updated_at=command.occurred_at,
            attempt=run.attempt + 1,
            execution_owner=command.execution_owner,
            pending_interaction_id=None,
        )
        event = StatusChanged(
            command.event_id,
            command.command_id,
            command.occurred_at,
            run.id,
            revision,
            run.status,
            RunStatus.RUNNING,
        )
        return Transitioned(updated, (event,))
    if isinstance(command, RequestInput):
        updated = replace(
            run,
            status=RunStatus.WAITING_INPUT,
            revision=revision,
            updated_at=command.occurred_at,
            execution_owner=None,
            pending_interaction_id=command.interaction_id,
        )
        event = InputRequested(
            command.event_id,
            command.command_id,
            command.occurred_at,
            run.id,
            revision,
            command.interaction_id,
        )
        return Transitioned(updated, (event,))
    updated = replace(
        run,
        status=RunStatus.PENDING_RESUME,
        revision=revision,
        updated_at=command.occurred_at,
        pending_interaction_id=None,
    )
    event = InputResolved(
        command.event_id,
        command.command_id,
        command.occurred_at,
        run.id,
        revision,
        command.interaction_id,
    )
    return Transitioned(updated, (event,))


def _apply_finish(run: Run, command: FinishCommand) -> Transitioned | UnknownVariant:
    if type(command) not in {SucceedRun, FailRun, CancelRun, ReleaseExecution}:
        return UnknownVariant("command", type(command).__name__)
    revision = run.revision + 1
    if isinstance(command, SucceedRun):
        updated = replace(
            run,
            status=RunStatus.SUCCEEDED,
            revision=revision,
            updated_at=command.occurred_at,
            execution_owner=None,
            pending_interaction_id=None,
        )
        event = RunSucceeded(
            command.event_id, command.command_id, command.occurred_at, run.id, revision
        )
        return Transitioned(updated, (event,))
    if isinstance(command, FailRun):
        updated = replace(
            run,
            status=RunStatus.FAILED,
            revision=revision,
            updated_at=command.occurred_at,
            execution_owner=None,
            pending_interaction_id=None,
        )
        event = RunFailed(
            command.event_id, command.command_id, command.occurred_at, run.id, revision
        )
        return Transitioned(updated, (event,))
    if isinstance(command, CancelRun):
        updated = replace(
            run,
            status=RunStatus.CANCELLED,
            revision=revision,
            updated_at=command.occurred_at,
            execution_owner=None,
            pending_interaction_id=None,
        )
        event = RunCancelled(
            command.event_id, command.command_id, command.occurred_at, run.id, revision
        )
        return Transitioned(updated, (event,))
    updated = replace(
        run,
        status=RunStatus.PENDING,
        revision=revision,
        updated_at=command.occurred_at,
        execution_owner=None,
        pending_interaction_id=None,
    )
    event = ExecutionReleased(
        command.event_id,
        command.command_id,
        command.occurred_at,
        run.id,
        revision,
    )
    return Transitioned(updated, (event,))
