"""Typed commands accepted by the Run state machine."""

from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum

from .errors import InvalidRunError, UnknownVariant
from .models import (
    CommandId,
    EventId,
    ExecutionOwnerId,
    InteractionId,
    validate_utc_timestamp,
)


class CommandKind(StrEnum):
    """Discriminate every mutating Run command."""

    CLAIM = "claim"
    REQUEST_INPUT = "request_input"
    SUBMIT_INPUT = "submit_input"
    SUCCEED = "succeed"
    FAIL = "fail"
    CANCEL = "cancel"
    RELEASE_EXECUTION = "release_execution"


@dataclass(frozen=True, slots=True)
class RunCommandBase:
    """Carry deterministic command, event, fencing, and time identity."""

    command_id: CommandId
    event_id: EventId
    expected_revision: int
    occurred_at: datetime

    def __post_init__(self) -> None:
        """Validate command fencing and injected event time."""
        if self.expected_revision < 1:
            invariant = "expected_revision must be positive"
            raise InvalidRunError(invariant)
        validate_utc_timestamp(self.occurred_at, "occurred_at")


@dataclass(frozen=True, slots=True)
class ClaimRun(RunCommandBase):
    """Claim and start pending execution under one semantic owner."""

    execution_owner: ExecutionOwnerId


@dataclass(frozen=True, slots=True)
class RequestInput(RunCommandBase):
    """Pause running execution for one durable interaction."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class SubmitInput(RunCommandBase):
    """Resolve one waiting interaction and queue durable resume."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class SucceedRun(RunCommandBase):
    """Finish running execution successfully."""


@dataclass(frozen=True, slots=True)
class FailRun(RunCommandBase):
    """Finish running execution with stable failure meaning."""


@dataclass(frozen=True, slots=True)
class CancelRun(RunCommandBase):
    """Cancel a non-terminal Run."""


@dataclass(frozen=True, slots=True)
class ReleaseExecution(RunCommandBase):
    """Release running ownership back to the pending queue."""


type RunCommand = (
    ClaimRun | RequestInput | SubmitInput | SucceedRun | FailRun | CancelRun | ReleaseExecution
)


def command_kind(command: RunCommand) -> CommandKind | UnknownVariant:
    """Return the stable discriminator for a typed command variant."""
    kinds = {
        ClaimRun: CommandKind.CLAIM,
        RequestInput: CommandKind.REQUEST_INPUT,
        SubmitInput: CommandKind.SUBMIT_INPUT,
        SucceedRun: CommandKind.SUCCEED,
        FailRun: CommandKind.FAIL,
        CancelRun: CommandKind.CANCEL,
        ReleaseExecution: CommandKind.RELEASE_EXECUTION,
    }
    kind = kinds.get(type(command))
    if kind is None:
        return UnknownVariant("command", type(command).__name__)
    return kind
