"""Endpoint-guarded PostgreSQL tests for terminal atomicity and races."""

from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from typing import final

import anyio
import pytest
from sqlalchemy import func, select, text
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from scyg_agent.adapters.database.journal_records import CommandRecord, EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.adapters.database.terminal_commit import (
    PostgreSQLTerminalCommitter,
    TerminalStage,
)
from scyg_agent.domain.ports.audit_store import AuditFact
from scyg_agent.domain.ports.event_store import AppendRequest
from scyg_agent.domain.ports.idempotency import AuditMetadata
from scyg_agent.domain.ports.terminal_commit import (
    TerminalCancellationRequested,
    TerminalCommitRequest,
    TerminalCommitted,
    TerminalReplay,
)
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    ExecutionOwnerId,
    RunId,
    RunStatus,
    RunSucceeded,
    RuntimeKind,
    UserId,
)
from scyg_agent.domain.runs.repository import (
    CancellationRequest,
    ClaimRequest,
    CompletionRequest,
    LeaseGuard,
    RunLease,
)
from tests.acceptance_settings import require_test_settings

NOW = datetime(2026, 7, 12, 15, tzinfo=UTC)
LEASE_DURATION = timedelta(minutes=5)
TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""


class InjectedTerminalFailureError(RuntimeError):
    pass


@final
class FailAt:
    def __init__(self, target: TerminalStage) -> None:
        self.target: TerminalStage = target
        self.reached: list[TerminalStage] = []

    async def reach(self, stage: TerminalStage) -> None:
        self.reached.append(stage)
        if stage is self.target:
            raise InjectedTerminalFailureError


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.fixture
async def terminal_environment() -> AsyncIterator[
    tuple[async_sessionmaker[AsyncSession], AsyncEngine]
]:
    database_url = require_test_settings().normal_url
    engine = create_async_engine(database_url, pool_size=5, max_overflow=0, pool_timeout=5)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    async with engine.begin() as connection:
        _ = await connection.execute(text(TRUNCATE_SQL))
    try:
        yield sessions, engine
    finally:
        await engine.dispose()


@pytest.mark.anyio
@pytest.mark.parametrize("stage", list(TerminalStage))
async def test_failure_at_every_terminal_write_stage_rolls_back_all_facts(
    terminal_environment: tuple[async_sessionmaker[AsyncSession], AsyncEngine],
    stage: TerminalStage,
) -> None:
    sessions, _ = terminal_environment
    lease, request = await _seed_request(sessions, "rollback")
    failpoint = FailAt(stage)
    committer = PostgreSQLTerminalCommitter(sessions, "agent_events", failpoint)

    with pytest.raises(InjectedTerminalFailureError):
        _ = await committer.commit(request)

    async with sessions() as session:
        row = (
            await session.execute(select(RunRecord).where(RunRecord.run_id == str(lease.run_id)))
        ).scalar_one()
        event_count = (await session.execute(select(func.count(EventRecord.event_id)))).scalar_one()
        audit_count = (
            await session.execute(select(func.count(AuditEventRecord.audit_id)))
        ).scalar_one()
        command_count = (
            await session.execute(select(func.count(CommandRecord.command_id)))
        ).scalar_one()
    assert row.status == RunStatus.RUNNING.value
    assert row.lease_token == lease.token.value
    assert event_count == 0
    assert audit_count == 0
    assert command_count == 1
    assert stage in failpoint.reached


@pytest.mark.anyio
async def test_concurrent_terminal_commit_has_one_commit_and_one_replay(
    terminal_environment: tuple[async_sessionmaker[AsyncSession], AsyncEngine],
) -> None:
    sessions, _ = terminal_environment
    _, request = await _seed_request(sessions, "concurrent")
    first = PostgreSQLTerminalCommitter(sessions, "agent_events")
    second = PostgreSQLTerminalCommitter(sessions, "agent_events")
    results: list[TerminalCommitted | TerminalReplay] = []

    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(_commit_into, first, request, results)
        _ = task_group.start_soon(_commit_into, second, request, results)

    assert len(results) == 2
    assert sum(isinstance(result, TerminalCommitted) for result in results) == 1
    assert sum(isinstance(result, TerminalReplay) for result in results) == 1
    async with sessions() as session:
        audit_count = (
            await session.execute(select(func.count(AuditEventRecord.audit_id)))
        ).scalar_one()
        command_count = (
            await session.execute(select(func.count(CommandRecord.command_id)))
        ).scalar_one()
    assert audit_count == 1
    assert command_count == 1


