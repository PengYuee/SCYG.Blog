"""双生产 Worker 测试的 PostgreSQL 装配与断言."""

from datetime import UTC, datetime, timedelta
from uuid import uuid4

import anyio
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.journal_records import EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord
from scyg_agent.adapters.database.run_records import AgentRunResultRecord, RunRecord
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.adapters.database.run_request_source import PostgreSQLRunInputSource
from scyg_agent.adapters.database.terminal_commit import PostgreSQLTerminalCommitter
from scyg_agent.agents.contracts import (
    Capability,
    ChatInput,
    SearchInput,
    input_digest,
    input_payload,
)
from scyg_agent.agents.runner import AgentRunner
from scyg_agent.application.control import ControlApplication
from scyg_agent.application.event_subscription import EventSubscriptionService
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
from scyg_agent.domain.runs.repository import CreateRunRequest
from scyg_agent.worker import Worker, WorkerConfig, WorkerDependencies, WorkerState

from .diagnostics import wait_for_completion
from .fixture_support import WorkerDatabaseFixture
from .runtime_support import BarrierAgentRunner, RunnerProbe

RUN_COUNT_SIMPLE = 8
RUN_COUNT_DEEP = 3
TERMINAL_KINDS = ("run_succeeded", "run_failed", "run_cancelled")


async def exercise_two_workers(database: WorkerDatabaseFixture) -> None:
    """驱动两个真实 Worker 完成混合队列生命周期."""
    sessions = database.session_factory()
    await seed_runs(sessions)
    first_repository = PostgreSQLRunRepository(sessions)
    second_repository = PostgreSQLRunRepository(sessions)
    probe = RunnerProbe()
    runner = BarrierAgentRunner(probe, PostgreSQLRunInputSource(sessions))
    config = WorkerConfig(
        capacity=5,
        lease_duration=timedelta(milliseconds=500),
        renewal_fraction=0.2,
        poll_interval=timedelta(milliseconds=5),
        error_backoff=timedelta(milliseconds=100),
        drain_timeout=timedelta(milliseconds=50),
    )
    first = build_worker("worker_postgres01", first_repository, sessions, runner, config)
    second = build_worker("worker_postgres02", second_repository, sessions, runner, config)
    cancelled_runs: list[RunId] = []
    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(first.start)
        _ = tasks.start_soon(second.start)
        try:
            with anyio.fail_after(5):
                await probe.both_started.wait()
            before = await active_expiries(sessions)
            await anyio.sleep(0.15)
            after = await active_expiries(sessions)
            assert any(after[key] > expiry for key, expiry in before.items() if key in after)
            cancel_run = await leased_by(sessions, "worker_postgres02")
            cancelled_runs.append(cancel_run)
            control = ControlApplication(
                sessions, "agent_events", EventSubscriptionService(database.event_store())
            )
            _ = await control.cancel("user-worker-pg", str(uuid4()), str(cancel_run))
            await first.stop()
            probe.gate.set()
            await wait_for_completion(sessions, probe, first, second)
        finally:
            probe.gate.set()
            with anyio.CancelScope(shield=True):
                if first.state not in {WorkerState.NEW, WorkerState.STOPPED}:
                    await first.stop()
                if second.state not in {WorkerState.NEW, WorkerState.STOPPED}:
                    await second.stop()
    assert len(cancelled_runs) == 1
    await assert_results(database, probe, cancelled_runs[0])
    assert first.state is WorkerState.STOPPED
    assert second.state is WorkerState.STOPPED


def build_worker(
    owner: str,
    repository: PostgreSQLRunRepository,
    sessions: async_sessionmaker[AsyncSession],
    runner: AgentRunner,
    config: WorkerConfig,
) -> Worker:
    """使用两个生产持久化适配器构造 Worker."""
    return Worker(
        ExecutionOwnerId(owner),
        WorkerDependencies(
            repository,
            PostgreSQLTerminalCommitter(sessions, "agent_events"),
            clock,
            runner,
        ),
        config,
    )


def clock() -> datetime:
    """返回注入 Worker 的 UTC 时钟."""
    return datetime.now(UTC)


