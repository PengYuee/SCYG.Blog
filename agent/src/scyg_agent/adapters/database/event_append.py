"""Atomic PostgreSQL DomainEvent append transaction."""

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from scyg_agent.domain.ports.event_store import (
    Appended,
    AppendRequest,
    AppendResult,
    EventCursor,
    EventIdentityConflict,
    EventRunNotFound,
    PartialEventBatchConflict,
    StoredEvent,
)

from .event_codec import record_matches_event, serialize_event
from .journal_records import EventRecord
from .run_records import RunRecord


async def append_events(
    session: AsyncSession, request: AppendRequest, notification_channel: str
) -> AppendResult:
    """Allocate contiguous sequence and insert one all-new or all-existing batch."""
    for event_id in sorted(str(event.event_id) for event in request.events):
        _ = await session.execute(
            select(func.pg_advisory_xact_lock(func.hashtextextended(event_id, 0)))
        )
    run = (
        await session.execute(
            select(RunRecord.run_id)
            .where(RunRecord.run_id == str(request.run_id))
            .with_for_update()
        )
    ).scalar_one_or_none()
    if run is None:
        return EventRunNotFound(request.run_id)
    return await append_events_locked(session, request, notification_channel)


async def append_events_locked(
    session: AsyncSession, request: AppendRequest, notification_channel: str
) -> AppendResult:
    """在调用方已持有 Run 行锁时追加事件, 保持同一事务原子性。."""
    existing = tuple(
        (
            await session.execute(
                select(EventRecord).where(
                    EventRecord.event_id.in_(tuple(str(event.event_id) for event in request.events))
                )
            )
        ).scalars()
    )
    if existing and len(existing) != len(request.events):
        return PartialEventBatchConflict(request.run_id)
    if existing:
        return _existing_batch(existing, request)
    latest = (
        await session.execute(
            select(func.coalesce(func.max(EventRecord.seq), 0)).where(
                EventRecord.run_id == str(request.run_id)
            )
        )
    ).scalar_one()
    stored = tuple(
        StoredEvent(EventCursor(latest + offset), event)
        for offset, event in enumerate(request.events, start=1)
    )
    for item in stored:
        kind, payload = serialize_event(item.event)
        session.add(
            EventRecord(
                event_id=str(item.event.event_id),
                run_id=str(request.run_id),
                seq=item.cursor.sequence,
                revision=item.event.revision,
                kind=kind.value,
                occurred_at=item.event.occurred_at,
                payload=payload,
            )
        )
    await session.flush()
    _ = await session.execute(select(func.pg_notify(notification_channel, str(request.run_id))))
    return Appended(stored)


def _existing_batch(records: tuple[EventRecord, ...], request: AppendRequest) -> AppendResult:
    """Return an idempotent batch or its first immutable identity conflict."""
    by_id = {record.event_id: record for record in records}
    for event in request.events:
        record = by_id[str(event.event_id)]
        if not record_matches_event(record, event):
            return EventIdentityConflict(event.event_id)
    return Appended(
        tuple(
            StoredEvent(EventCursor(by_id[str(event.event_id)].seq), event)
            for event in request.events
        )
    )
