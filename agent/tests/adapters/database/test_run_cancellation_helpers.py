"""No-endpoint tests for persisted cancellation repository paths."""

from datetime import UTC, datetime, timedelta
from uuid import UUID

import pytest
from sqlalchemy.ext.asyncio import async_sessionmaker

from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.domain.runs import ExecutionOwnerId, RunId, RunStatus
from scyg_agent.domain.runs.cancellation import (
    CancellationQueued,
    CancellationRequested,
    CancellationTerminal,
)
from scyg_agent.domain.runs.repository import (
    CancellationRequest,
    DataIntegrityError,
    LeaseGuard,
    LeaseLost,
    LeaseToken,
    NotFound,
    RenewRequest,
)
from tests.adapters.database.scripted_session_support import FakeResult, ScriptedSession, session

NOW = datetime(2026, 7, 12, 16, tzinfo=UTC)
RUN_ID = RunId("run_cancelhelp1")
OWNER = ExecutionOwnerId("worker_cancelhelp1")
TOKEN = LeaseToken(UUID("12345678-1234-5678-9234-567812345678"))


def _row(status: RunStatus) -> RunRecord:
    running = status is RunStatus.RUNNING
    return RunRecord(
        run_id=str(RUN_ID),
        owner_user_id="user-cancel",
        operation_id="op:cancel",
        task_type="summary",
        runtime_kind="simple",
        runtime_version="v1",
        revision=2 if running else 1,
        status=status.value,
        created_at=NOW - timedelta(minutes=1),
        updated_at=NOW - timedelta(minutes=1),
        attempt=1 if running else 0,
        next_attempt_at=NOW if status is RunStatus.PENDING else None,
        lease_owner=str(OWNER) if running else None,
        lease_token=TOKEN.value if running else None,
        lease_expires_at=NOW + timedelta(minutes=5) if running else None,
        cancellation_requested_at=None,
        pending_interaction_id=None,
        terminal_at=NOW if status is RunStatus.SUCCEEDED else None,
        terminal_metadata=None,
        error_code=None,
        error_message=None,
        error_metadata=None,
    )


def _repository() -> PostgreSQLRunRepository:
    return PostgreSQLRunRepository(async_sessionmaker())


@pytest.mark.anyio
async def test_cancellation_maps_missing_and_corrupt_rows(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = _repository()
    missing = await repository.request_cancellation_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(None)])),
        CancellationRequest(RUN_ID, NOW),
    )
    corrupt = _row(RunStatus.PENDING)
    corrupt.status = "future"
    invalid = await repository.request_cancellation_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(corrupt)])),
        CancellationRequest(RUN_ID, NOW),
    )

    assert isinstance(missing, NotFound)
    assert isinstance(invalid, DataIntegrityError)


@pytest.mark.anyio
async def test_queued_cancellation_is_idempotent(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = _repository()
    row = _row(RunStatus.PENDING)
    first = await repository.request_cancellation_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(row)])),
        CancellationRequest(RUN_ID, NOW),
    )
    second = await repository.request_cancellation_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(row)])),
        CancellationRequest(RUN_ID, NOW),
    )

    assert isinstance(first, CancellationQueued)
    assert isinstance(second, CancellationQueued)
    assert not first.replayed
    assert second.replayed


@pytest.mark.anyio
async def test_leased_and_terminal_cancellation_are_distinct(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = _repository()
    running = await repository.request_cancellation_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(_row(RunStatus.RUNNING))])),
        CancellationRequest(RUN_ID, NOW),
    )
    terminal = await repository.request_cancellation_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(_row(RunStatus.SUCCEEDED))])),
        CancellationRequest(RUN_ID, NOW),
    )

    assert isinstance(running, CancellationRequested)
    assert running.lease is not None
    assert running.lease.token == TOKEN
    assert isinstance(terminal, CancellationTerminal)


@pytest.mark.anyio
async def test_renew_observes_cancellation_before_extending(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = _repository()
    row = _row(RunStatus.RUNNING)
    row.cancellation_requested_at = NOW
    guard = LeaseGuard(RUN_ID, OWNER, TOKEN, 2, NOW)
    result = await repository.renew_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(row)])),
        RenewRequest(guard, timedelta(minutes=1)),
    )

    assert isinstance(result, CancellationRequested)
    assert result.lease is not None
    assert result.lease.expires_at == row.lease_expires_at


@pytest.mark.anyio
async def test_renew_missing_fence_is_lease_loss(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    repository = _repository()
    guard = LeaseGuard(RUN_ID, OWNER, TOKEN, 2, NOW)
    result = await repository.renew_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(None)])),
        RenewRequest(guard, timedelta(minutes=1)),
    )

    assert isinstance(result, LeaseLost)
