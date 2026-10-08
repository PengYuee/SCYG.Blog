"""Explicitly ready event subscriptions producing complete SSE frames."""

import json
from collections.abc import AsyncIterator
from typing import final

from scyg_agent.adapters.database.event_codec import serialize_event
from scyg_agent.domain.ports.event_store import EventStore, OpenedEventSubscription, StoredEvent
from scyg_agent.domain.runs import RunId

from .event_cursor import decode_event_cursor, encode_event_cursor

HEARTBEAT_SECONDS = 10.0
HEARTBEAT_FRAME = ": heartbeat\n\n"


def encode_event_frame(stored: StoredEvent) -> str:
    """Preserve the durable domain payload and use a Run-bound reconnection ID."""
    event = stored.event
    kind, payload = serialize_event(event)
    encoded = json.dumps(
        {
            "event_id": str(event.event_id),
            "run_id": str(event.run_id),
            "revision": event.revision,
            "occurred_at": event.occurred_at.isoformat(),
            "kind": kind.value,
            "payload": payload,
        },
        ensure_ascii=False,
        allow_nan=False,
        separators=(",", ":"),
    )
    cursor = encode_event_cursor(event.run_id, stored.cursor)
    return f"id: {cursor}\nevent: {kind.value}\ndata: {encoded}\n\n"


@final
class OpenedEventStream:
    """Own an already registered subscription independently of generator startup."""

    def __init__(self, subscription: OpenedEventSubscription) -> None:
        """Take ownership of an already ready durable subscription."""
        self._subscription = subscription

    async def frames(self) -> AsyncIterator[str]:
        """Yield complete frames, heartbeats every ten seconds, and terminal EOF."""
        try:
            while True:
                try:
                    event = await self._subscription.next_event(HEARTBEAT_SECONDS)
                except StopAsyncIteration:
                    return
                yield HEARTBEAT_FRAME if event is None else encode_event_frame(event)
        finally:
            await self.aclose()

    async def aclose(self) -> None:
        """Release even when the consumer never starts frames()."""
        await self._subscription.aclose()


@final
class EventSubscriptionService:
    """Open an authorized Run's validated listener before transport readiness."""

    def __init__(self, event_store: EventStore) -> None:
        """Bind the journal used for cursor validation and listener readiness."""
        self._event_store = event_store

    async def open_events(self, run_id: RunId, cursor: str | None) -> OpenedEventStream:
        """Validate the opaque cursor and await listener/race-replay readiness."""
        parsed = decode_event_cursor(run_id, cursor)
        subscription = await self._event_store.open_subscription(run_id, parsed)
        return OpenedEventStream(subscription)
