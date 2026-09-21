"""双生产 Worker 测试的 PostgreSQL 装配与断言."""

from datetime import UTC, datetime, timedelta
from hashlib import sha256

import anyio
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.journal_records import CommandRecord, EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.adapters.database.terminal_commit import PostgreSQLTerminalCommitter
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
from scyg_agent.domain.runs.repository import CancellationRequest, CreateRunRequest
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRouter
from scyg_agent.worker import Worker, WorkerConfig, WorkerDependencies, WorkerState

from .diagnostics import wait_for_completion
from .fixture_support import WorkerDatabaseFixture
from .runtime_support import DeepFailureAdapter, RuntimeProbe, SimpleBarrierAdapter

RUN_COUNT_SIMPLE = 8
RUN_COUNT_DEEP = 3
TERMINAL_KINDS = ("run_succeeded", "run_failed", "run_cancelled")


async def exercise_two_workers(database: WorkerDatabaseFixture) -> None:
    """驱动两个真实 Worker 完成混合队列生命周期."""
    sessions = database.session_factory()
    await seed_runs(sessions)
    first_repository = PostgreSQLRunRepository(sessions)
    second_repository = PostgreSQLRunRepository(sessions)
    probe = RuntimeProbe()
    router = RuntimeRouter(default_registry(SimpleBarrierAdapter(probe), DeepFailureAdapter(probe)))
    config = WorkerConfig(
        simple_capacity=4,
        deep_capacity=1,
        lease_duration=timedelta(milliseconds=500),
        renewal_fraction=0.2,
        poll_interval=timedelta(milliseconds=5),
        error_backoff=timedelta(milliseconds=100),
        drain_timeout=timedelta(milliseconds=50),
    )
    first = build_worker("worker_postgres01", first_repository, sessions, router, config)
    second = build_worker("worker_postgres02", second_repository, sessions, router, config)
    cancelled_runs: list[RunId] = []
    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(first.start)
        _ = tasks.start_soon(second.start)
        try:
            await probe.started.wait()
            before = await active_expiries(sessions)
            await anyio.sleep(0.15)
            after = await active_expiries(sessions)
            assert any(after[key] > expiry for key, expiry in before.items() if key in after)
            cancel_run = await leased_by(sessions, "worker_postgres02")
            cancelled_runs.append(cancel_run)
            _ = await first_repository.request_cancellation(
                CancellationRequest(cancel_run, clock())
            )
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
    router: RuntimeRouter,
    config: WorkerConfig,
) -> Worker:
    """使用两个生产持久化适配器构造 Worker."""
    return Worker(
        ExecutionOwnerId(owner),
        WorkerDependencies(
            repository,
            PostgreSQLTerminalCommitter(sessions, "agent_events"),
            clock,
        ),
        router,
        config,
    )


def clock() -> datetime:
    """返回注入 Worker 的 UTC 时钟."""
    return datetime.now(UTC)


async def seed_runs(sessions: async_sessionmaker[AsyncSession]) -> None:
    """通过生产仓储创建混合 Run, 并插入审计命令父事实."""
    repository = PostgreSQLRunRepository(sessions)
    specs = tuple((RuntimeKind.SIMPLE, TaskType.SUMMARY, i) for i in range(8)) + tuple(
        (RuntimeKind.DEEP, TaskType.RESEARCH, 8 + i) for i in range(3)
    )
    now = clock()
    for kind, task_type, index in specs:
        run_id = RunId(f"run_workerpg{index:03d}")
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
                RunInput("测试输入", f"article-{index}"),
            )
        )
        await seed_command_parents(sessions, run_id, now)


async def seed_command_parents(
    sessions: async_sessionmaker[AsyncSession], run_id: RunId, now: datetime
) -> None:
    """按父行优先顺序建立首次执行和一次恢复的命令事实."""
    async with sessions.begin() as session:
        for attempt in (1, 2):
            digest = sha256(f"{run_id}:{attempt}:worker".encode()).hexdigest()[:24]
            session.add(
                CommandRecord(
                    command_id=f"cmd_{digest}",
                    run_id=str(run_id),
                    expected_revision=attempt + 1,
                    sequence=attempt - 1,
                    kind="worker_terminal",
                    request_digest="a" * 64,
                    semantic_digest="b" * 64,
                    result_status="pending",
                    result_reference=None,
                    created_at=now,
                    completed_at=None,
                )
            )
        await session.flush()


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
    """返回指定 Worker 当前拥有的一个 SIMPLE Run."""
    async with sessions() as session:
        run_id = (
            await session.execute(
                select(RunRecord.run_id)
                .where(
                    RunRecord.lease_owner == owner,
                    RunRecord.runtime_kind == RuntimeKind.SIMPLE.value,
                )
                .limit(1)
            )
        ).scalar_one()
    return RunId(run_id)


async def assert_results(
    database: WorkerDatabaseFixture,
    probe: RuntimeProbe,
    cancelled_run: RunId,
) -> None:
    """验证并发、围栏、终态、事件和审计唯一性."""
    sessions = database.session_factory()
    for owner in (ExecutionOwnerId("worker_postgres01"), ExecutionOwnerId("worker_postgres02")):
        assert probe.maxima.get((owner, RuntimeKind.SIMPLE), 0) <= 4
        assert probe.maxima.get((owner, RuntimeKind.DEEP), 0) <= 1
    assert len({(visit.run_id, visit.attempt) for visit in probe.visits}) == len(probe.visits)
    assert {visit.kind for visit in probe.visits} == {RuntimeKind.SIMPLE, RuntimeKind.DEEP}
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
    terminal_counts = dict(event_rows)
    audit_counts = dict(audit_rows)
    assert len(final_rows) == RUN_COUNT_SIMPLE + RUN_COUNT_DEEP
    assert all(row.status in {"succeeded", "failed", "cancelled"} for row in final_rows)
    assert (
        next(row for row in final_rows if persisted_run_matches(row.run_id, cancelled_run)).status
        == "cancelled"
    )
    assert all(terminal_counts.get(row.run_id) == 1 for row in final_rows)
    assert all(audit_counts.get(row.run_id) == 1 for row in final_rows)
    store = database.event_store()
    snapshots = [await store.terminal_snapshot(RunId(row.run_id)) for row in final_rows]
    assert all(snapshot is not None for snapshot in snapshots)


def persisted_run_matches(persisted_run_id: str, run_id: RunId) -> bool:
    """通过 branded ID 边界解析持久化字符串后比较身份."""
    return RunId(persisted_run_id) == run_id
