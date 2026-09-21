"""Typed observable outcomes for persisted Run cancellation requests."""

from dataclasses import dataclass
from datetime import datetime

from .models import Run, RunId
from .repository_values import RunLease


@dataclass(frozen=True, slots=True)
class CancellationRequested:
    """Report a user cancellation visible to the current fence owner."""

    run_id: RunId
    requested_at: datetime
    lease: RunLease | None
    replayed: bool


@dataclass(frozen=True, slots=True)
class CancellationQueued:
    """Report cancellation persisted before a Run is claimed."""

    run_id: RunId
    requested_at: datetime
    replayed: bool


@dataclass(frozen=True, slots=True)
class CancellationTerminal:
    """Report an already immutable terminal Run."""

    run: Run
