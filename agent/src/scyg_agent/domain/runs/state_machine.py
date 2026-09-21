"""Exhaustive pure Run transition reducer."""

from .commands import CommandKind, RunCommand, SubmitInput, command_kind
from .errors import InvalidRunError, UnknownVariant
from .models import Run, RunStatus
from .outcomes import (
    IllegalTransition,
    InteractionMismatch,
    RevisionMismatch,
    TimestampRegression,
    TransitionResult,
)
from .transition_handlers import apply_transition


def allowed_commands(status: RunStatus) -> tuple[CommandKind, ...]:
    """Return the explicit legal command row for one lifecycle status."""
    rows = {
        RunStatus.PENDING: (CommandKind.CLAIM, CommandKind.CANCEL),
        RunStatus.RUNNING: (
            CommandKind.REQUEST_INPUT,
            CommandKind.SUCCEED,
            CommandKind.FAIL,
            CommandKind.CANCEL,
            CommandKind.RELEASE_EXECUTION,
        ),
        RunStatus.WAITING_INPUT: (CommandKind.SUBMIT_INPUT, CommandKind.CANCEL),
        RunStatus.PENDING_RESUME: (CommandKind.CLAIM, CommandKind.CANCEL),
        RunStatus.SUCCEEDED: (),
        RunStatus.FAILED: (),
        RunStatus.CANCELLED: (),
    }
    return rows[status]


def transition(run: Run, command: RunCommand) -> TransitionResult:
    """Apply one revision-fenced command without I/O, clocks, or hidden identity."""
    if command.expected_revision != run.revision:
        return RevisionMismatch(command.expected_revision, run.revision)

    kind = command_kind(command)
    if isinstance(kind, UnknownVariant):
        return kind
    if kind not in allowed_commands(run.status):
        return IllegalTransition(run.status, kind)
    if command.occurred_at < run.updated_at:
        return TimestampRegression(run.updated_at, command.occurred_at)
    if isinstance(command, SubmitInput):
        pending_interaction_id = run.pending_interaction_id
        if pending_interaction_id is None:
            invariant = "WAITING_INPUT requires a pending interaction"
            raise InvalidRunError(invariant)
        if pending_interaction_id != command.interaction_id:
            return InteractionMismatch(pending_interaction_id, command.interaction_id)

    return apply_transition(run, command)
