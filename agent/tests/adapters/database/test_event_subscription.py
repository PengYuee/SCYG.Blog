"""Deterministic subscription listener lifecycle matrix."""

from collections.abc import Callable
from dataclasses import replace
from datetime import UTC, datetime
from typing import final

import anyio
import pytest
from anyio.lowlevel import checkpoint

from scyg_agent.adapters.database.event_subscription import (
    ListenerConnection,
    SubscriptionCursorError,
    SubscriptionResources,
    open_subscription,
)
from scyg_agent.domain.ports.event_store import (
    CursorTooOld,
    EventCursor,
    FutureCursor,
    ReplayPage,
    StoredEvent,
)
from scyg_agent.domain.runs import CommandId, EventId, RunId, RunSucceeded

RUN_ID = RunId("run_00000001")


@pytest.fixture
def anyio_backend() -> str:
    """Use the asyncio backend used by asyncpg."""
    return "asyncio"


@final
class FakeListener:
    """Record listener lifecycle and inject one selected failure."""

    def __init__(
        self, *, add_error: bool = False, remove_error: bool = False, close_blocks: bool = False
    ) -> None:
        """Configure lifecycle failures and zero call counters."""
        self.add_error = add_error
        self.remove_error = remove_error
        self.close_blocks = close_blocks
        self.add_calls = 0
        self.remove_calls = 0
        self.close_calls = 0
        self.terminate_calls = 0
        self.closed = False
        self.callback: Callable[..., None] | None = None

    async def add_listener(self, channel: str, callback: Callable[..., None]) -> None:
        """Register or inject add failure."""
        del channel
        self.add_calls += 1
        if self.add_error:
            message = "add failed"
            raise RuntimeError(message)
        self.callback = callback

    async def remove_listener(self, channel: str, callback: Callable[..., None]) -> None:
        """Remove or inject remove failure."""
        del channel, callback
        self.remove_calls += 1
        if self.remove_error:
            message = "remove failed"
            raise RuntimeError(message)
        self.callback = None

    async def close(self) -> None:
        """Close immediately or block until terminated."""
        self.close_calls += 1
        if self.close_blocks:
            await anyio.sleep_forever()
        self.closed = True

    def terminate(self) -> None:
        """Abort a close that exceeded its bounded deadline."""
        self.terminate_calls += 1
        self.closed = True

    def is_closed(self) -> bool:
        """Expose final transport state."""
        return self.closed


async def _empty_replay(_run_id: RunId, _cursor: EventCursor, _limit: int) -> ReplayPage:
    """Return an empty durable page so subscription reaches listener wait."""
    return ReplayPage((), EventCursor(0), EventCursor(0))


def _resources(listener: FakeListener, *, close_timeout: float = 0.05) -> SubscriptionResources:
    """Build deterministic lifecycle dependencies around one fake listener."""

    async def factory(_dsn: str) -> ListenerConnection:
        return listener

    return SubscriptionResources(
        10, _empty_replay, "postgresql://unused", "scyg_t11_events", factory, close_timeout
    )


@pytest.mark.anyio
async def test_factory_failure_acquires_no_listener_resource() -> None:
    # Given: a factory that fails before returning a connection.
    async def failing_factory(_dsn: str) -> ListenerConnection:
        message = "factory failed"
        raise RuntimeError(message)

    resources = SubscriptionResources(
        10, _empty_replay, "postgresql://unused", "scyg_t11_events", failing_factory, 0.05
    )
    # Opening itself reports the factory error before readiness.
    with pytest.raises(RuntimeError, match="factory failed"):
        _ = await open_subscription(RUN_ID, EventCursor(0), resources)


@pytest.mark.anyio
@pytest.mark.parametrize("failure", ["add", "remove"])
async def test_add_or_remove_failure_still_closes_acquired_connection(failure: str) -> None:
    # Given: one acquired connection failing during add or remove.
    listener = FakeListener(add_error=failure == "add", remove_error=failure == "remove")
    # When: add fails, or the ready subscription is explicitly closed.
    if failure == "add":
        with pytest.raises(RuntimeError, match="add failed"):
            _ = await open_subscription(RUN_ID, EventCursor(0), _resources(listener))
    else:
        subscription = await open_subscription(RUN_ID, EventCursor(0), _resources(listener))
        with pytest.raises(RuntimeError, match="remove_listener"):
            await subscription.aclose()

    # Then: every acquired connection receives exactly one close attempt.
    assert listener.close_calls == 1
    assert listener.closed


@pytest.mark.anyio
async def test_cancellation_while_waiting_closes_connection_once() -> None:
    # Given: subscription waiting after listener registration.
    listener = FakeListener()
    subscription = await open_subscription(RUN_ID, EventCursor(0), _resources(listener))
    # When: its owner cancels the active wait.
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(subscription.next_event)
        await checkpoint()
        task_group.cancel_scope.cancel()
    await subscription.aclose()

    # Then: callback and connection are both released exactly once.
    assert listener.remove_calls == 1
    assert listener.close_calls == 1
    assert listener.closed


