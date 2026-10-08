"""PostgreSQL event journal with LISTEN/NOTIFY used only for wakeups."""

import re
from dataclasses import dataclass
from typing import TYPE_CHECKING, Final, Self, cast, final, override

from sqlalchemy import func, select, true
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from sqlalchemy.orm import aliased

from scyg_agent.domain.ports.event_store import (
    AppendRequest,
    AppendResult,
    CursorTooOld,
    EventCursor,
    FutureCursor,
    ReplayPage,
    ReplayResult,
    StoredEvent,
    TerminalSnapshot,
    project_terminal_snapshot,
    validate_replay_limit,
)
from scyg_agent.domain.runs import RunId

from .event_append import append_events
from .event_codec import deserialize_event
from .event_subscription import (
    EventSubscription,
    ListenerFactory,
    SubscriptionResources,
    open_subscription,
)
from .journal_records import EventRecord
from .run_records import RunRecord

if TYPE_CHECKING:
    from sqlalchemy import Result

_CHANNEL_PATTERN: Final = re.compile(r"[a-z][a-z0-9_]{0,62}")
_MAX_STORED_SEQUENCE: Final = 2**31 - 1


@dataclass(frozen=True, slots=True)
class NotificationChannel:
    """Carry a validated PostgreSQL notification identifier."""

    value: str

    @classmethod
    def parse(cls, raw: str) -> Self:
        """Reject identifiers that cannot safely reach LISTEN."""
        if _CHANNEL_PATTERN.fullmatch(raw) is None:
            raise InvalidNotificationChannelError
        return cls(raw)


@dataclass(frozen=True, slots=True)
class InvalidNotificationChannelError(ValueError):
    """Report an unsafe notification identifier without reflecting it."""

    @override
    def __str__(self) -> str:
        """Return a stable value-free diagnostic."""
        return "notification channel must be a safe PostgreSQL identifier"


@final
class PostgreSQLEventStore:
    """Own short event transactions and cancellable dedicated listener connections."""

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        listener_dsn: str,
        channel: NotificationChannel,
        listener_factory: ListenerFactory,
        replay_limit: int = 100,
    ) -> None:
        """Bind database resources and a bounded replay batch size."""
        self._sessions = sessions
        self._listener_dsn = listener_dsn
        self._channel = channel
        self._listener_factory = listener_factory
        self._replay_limit = validate_replay_limit(replay_limit)

    async def append(self, request: AppendRequest) -> AppendResult:
        """Append under event-identity and Run-row locks, then notify on commit."""
        async with self._sessions.begin() as session:
            return await append_events(session, request, self._channel.value)

    async def replay(self, run_id: RunId, cursor: EventCursor, limit: int) -> ReplayResult:
        """Read exactly `seq > cursor` in ascending bounded order."""
        bounded_limit = validate_replay_limit(limit)
        # Keep one statement snapshot; values above INTEGER's range are necessarily
        # future cursors and must never reach asyncpg's int4 parameter encoder.
        query_sequence = min(cursor.sequence, _MAX_STORED_SEQUENCE)
        bounds = (
            select(
                func.coalesce(func.min(EventRecord.seq), 0).label("minimum"),
                func.coalesce(func.max(EventRecord.seq), 0).label("latest"),
            )
            .where(EventRecord.run_id == str(run_id))
            .subquery()
        )
        page = (
            select(EventRecord)
            .where(
                EventRecord.run_id == str(run_id),
                EventRecord.seq > query_sequence,
            )
            .order_by(EventRecord.seq)
            .limit(bounded_limit)
            .subquery()
        )
        record = aliased(EventRecord, page)
        status = select(RunRecord.status).where(RunRecord.run_id == str(run_id)).scalar_subquery()
        async with self._sessions() as session:
            result = cast(
                "Result[tuple[int, int, str | None, EventRecord | None]]",
                (
                    await session.execute(
                        select(bounds.c.minimum, bounds.c.latest, status, record)
                        .select_from(bounds.outerjoin(page, true()))
                        .order_by(record.seq)
                    )
                ),
            )
            rows = result.tuples().all()
        first = rows[0]
        minimum, latest = EventCursor(first[0]), EventCursor(first[1])
        if cursor.sequence > latest.sequence:
            return FutureCursor(cursor, latest)
        if latest.sequence and cursor.sequence < minimum.sequence - 1:
            return CursorTooOld(cursor, minimum)
        return ReplayPage(
            tuple(
                StoredEvent(EventCursor(record.seq), deserialize_event(record))
                for row in rows
                if (record := row[3]) is not None
            ),
            latest,
            minimum,
            first[2] in ("succeeded", "failed", "cancelled"),
        )

    async def minimum_retained_sequence(self, run_id: RunId) -> EventCursor:
        """Expose the retention floor required by cursor-gap policies."""
        async with self._sessions() as session:
            minimum, _ = await self._bounds(session, run_id)
            return minimum

    async def terminal_snapshot(self, run_id: RunId) -> TerminalSnapshot | None:
        """Load terminal events from durable storage and apply the pure projection."""
        async with self._sessions() as session:
            records = (
                await session.execute(
                    select(EventRecord)
                    .where(EventRecord.run_id == str(run_id))
                    .order_by(EventRecord.seq.desc())
                )
            ).scalars()
            stored = tuple(
                StoredEvent(EventCursor(record.seq), deserialize_event(record))
                for record in records
            )
        return project_terminal_snapshot(tuple(reversed(stored)))

    async def open_subscription(
        self, run_id: RunId, cursor: EventCursor | None
    ) -> EventSubscription:
        """Return only after cursor validation, LISTEN and bounded race replay."""
        return await open_subscription(
            run_id,
            cursor,
            SubscriptionResources(
                self._replay_limit,
                self.replay,
                self._listener_dsn,
                self._channel.value,
                self._listener_factory,
            ),
        )

    async def _bounds(
        self, session: AsyncSession, run_id: RunId
    ) -> tuple[EventCursor, EventCursor]:
        """Read one journal retention floor and head in the caller-owned session."""
        result = await session.execute(
            select(
                func.coalesce(func.min(EventRecord.seq), 0),
                func.coalesce(func.max(EventRecord.seq), 0),
            ).where(EventRecord.run_id == str(run_id))
        )
        minimum, latest = result.tuples().one()
        return EventCursor(minimum), EventCursor(latest)
