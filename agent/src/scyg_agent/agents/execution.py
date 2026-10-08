"""Fence synchronous RPC start factories with the authoritative Run row lock."""

from collections.abc import Awaitable, Callable, Coroutine
from datetime import datetime
from typing import Protocol

from grpc import aio
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.domain.runs.repository import LeaseGuard


class ExecutionLeaseLostError(RuntimeError):
    """Reject dispatch after cancellation, expiry, or ownership loss."""


class ExecutionDispatch(Protocol):
    """Start RPC calls under the active Worker lease fence."""

    async def dispatch[T](self, start: Callable[[], Awaitable[T]]) -> T:
        """Create the network Call under the lock and await it after commit."""
        ...


async def lock_execution(
    session: AsyncSession, guard: LeaseGuard, clock: Callable[[], datetime] | None = None
) -> None:
    """Share cancellation's Run lock and require the current live lease."""
    run = (
        await session.execute(
            select(RunRecord).where(RunRecord.run_id == str(guard.run_id)).with_for_update()
        )
    ).scalar_one_or_none()
    now = clock() if clock is not None else guard.now
    if (
        run is None
        or run.status != "running"
        or run.cancellation_requested_at is not None
        or run.lease_owner != str(guard.owner)
        or run.lease_token != guard.token.value
        or run.revision != guard.expected_revision
        or run.lease_expires_at is None
        or run.lease_expires_at <= now
    ):
        raise ExecutionLeaseLostError


class PostgreSQLExecutionDispatch:
    """Bind a Worker lease to every Blog RPC in one graph invocation."""

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        guard: LeaseGuard,
        clock: Callable[[], datetime],
    ) -> None:
        """Bind the mandatory session factory, lease identity, and current clock."""
        self._sessions: async_sessionmaker[AsyncSession] = sessions
        self._guard: LeaseGuard = guard
        self._clock: Callable[[], datetime] = clock

    async def check(self) -> None:
        """Reject a cancelled or lost lease before admitting graph execution."""
        async with self._sessions.begin() as session:
            await lock_execution(session, self._guard, self._clock)

    async def dispatch[T](self, start: Callable[[], Awaitable[T]]) -> T:
        """Start synchronously under the Run lock, then await outside the transaction."""
        pending: Awaitable[T] | None = None
        committed = False
        try:
            async with self._sessions.begin() as session:
                await lock_execution(session, self._guard, self._clock)
                call = start()
                pending = call
            committed = True
        finally:
            if not committed:
                if isinstance(pending, Coroutine):
                    pending.close()
                elif isinstance(pending, aio.Call):
                    # Production starters return grpc.aio Calls, whose cancel is synchronous.
                    _ = pending.cancel()
        return await call
