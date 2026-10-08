"""Cancellable LISTEN/NOTIFY wakeup lifecycle for durable event replay."""

import sys
from collections import deque
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import NoReturn, Protocol, final, override

import anyio
import asyncpg
from anyio import ClosedResourceError, WouldBlock
from anyio.lowlevel import checkpoint
from anyio.streams.memory import MemoryObjectReceiveStream, MemoryObjectSendStream

from scyg_agent.domain.ports.event_store import (
    CursorTooOld,
    EventCursor,
    FutureCursor,
    ReplayPage,
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


@dataclass(frozen=True, slots=True)
class RegisteredListener:
    """Bind the listener callback and wake streams owned by one subscription."""

    connection: ListenerConnection
    callback: Callable[..., None]
    send: MemoryObjectSendStream[None]
    receive: MemoryObjectReceiveStream[None]


def _end_subscription() -> NoReturn:
    raise StopAsyncIteration


@final
class EventSubscription:
    """Own a registered listener and one bounded, validated replay page."""

    def __init__(
        self,
        run_id: RunId,
        cursor: EventCursor,
        resources: SubscriptionResources,
        listener: RegisteredListener,
        page: ReplayPage,
    ) -> None:
        """Capture the listener and bounded race replay after explicit readiness."""
        self._run_id = run_id
        self._cursor = cursor
        self._resources = resources
        self._connection = listener.connection
        self._callback = listener.callback
        self._send = listener.send
        self._receive = listener.receive
        self._page = page
        self._pending = deque(page.events)
        self._closed = False

    async def next_event(self, wait_seconds: float = 10.0) -> StoredEvent | None:
        """Replay truth after every wake or heartbeat and end at a terminal head."""
        try:
            while not self._closed:
                if self._pending:
                    stored = self._pending.popleft()
                    self._cursor = stored.cursor
                    return stored
                if self._page.terminal and self._cursor == self._page.latest:
                    await self.aclose()
                    _end_subscription()
                self._page = _checked(
                    await self._resources.replay(
                        self._run_id, self._cursor, self._resources.replay_limit
                    )
                )
                self._pending.extend(self._page.events)
                if self._pending or self._page.terminal:
                    continue
                with anyio.move_on_after(wait_seconds) as waiting:
                    await self._receive.receive()
                if waiting.cancel_called:
                    return None
            _end_subscription()
        except BaseException:  # BROAD_EXCEPT_OK: close on cancellation, then re-raise.
            await self.aclose()
            raise

    async def aclose(self) -> None:
        """Release listener and wake streams once, including under cancellation."""
        if self._closed:
            return
        self._closed = True
        try:
            await _cleanup_listener(
                self._connection, self._resources, self._callback, registered=True
            )
        finally:
            with anyio.CancelScope(shield=True):
                await self._send.aclose()
                await self._receive.aclose()


def _checked(page: ReplayResult) -> ReplayPage:
    if isinstance(page, (FutureCursor, CursorTooOld)):
        raise SubscriptionCursorError(page)
    return page


async def open_subscription(
    run_id: RunId,
    cursor: EventCursor | None,
    resources: SubscriptionResources,
) -> EventSubscription:
    """Validate, LISTEN, and check a bounded race replay before returning ready."""
    initial = await resources.replay(run_id, cursor or EventCursor(0), resources.replay_limit)
    if cursor is None:
        floor = (
            initial.minimum_retained
            if isinstance(initial, CursorTooOld)
            else _checked(initial).minimum_retained
        )
        cursor = EventCursor(max(0, floor.sequence - 1))
    else:
        _ = _checked(initial)
    wake_send, wake_receive = anyio.create_memory_object_stream[None](max_buffer_size=1)

    def wakeup(
        _connection: ListenerConnection, _process_id: int, _channel: str, payload: str
    ) -> None:
        if payload != str(run_id):
            return
        try:
            wake_send.send_nowait(None)
        except (WouldBlock, ClosedResourceError):
            return

    connection: ListenerConnection | None = None
    registered = False
    try:
        connection = await resources.listener_factory(resources.listener_dsn)
        await connection.add_listener(resources.channel, wakeup)
        registered = True
        page = _checked(await resources.replay(run_id, cursor, resources.replay_limit))
        return EventSubscription(
            run_id,
            cursor,
            resources,
            RegisteredListener(connection, wakeup, wake_send, wake_receive),
            page,
        )
    except BaseException:  # BROAD_EXCEPT_OK: unwind partial resources on cancellation.
        try:
            if connection is not None:
                await _cleanup_listener(connection, resources, wakeup, registered=registered)
        finally:
            with anyio.CancelScope(shield=True):
                await wake_send.aclose()
                await wake_receive.aclose()
        raise


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