async def seed_runs(sessions: async_sessionmaker[AsyncSession]) -> None:
    """通过生产仓储创建混合 Run, 不伪造自主 Worker 的命令父记录."""
    repository = PostgreSQLRunRepository(sessions)
    specs = tuple((RuntimeKind.SIMPLE, TaskType.QUESTION, i) for i in range(8)) + tuple(
        (RuntimeKind.DEEP, TaskType.RESEARCH, 8 + i) for i in range(3)
    )
    now = clock()
    for kind, task_type, index in specs:
        run_id = RunId(f"run_workerpg{index:03d}")
        value = (
            ChatInput(message="测试输入")
            if kind is RuntimeKind.SIMPLE
            else SearchInput(query="测试输入")
        )
        run = Run(
            run_id,
            UserId("user-worker-pg"),
            task_type,
            RuntimeSelection(kind, "v1"),
            1,
            RunStatus.PENDING,
            now,
            now,
            0,
            None,
            None,
        )
        _ = await repository.create(
            CreateRunRequest(
                run,
                OperationId(f"worker-pg:{index}"),
                now,
                RunInput(
                    "测试输入",
                    f"article-{index}",
                    "chat" if kind is RuntimeKind.SIMPLE else "search",
                    "chat-v1" if kind is RuntimeKind.SIMPLE else "search-v1",
                    "v1",
                    "v1",
                    input_payload(value),
                    input_digest(value),
                    None,
                    str(run_id),
                    "standard",
                    "v1",
                ),
            )
        )


async def active_expiries(
    sessions: async_sessionmaker[AsyncSession],
) -> dict[str, datetime]:
    """读取活动租约的到期时间."""
    async with sessions() as session:
        rows = (
            await session.execute(
                select(RunRecord.run_id, RunRecord.lease_expires_at).where(
                    RunRecord.status == RunStatus.RUNNING.value
                )
            )
        ).tuples()
    return {run_id: expires_at for run_id, expires_at in rows if expires_at is not None}


async def leased_by(sessions: async_sessionmaker[AsyncSession], owner: str) -> RunId:
    """Return a Run currently owned by the specified Worker."""
    async with sessions() as session:
        run_id = (
            await session.execute(
                select(RunRecord.run_id)
                .where(
                    RunRecord.lease_owner == owner,
                )
                .limit(1)
            )
        ).scalar_one()
    return RunId(run_id)


async def assert_results(
    database: WorkerDatabaseFixture,
    probe: RunnerProbe,
    cancelled_run: RunId,
) -> None:
    """验证并发、围栏、终态、事件和审计唯一性."""
    sessions = database.session_factory()
    for owner in (ExecutionOwnerId("worker_postgres01"), ExecutionOwnerId("worker_postgres02")):
        assert 1 <= probe.maxima.get(owner, 0) <= 5
    assert len({(visit.run_id, visit.attempt) for visit in probe.visits}) == len(probe.visits)
    assert {visit.capability for visit in probe.visits} == {Capability.CHAT, Capability.SEARCH}
    assert any(
        visit.attempt == 2 and visit.owner == ExecutionOwnerId("worker_postgres02")
        for visit in probe.visits
    )
    async with sessions() as session:
        final_rows = tuple((await session.execute(select(RunRecord))).scalars())
        event_rows = (
            (
                await session.execute(
                    select(EventRecord.run_id, func.count(EventRecord.event_id))
                    .where(EventRecord.kind.in_(TERMINAL_KINDS))
                    .group_by(EventRecord.run_id)
                )
            )
            .tuples()
            .all()
        )
        audit_rows = (
            (
                await session.execute(
                    select(AuditEventRecord.run_id, func.count(AuditEventRecord.audit_id)).group_by(
                        AuditEventRecord.run_id
                    )
                )
            )
            .tuples()
            .all()
        )
        result_rows = tuple((await session.execute(select(AgentRunResultRecord))).scalars())
    terminal_counts = dict(event_rows)
    audit_counts = dict(audit_rows)
    assert len(final_rows) == RUN_COUNT_SIMPLE + RUN_COUNT_DEEP
    assert all(row.status in {"succeeded", "failed", "cancelled"} for row in final_rows)
    assert (
        next(row for row in final_rows if persisted_run_matches(row.run_id, cancelled_run)).status
        == "cancelled"
    )
    for row in final_rows:
        assert row.lease_owner is None
        assert row.lease_token is None
        assert row.lease_expires_at is None
        if row.run_id != str(cancelled_run):
            assert row.status == ("succeeded" if row.capability == "chat" else "failed")
    assert {row.run_id for row in result_rows} == {
        row.run_id for row in final_rows if row.status == "succeeded"
    }
    assert all(row.capability == "chat" for row in result_rows)
    assert all(
        row.result_payload == {"response": "Persisted worker result", "source_article_ids": []}
        for row in result_rows
    )
    assert all(terminal_counts.get(row.run_id) == 1 for row in final_rows)
    assert audit_counts.get(str(cancelled_run), 0) == 0
    assert all(
        audit_counts.get(row.run_id) == 1 for row in final_rows if row.run_id != str(cancelled_run)
    )
    store = database.event_store()
    snapshots = [await store.terminal_snapshot(RunId(row.run_id)) for row in final_rows]
    assert all(snapshot is not None for snapshot in snapshots)


def persisted_run_matches(persisted_run_id: str, run_id: RunId) -> bool:
    """通过 branded ID 边界解析持久化字符串后比较身份."""
    return RunId(persisted_run_id) == run_id
