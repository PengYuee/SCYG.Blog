"""The RPC factory runs under the Run lock; network waiting never does."""

from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager
from datetime import timedelta
from typing import cast
from uuid import UUID

import pytest
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.agents.execution import ExecutionLeaseLostError, PostgreSQLExecutionDispatch
from scyg_agent.domain.runs import ExecutionOwnerId
from scyg_agent.domain.runs.repository import LeaseGuard, LeaseToken
from tests.adapters.database.scripted_session_support import (
    NOW,
    RUN_ID,
    FakeResult,
    ScriptedSession,
    run,
    session,
)


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class Transactions:
    def __init__(self, value: AsyncSession) -> None:
        self.value: AsyncSession = value
        self.active: bool = False
        self.events: list[str] = []

    @asynccontextmanager
    async def begin(self) -> AsyncIterator[AsyncSession]:
        self.active = True
        self.events.append("begin")
        try:
            yield self.value
        finally:
            self.active = False
            self.events.append("commit")

    def factory(self) -> async_sessionmaker[AsyncSession]:
        return cast("async_sessionmaker[AsyncSession]", cast("object", self))


def lease_guard() -> LeaseGuard:
    return LeaseGuard(RUN_ID, ExecutionOwnerId("worker_dispatch"), LeaseToken(UUID(int=1)), 1, NOW)


def live_run() -> RunRecord:
    record = run("running")
    guard = lease_guard()
    record.lease_owner = str(guard.owner)
    record.lease_token = guard.token.value
    record.lease_expires_at = NOW + timedelta(minutes=5)
    return record


def rpc_factory(transactions: Transactions) -> Callable[[], Awaitable[str]]:
    async def response() -> str:
        assert not transactions.active
        transactions.events.append("await")
        return "response"

    def start() -> Awaitable[str]:
        assert transactions.active
        transactions.events.append("start")
        return response()

    return start


@pytest.mark.anyio
async def test_gateway_factory_starts_before_commit_and_awaits_after(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    value = session(monkeypatch, ScriptedSession([FakeResult(live_run())]))
    transactions = Transactions(value)
    dispatch = PostgreSQLExecutionDispatch(transactions.factory(), lease_guard(), lambda: NOW)

    assert await dispatch.dispatch(rpc_factory(transactions)) == "response"
    assert transactions.events == ["begin", "start", "commit", "await"]


@pytest.mark.anyio
@pytest.mark.parametrize(
    "lost",
    ["cancelled", "owner", "token", "revision", "expired", "requested"],
)
async def test_cancelled_or_lost_worker_never_starts_rpc(
    monkeypatch: pytest.MonkeyPatch,
    lost: str,
) -> None:
    record = live_run()
    if lost == "cancelled":
        record.status = "cancelled"
    elif lost == "owner":
        record.lease_owner = "worker_other0001"
    elif lost == "token":
        record.lease_token = UUID(int=2)
    elif lost == "revision":
        record.revision = 2
    elif lost == "expired":
        record.lease_expires_at = NOW
    else:
        record.cancellation_requested_at = NOW
    value = session(monkeypatch, ScriptedSession([FakeResult(record)]))
    transactions = Transactions(value)
    dispatch = PostgreSQLExecutionDispatch(transactions.factory(), lease_guard(), lambda: NOW)

    with pytest.raises(ExecutionLeaseLostError):
        _ = await dispatch.dispatch(rpc_factory(transactions))
    assert "start" not in transactions.events


@pytest.mark.anyio
async def test_clock_is_sampled_after_row_lock(monkeypatch: pytest.MonkeyPatch) -> None:
    record = live_run()
    value = session(monkeypatch, ScriptedSession([FakeResult(record)]))
    transactions = Transactions(value)
    dispatch = PostgreSQLExecutionDispatch(
        transactions.factory(), lease_guard(), lambda: NOW + timedelta(minutes=5)
    )

    with pytest.raises(ExecutionLeaseLostError):
        _ = await dispatch.dispatch(rpc_factory(transactions))
    assert "start" not in transactions.events
