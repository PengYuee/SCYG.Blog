"""Real PostgreSQL monotonic renewal regression tests."""

from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta

import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.domain.runs import (
    ExecutionOwnerId,
    OperationId,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.domain.runs.repository import (
    ClaimRequest,
    CreateRunRequest,
    LeaseGuard,
    LeaseLost,
    Renewed,
    RenewRequest,
)
from tests.acceptance_settings import require_test_settings

NOW = datetime(2026, 7, 12, 9, 0, tzinfo=UTC)


@pytest.fixture
def anyio_backend() -> str:
    """Use the asyncio backend required by SQLAlchemy asyncpg."""
    return "asyncio"


@pytest.fixture
async def repository() -> AsyncIterator[PostgreSQLRunRepository]:
    """Create a clean repository over the shared PostgreSQL database."""
    database_url = require_test_settings().normal_url
    engine = create_async_engine(database_url, pool_size=2, max_overflow=0, pool_timeout=5)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    async with engine.begin() as connection:
        _ = await connection.execute(text("TRUNCATE agent_runs CASCADE"))
    try:
        yield PostgreSQLRunRepository(sessions)
    finally:
        await engine.dispose()


@pytest.mark.anyio
async def test_renew_is_monotonic_and_exact_expiry_is_lost(
    repository: PostgreSQLRunRepository,
) -> None:
    # Given: a claimed lease with ten minutes remaining.
    run = Run(
        RunId("run_renew0000"),
        UserId("user-t10"),
        TaskType.SUMMARY,
        RuntimeSelection(RuntimeKind.SIMPLE, "v1"),
        1,
        RunStatus.PENDING,
        NOW,
        NOW,
        0,
        None,
        None,
    )
    _ = await repository.create(
        CreateRunRequest(run, OperationId("t10:renew"), NOW, RunInput("测试输入", "article-1"))
    )
    lease = (
        await repository.claim(
            ClaimRequest(
                1,
                ExecutionOwnerId("worker_renew000"),
                RuntimeKind.SIMPLE,
                NOW,
                timedelta(minutes=10),
            )
        )
    )[0]
    guard = LeaseGuard(
        lease.run_id,
        lease.owner,
        lease.token,
        lease.revision,
        NOW + timedelta(minutes=1),
    )

    # When: first a shorter candidate, then a true extension, then exact expiry is used.
    preserved = await repository.renew(RenewRequest(guard, timedelta(minutes=1)))
    extended = await repository.renew(RenewRequest(guard, timedelta(minutes=15)))

    # Then: persisted expiry never shrinks, grows normally, and boundary renewal loses.
    assert isinstance(preserved, Renewed)
    assert preserved.lease.expires_at == lease.expires_at
    assert isinstance(extended, Renewed)
    assert extended.lease.expires_at == guard.now + timedelta(minutes=15)
    boundary_guard = LeaseGuard(
        lease.run_id,
        lease.owner,
        lease.token,
        lease.revision,
        extended.lease.expires_at,
    )
    assert isinstance(
        await repository.renew(RenewRequest(boundary_guard, timedelta(minutes=1))), LeaseLost
    )