@pytest.mark.anyio
async def test_close_timeout_terminates_and_confirms_closed_state() -> None:
    # Given: a driver close operation that never returns.
    listener = FakeListener(close_blocks=True)
    subscription = await open_subscription(
        RUN_ID, EventCursor(0), _resources(listener, close_timeout=0.01)
    )
    await subscription.aclose()

    # Then: timeout aborts transport and confirms final closure without hanging.
    assert listener.close_calls == 1
    assert listener.terminate_calls == 1
    assert listener.is_closed()


@pytest.mark.anyio
async def test_open_registers_before_race_replay_and_closes_without_iteration() -> None:
    listener = FakeListener()
    calls = 0

    async def replay(_run_id: RunId, _cursor: EventCursor, _limit: int) -> ReplayPage:
        nonlocal calls
        calls += 1
        assert listener.add_calls == (0 if calls == 1 else 1)
        return ReplayPage((), EventCursor(0), EventCursor(0))

    resources = _resources(listener)
    subscription = await open_subscription(
        RUN_ID, EventCursor(0), replace(resources, replay=replay)
    )
    assert calls == 2
    await subscription.aclose()
    await subscription.aclose()
    assert listener.remove_calls == listener.close_calls == 1


@pytest.mark.anyio
@pytest.mark.parametrize("race", [False, True])
@pytest.mark.parametrize("kind", ["old", "future"])
async def test_cursor_gaps_are_rejected_before_open_returns(kind: str, *, race: bool) -> None:
    listener = FakeListener()
    calls = 0

    async def replay(
        _run_id: RunId, cursor: EventCursor, _limit: int
    ) -> ReplayPage | CursorTooOld | FutureCursor:
        nonlocal calls
        calls += 1
        if race and calls == 1:
            return ReplayPage((), EventCursor(0), EventCursor(0))
        if kind == "old":
            return CursorTooOld(cursor, EventCursor(3))
        return FutureCursor(cursor, EventCursor(0))

    with pytest.raises(SubscriptionCursorError):
        _ = await open_subscription(
            RUN_ID, EventCursor(1), replace(_resources(listener), replay=replay)
        )
    assert listener.add_calls == int(race)
    assert listener.close_calls == int(race)


@pytest.mark.anyio
async def test_omitted_cursor_starts_at_retained_floor_and_terminal_head_ends() -> None:
    listener = FakeListener()
    calls: list[int] = []

    terminal = StoredEvent(
        EventCursor(8),
        RunSucceeded(
            EventId("evt_00000001"),
            CommandId("cmd_00000001"),
            datetime(2026, 1, 1, tzinfo=UTC),
            RUN_ID,
            1,
        ),
    )

    async def replay(_run_id: RunId, cursor: EventCursor, _limit: int) -> ReplayPage | CursorTooOld:
        calls.append(cursor.sequence)
        if cursor.sequence == 0:
            return CursorTooOld(cursor, EventCursor(8))
        return ReplayPage((terminal,), EventCursor(8), EventCursor(8), terminal=True)

    subscription = await open_subscription(
        RUN_ID, None, replace(_resources(listener), replay=replay)
    )
    assert calls == [0, 7]
    assert await subscription.next_event() == terminal
    with pytest.raises(StopAsyncIteration):
        _ = await subscription.next_event()
    assert listener.close_calls == 1


@pytest.mark.anyio
async def test_idle_subscription_returns_heartbeat_timeout() -> None:
    listener = FakeListener()
    subscription = await open_subscription(RUN_ID, None, _resources(listener))
    assert await subscription.next_event(0.01) is None
    await subscription.aclose()


@pytest.mark.anyio
async def test_race_replay_buffers_commit_during_listener_registration() -> None:
    listener = FakeListener()
    stored = StoredEvent(
        EventCursor(1),
        RunSucceeded(
            EventId("evt_00000001"),
            CommandId("cmd_00000001"),
            datetime(2026, 1, 1, tzinfo=UTC),
            RUN_ID,
            1,
        ),
    )
    calls = 0

    async def replay(_run_id: RunId, _cursor: EventCursor, _limit: int) -> ReplayPage:
        nonlocal calls
        calls += 1
        if calls == 1:
            return ReplayPage((), EventCursor(0), EventCursor(0))
        assert listener.add_calls == 1
        return ReplayPage((stored,), EventCursor(1), EventCursor(1), terminal=True)

    subscription = await open_subscription(
        RUN_ID, EventCursor(0), replace(_resources(listener), replay=replay)
    )
    assert calls == 2
    assert await subscription.next_event() == stored
    with pytest.raises(StopAsyncIteration):
        _ = await subscription.next_event()
    assert calls == 2
    assert listener.close_calls == 1
