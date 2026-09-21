"""Cancellable LISTEN/NOTIFY wakeup lifecycle for durable event replay."""

import sys
from collections.abc import AsyncGenerator, Awaitable, Callable
from dataclasses import dataclass
from typing import Protocol, override

import anyio
import asyncpg
from anyio import ClosedResourceError, WouldBlock
from anyio.lowlevel import checkpoint

from scyg_agent.domain.ports.event_store import (
    CursorTooOld,
    EventCursor,
    FutureCursor,
    ReplayResult,
    StoredEvent,
)
from scyg_agent.domain.runs import RunId


class ListenerConnection(Protocol):
    """Expose only the asyncpg listener lifecycle required by subscriptions."""

    async def add_listener(self, channel: str, callback: Callable[..., None]) -> None:
        """Register one notification callback."""
        ...

    async def remove_listener(self, channel: str, callback: Callable[..., None]) -> None:
        """Remove one notification callback."""
        ...

    async def close(self) -> None:
        """Close the listener connection."""
        ...

    def terminate(self) -> None:
        """Abort the underlying transport when graceful close cannot finish."""
        ...

    def is_closed(self) -> bool:
        """Report whether the underlying connection transport is closed."""
        ...


type ListenerFactory = Callable[[str], Awaitable[ListenerConnection]]
type Replay = Callable[[RunId, EventCursor, int], Awaitable[ReplayResult]]


@dataclass(frozen=True, slots=True)
class SubscriptionResources:
    """Bind listener and replay dependencies for one event store."""

    replay_limit: int
    replay: Replay
    listener_dsn: str
    channel: str
    listener_factory: ListenerFactory
    close_timeout: float = 5.0


async def open_listener_connection(dsn: str) -> ListenerConnection:
    """Open a dedicated asyncpg connection used only for notifications."""
    return await asyncpg.connect(dsn)


@dataclass(frozen=True, slots=True)
class SubscriptionCursorError(RuntimeError):
    """Stop a live tail whose cursor cannot be replayed."""

    outcome: FutureCursor | CursorTooOld

    @override
    def __str__(self) -> str:
        """Return a stable cursor-gap diagnostic."""
        return "subscription cursor is outside the retained event range"


@dataclass(frozen=True, slots=True)
class ListenerCleanupError(RuntimeError):
    """Report listener cleanup failure when no original operation error exists."""

    operation: str

    @override
    def __str__(self) -> str:
        """Return a stable value-free cleanup diagnostic."""
        return f"listener cleanup failed during {self.operation}"


async def subscribe_events(
    run_id: RunId,
    cursor: EventCursor,
    resources: SubscriptionResources,
) -> AsyncGenerator[StoredEvent]:
    """Replay, establish listener, replay race window, then wake and replay."""
    wake_send, wake_receive = anyio.create_memory_object_stream[None](max_buffer_size=1)

    def wakeup(
        _connection: ListenerConnection, _process_id: int, _channel: str, payload: str
    ) -> None:
        """Coalesce Run wakeups while durable rows remain event truth."""
        if payload != str(run_id):
            return
        try:
            wake_send.send_nowait(None)
        except (WouldBlock, ClosedResourceError):
            return

    async with wake_send, wake_receive:
        current = cursor
        initial = await resources.replay(run_id, current, resources.replay_limit)
        if isinstance(initial, (FutureCursor, CursorTooOld)):
            raise SubscriptionCursorError(initial)
        for stored in initial.events:
            current = stored.cursor
            yield stored
        connection = await resources.listener_factory(resources.listener_dsn)
        registered = False
        try:
            await connection.add_listener(resources.channel, wakeup)
            registered = True
            while True:
                page = await resources.replay(run_id, current, resources.replay_limit)
                if isinstance(page, (FutureCursor, CursorTooOld)):
                    raise SubscriptionCursorError(page)
                for stored in page.events:
                    current = stored.cursor
                    yield stored
                if page.events:
                    continue
                await wake_receive.receive()
        finally:
            await _cleanup_listener(connection, resources, wakeup, registered=registered)


async def _cleanup_listener(
    connection: ListenerConnection,
    resources: SubscriptionResources,
    callback: Callable[..., None],
    *,
    registered: bool,
) -> None:
    """Remove callback and guarantee graceful or aborted transport closure."""
    original_error = sys.exception()
    cleanup_operation: str | None = None
    with anyio.CancelScope(shield=True):
        if registered:
            try:
                await connection.remove_listener(resources.channel, callback)
            except RuntimeError:
                cleanup_operation = "remove_listener"
        try:
            with anyio.fail_after(resources.close_timeout):
                await connection.close()
        except TimeoutError:
            connection.terminate()
            await checkpoint()
            if not connection.is_closed():
                cleanup_operation = "terminate"
        except RuntimeError:
            connection.terminate()
            await checkpoint()
            cleanup_operation = "close"
    if cleanup_operation is not None and original_error is None:
        raise ListenerCleanupError(cleanup_operation)
