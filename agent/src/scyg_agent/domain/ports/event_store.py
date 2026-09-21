"""Pure typed contract for durable Run domain-event journals."""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from enum import StrEnum
from typing import Final, Protocol, override

from scyg_agent.domain.runs import (
    DomainEvent,
    EventId,
    RunCancelled,
    RunFailed,
    RunId,
    RunStatus,
    RunSucceeded,
)

MAX_REPLAY_LIMIT: Final = 1000


class AppendRequestRule(StrEnum):
    """Name stable value-free append request boundary failures."""

    SINGLE_RUN = "append batch must contain events for exactly one run"
    UNIQUE_IDENTITIES = "append batch event identities must be unique"


@dataclass(frozen=True, slots=True, order=True)
class EventCursor:
    """Identify the last consumed per-Run durable sequence."""

    sequence: int

    def __post_init__(self) -> None:
        """Reject cursors outside the journal sequence domain."""
        if self.sequence < 0:
            raise InvalidEventCursorError


@dataclass(frozen=True, slots=True)
class InvalidEventCursorError(ValueError):
    """Reject construction of a cursor outside its value domain."""

    @override
    def __str__(self) -> str:
        """Return the stable boundary diagnostic."""
        return "event cursor must be a nonnegative integer"


@dataclass(frozen=True, slots=True)
class MalformedCursor:
    """Report an unparseable cursor as an expected typed outcome."""

    message: str = "event cursor must be a nonnegative integer"

    @override
    def __str__(self) -> str:
        """Return the stable boundary diagnostic."""
        return self.message


@dataclass(frozen=True, slots=True)
class FutureCursor:
    """Report a cursor beyond the durable journal head."""

    cursor: EventCursor
    latest: EventCursor


@dataclass(frozen=True, slots=True)
class CursorTooOld:
    """Report a cursor whose next event predates retained journal truth."""

    cursor: EventCursor
    minimum_retained: EventCursor


@dataclass(frozen=True, slots=True)
class StoredEvent:
    """Pair a stable domain event with its durable per-Run sequence."""

    cursor: EventCursor
    event: DomainEvent


@dataclass(frozen=True, slots=True)
class AppendRequest:
    """Append one nonempty ordered event batch for exactly one Run."""

    run_id: RunId
    events: tuple[DomainEvent, ...]

    def __post_init__(self) -> None:
        """Require a batch whose event ownership matches the requested Run."""
        if not self.events or any(event.run_id != self.run_id for event in self.events):
            raise InvalidAppendRequestError(AppendRequestRule.SINGLE_RUN)
        event_ids = tuple(event.event_id for event in self.events)
        if len(set(event_ids)) != len(event_ids):
            raise InvalidAppendRequestError(AppendRequestRule.UNIQUE_IDENTITIES)


@dataclass(frozen=True, slots=True)
class InvalidAppendRequestError(ValueError):
    """Report an empty or mixed-Run append batch."""

    rule: AppendRequestRule

    @override
    def __str__(self) -> str:
        """Return a stable value-free boundary diagnostic."""
        return self.rule.value


@dataclass(frozen=True, slots=True)
class Appended:
    """Return durable records for a newly appended or idempotently replayed batch."""

    events: tuple[StoredEvent, ...]


@dataclass(frozen=True, slots=True)
class EventIdentityConflict:
    """Report reuse of an event identity for different immutable content."""

    event_id: EventId


@dataclass(frozen=True, slots=True)
class PartialEventBatchConflict:
    """Reject a batch mixing existing identities with new identities."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class EventRunNotFound:
    """Report an append target that has no durable Run row."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class InvalidReplayLimitError(ValueError):
    """Reject replay limits outside the bounded positive range."""

    @override
    def __str__(self) -> str:
        """Return a stable value-free diagnostic."""
        return f"replay limit must be between 1 and {MAX_REPLAY_LIMIT}"


@dataclass(frozen=True, slots=True)
class ReplayPage:
    """Return an ordered bounded durable replay and its observed journal head."""

    events: tuple[StoredEvent, ...]
    latest: EventCursor
    minimum_retained: EventCursor


@dataclass(frozen=True, slots=True)
class TerminalSnapshot:
    """Project terminal Run state solely from the durable journal."""

    run_id: RunId
    cursor: EventCursor
    event_id: EventId
    status: RunStatus
    revision: int


type CursorParseResult = EventCursor | MalformedCursor
type AppendResult = Appended | EventIdentityConflict | PartialEventBatchConflict | EventRunNotFound
type ReplayResult = ReplayPage | FutureCursor | CursorTooOld


def parse_event_cursor(raw: str | None) -> CursorParseResult:
    """Parse an optional untrusted cursor, where absence means before sequence one."""
    if raw is None or raw == "":
        return EventCursor(0)
    try:
        return EventCursor(int(raw))
    except ValueError:
        return MalformedCursor()


def validate_replay_limit(limit: int) -> int:
    """Return a bounded positive replay limit or raise its typed boundary error."""
    if limit < 1 or limit > MAX_REPLAY_LIMIT:
        raise InvalidReplayLimitError
    return limit


def project_terminal_snapshot(events: tuple[StoredEvent, ...]) -> TerminalSnapshot | None:
    """Derive the latest terminal snapshot without runtime-framework state."""
    terminal_statuses: dict[type[DomainEvent], RunStatus] = {
        RunSucceeded: RunStatus.SUCCEEDED,
        RunFailed: RunStatus.FAILED,
        RunCancelled: RunStatus.CANCELLED,
    }
    for stored in reversed(events):
        event = stored.event
        status = terminal_statuses.get(type(event))
        if status is None:
            continue
        return TerminalSnapshot(event.run_id, stored.cursor, event.event_id, status, event.revision)
    return None


class EventStore(Protocol):
    """Persist, replay, and tail domain events with the database as sole truth."""

    async def append(self, request: AppendRequest) -> AppendResult:
        """Atomically append or idempotently return one event batch."""
        ...  # pragma: no cover - protocol declaration.

    async def replay(self, run_id: RunId, cursor: EventCursor, limit: int) -> ReplayResult:
        """Read a bounded sequence-strict page after the supplied cursor."""
        ...  # pragma: no cover - protocol declaration.

    async def minimum_retained_sequence(self, run_id: RunId) -> EventCursor:
        """Return the oldest retained sequence, or the empty journal baseline."""
        ...  # pragma: no cover - protocol declaration.

    async def terminal_snapshot(self, run_id: RunId) -> TerminalSnapshot | None:
        """Load the persisted terminal projection derived from durable events."""
        ...  # pragma: no cover - protocol declaration.

    def subscribe(self, run_id: RunId, cursor: EventCursor) -> AsyncIterator[StoredEvent]:
        """Tail durable events using notifications only as coalescible wakeups."""
        ...  # pragma: no cover - protocol declaration.