@pytest.mark.anyio
async def test_cancellation_wins_race_until_owner_commits_cancelled(
    terminal_environment: tuple[async_sessionmaker[AsyncSession], AsyncEngine],
) -> None:
    sessions, _ = terminal_environment
    lease, request = await _seed_request(sessions, "cancel")
    repository = PostgreSQLRunRepository(sessions)
    _ = await repository.request_cancellation(CancellationRequest(lease.run_id, NOW))
    committer = PostgreSQLTerminalCommitter(sessions, "agent_events")

    result = await committer.commit(request)

    assert isinstance(result, TerminalCancellationRequested)


async def _commit_into(
    committer: PostgreSQLTerminalCommitter,
    request: TerminalCommitRequest,
    results: list[TerminalCommitted | TerminalReplay],
) -> None:
    result = await committer.commit(request)
    assert isinstance(result, (TerminalCommitted, TerminalReplay))
    results.append(result)


async def _seed_request(
    sessions: async_sessionmaker[AsyncSession], suffix: str
) -> tuple[RunLease, TerminalCommitRequest]:
    repository = PostgreSQLRunRepository(sessions)
    run_id = RunId(f"run_terminal_{suffix}")
    command_id = CommandId(f"cmd_terminal_{suffix}")
    await _seed_run_and_command(sessions, run_id, command_id, suffix)
    lease = (
        await repository.claim(
            ClaimRequest(
                1,
                ExecutionOwnerId(f"worker_terminal_{suffix}"),
                RuntimeKind.SIMPLE,
                NOW,
                LEASE_DURATION,
            )
        )
    )[0]
    event = RunSucceeded(
        EventId(f"evt_terminal_{suffix}"),
        command_id,
        NOW,
        run_id,
        lease.revision + 1,
    )
    audit = AuditFact(
        f"audit-terminal-{suffix}",
        run_id,
        UserId("user-terminal"),
        command_id,
        None,
        "terminal_commit",
        "succeeded",
        NOW,
        AuditMetadata("source", "worker"),
    )
    request = TerminalCommitRequest(
        CompletionRequest(
            LeaseGuard(run_id, lease.owner, lease.token, lease.revision, NOW),
            RunStatus.SUCCEEDED,
        ),
        AppendRequest(run_id, (event,)),
        audit,
    )
    assert request.audit.command_id == command_id
    assert request.audit.run_id == run_id
    return lease, request


async def _seed_run_and_command(
    sessions: async_sessionmaker[AsyncSession],
    run_id: RunId,
    command_id: CommandId,
    suffix: str,
) -> None:
    async with sessions.begin() as session:
        run_record = RunRecord(
            run_id=str(run_id),
            owner_user_id="user-terminal",
            operation_id=f"terminal:{suffix}",
            task_type="summary",
            runtime_kind="simple",
            runtime_version="v1",
            revision=1,
            status="pending",
            created_at=NOW - timedelta(minutes=1),
            updated_at=NOW - timedelta(minutes=1),
            attempt=0,
            next_attempt_at=NOW,
            lease_owner=None,
            lease_token=None,
            lease_expires_at=None,
            cancellation_requested_at=None,
            pending_interaction_id=None,
            terminal_at=None,
            terminal_metadata=None,
            error_code=None,
            error_message=None,
            error_metadata=None,
        )
        session.add(run_record)
        await session.flush()
        command_record = CommandRecord(
            command_id=str(command_id),
            run_id=str(run_id),
            expected_revision=1,
            sequence=0,
            kind="terminal_commit",
            request_digest="a" * 64,
            semantic_digest="b" * 64,
            result_status="pending",
            result_reference=None,
            created_at=NOW,
            completed_at=None,
        )
        session.add(command_record)
        await session.flush()
        assert command_record.run_id == run_record.run_id
        assert command_record.command_id == str(command_id)
