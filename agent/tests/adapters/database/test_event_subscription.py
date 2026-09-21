"""Deterministic subscription listener lifecycle matrix."""

from collections.abc import AsyncGenerator, Callable
from typing import final

import anyio
import pytest
from anyio.lowlevel import checkpoint

from scyg_agent.adapters.database.event_subscription import (
    ListenerConnection,
    SubscriptionResources,
    subscribe_events,
)
from scyg_agent.domain.ports.event_store import EventCursor, ReplayPage, StoredEvent
from scyg_agent.domain.runs import RunId

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
    subscription = subscribe_events(RUN_ID, EventCursor(0), resources)

    # When/Then: the original factory error propagates without cleanup of an unowned resource.
    with pytest.raises(RuntimeError, match="factory failed"):
        _ = await anext(subscription)


@pytest.mark.anyio
@pytest.mark.parametrize("failure", ["add", "remove"])
async def test_add_or_remove_failure_still_closes_acquired_connection(failure: str) -> None:
    # Given: one acquired connection failing during add or remove.
    listener = FakeListener(add_error=failure == "add", remove_error=failure == "remove")
    subscription = subscribe_events(RUN_ID, EventCursor(0), _resources(listener))

    # When: add fails, or a registered subscription is explicitly closed.
    if failure == "add":
        with pytest.raises(RuntimeError, match="add failed"):
            _ = await anext(subscription)
    else:
        async with anyio.create_task_group() as task_group:
            _ = task_group.start_soon(_advance, subscription)
            while listener.add_calls == 0:
                await checkpoint()
            task_group.cancel_scope.cancel()

    # Then: every acquired connection receives exactly one close attempt.
    assert listener.close_calls == 1
    assert listener.closed


@pytest.mark.anyio
async def test_cancellation_while_waiting_closes_connection_once() -> None:
    # Given: subscription waiting after listener registration.
    listener = FakeListener()
    subscription = subscribe_events(RUN_ID, EventCursor(0), _resources(listener))

    # When: its owner cancels and explicitly closes the async generator.
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(_advance, subscription)
        while listener.add_calls == 0:
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
    subscription = subscribe_events(
        RUN_ID, EventCursor(0), _resources(listener, close_timeout=0.01)
    )

    # When: the registered generator is cancelled and closed.
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(_advance, subscription)
        while listener.add_calls == 0:
            await checkpoint()
        task_group.cancel_scope.cancel()
    await subscription.aclose()

    # Then: timeout aborts transport and confirms final closure without hanging.
    assert listener.close_calls == 1
    assert listener.terminate_calls == 1
    assert listener.is_closed()


async def _advance(subscription: AsyncGenerator[StoredEvent]) -> None:
    """Advance an empty subscription until cancellation reaches its listener wait."""
    _ = await anext(subscription)
