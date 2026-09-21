"""Frozen success and expected failure variants returned by Run transitions."""

from dataclasses import dataclass
from datetime import datetime

from .commands import CommandKind
from .errors import UnknownVariant
from .events import DomainEvent
from .models import InteractionId, Run, RunStatus


@dataclass(frozen=True, slots=True)
class RevisionMismatch:
    """Reject a command fenced against an obsolete aggregate revision."""

    expected_revision: int
    actual_revision: int


@dataclass(frozen=True, slots=True)
class IllegalTransition:
    """Reject a command that is invalid for the current durable status."""

    status: RunStatus
    command: CommandKind


@dataclass(frozen=True, slots=True)
class InteractionMismatch:
    """Reject input submitted for an interaction other than the pending one."""

    expected: InteractionId
    actual: InteractionId


@dataclass(frozen=True, slots=True)
class TimestampRegression:
    """Reject a mutation whose injected occurrence time moves backward."""

    current: datetime
    occurred_at: datetime


@dataclass(frozen=True, slots=True)
class Transitioned:
    """Return one new aggregate snapshot and its immutable event facts."""

    run: Run
    events: tuple[DomainEvent, ...]


type TransitionResult = (
    Transitioned
    | RevisionMismatch
    | IllegalTransition
    | InteractionMismatch
    | TimestampRegression
    | UnknownVariant
)
