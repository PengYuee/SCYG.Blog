"""Unified Recipe capacity, fair polling, and bounded Worker lifecycle tests."""

from datetime import UTC, datetime, timedelta
from typing import final, override
from uuid import UUID

import anyio
import pytest
from sqlalchemy.exc import SQLAlchemyError

from scyg_agent.domain.ports.terminal_commit import TerminalCommitRequest, TerminalCommitResult
from scyg_agent.domain.runs import ExecutionOwnerId, RunId, RuntimeKind
from scyg_agent.domain.runs.repository import (
    ClaimRequest,
    GetResult,
    RenewRequest,
    RenewResult,
    RunLease,
)
from scyg_agent.domain.runs.repository_values import LeaseToken
from scyg_agent.worker import (
    InvalidWorkerTransitionError,
    Worker,
    WorkerConfig,
    WorkerDependencies,
    WorkerState,
)

from .test_executor import StructuredRunner

NOW = datetime(2026, 7, 12, 18, tzinfo=UTC)


class QueueRepository:
    """按运行时维护测试队列并记录每次 claim 上限."""

    def __init__(self) -> None:
        self.queues: dict[RuntimeKind, list[RunLease]]
        self.queues = {
            RuntimeKind.SIMPLE: self._leases(RuntimeKind.SIMPLE, 8),
            RuntimeKind.DEEP: self._leases(RuntimeKind.DEEP, 3),
        }
        self.claim_limits: list[tuple[RuntimeKind, int]] = []

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """只返回请求容量内的租约."""
        self.claim_limits.append((request.runtime_kind, request.limit))
        queue = self.queues[request.runtime_kind]
        claimed = tuple(queue[: request.limit])
        del queue[: request.limit]
        return claimed

    async def get(self, run_id: RunId) -> GetResult:
        """本测试用替换执行器不会读取 Run."""
        raise AssertionError(run_id)

    async def renew(self, request: RenewRequest) -> RenewResult:
        """本测试用替换执行器不会续租."""
        raise AssertionError(request)

    @staticmethod
    def _leases(kind: RuntimeKind, count: int) -> list[RunLease]:
        """构造确定性租约集合."""
        return [
            RunLease(
                RunId(f"run_{kind.value}{index:04d}"),
                ExecutionOwnerId("worker_schedule01"),
                LeaseToken(UUID(int=index + (1 if kind is RuntimeKind.SIMPLE else 100))),
                2,
                1,
                NOW + timedelta(minutes=1),
            )
            for index in range(count)
        ]


class UnusedCommitter:
    """容量测试不进入终态提交."""

    async def commit(self, request: TerminalCommitRequest) -> TerminalCommitResult:
        """若被调用则说明测试替换失效."""
        raise AssertionError(request)


@final
class FailingClaimRepository(QueueRepository):
    """首次 claim 抛出数据库瞬时错误并通知测试."""

    def __init__(self) -> None:
        super().__init__()
        self.failed = anyio.Event()

    @override
    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        _ = request
        self.failed.set()
        raise SQLAlchemyError


@final
class DelayedClaimRepository(QueueRepository):
    """模拟最终成功但耗时超过退避间隔的真实数据库认领."""

    @override
    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """在返回合法租约前产生可区分的确定性延迟."""
        await anyio.sleep(0.05)
        return await super().claim(request)


@pytest.mark.anyio
async def test_mixed_queues_never_exceed_shared_capacity(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Both persisted queue kinds share the same bounded Recipe capacity."""
    repository = QueueRepository()
    active = {RuntimeKind.SIMPLE: 0, RuntimeKind.DEEP: 0}
    maxima = {RuntimeKind.SIMPLE: 0, RuntimeKind.DEEP: 0}
    completed = 0
    peak_total = 0
    first_wave = anyio.Event()
    all_done = anyio.Event()

    async def controlled_execute(
        lease: RunLease,
        _dependencies: WorkerDependencies,
        _config: WorkerConfig,
    ) -> None:
        nonlocal completed, peak_total
        kind = RuntimeKind.SIMPLE if "simple" in str(lease.run_id) else RuntimeKind.DEEP
        active[kind] += 1
        maxima[kind] = max(maxima[kind], active[kind])
        peak_total = max(peak_total, sum(active.values()))
        if sum(active.values()) == 4:
            first_wave.set()
        await first_wave.wait()
        active[kind] -= 1
        completed += 1
        if completed == 11:
            all_done.set()

    monkeypatch.setattr("scyg_agent.worker.service.execute_lease", controlled_execute)
    worker = Worker(
        ExecutionOwnerId("worker_schedule01"),
        WorkerDependencies(repository, UnusedCommitter(), lambda: NOW, StructuredRunner()),
        WorkerConfig(poll_interval=timedelta(milliseconds=1)),
    )
    fresh = Worker(
        ExecutionOwnerId("worker_schedule02"),
        WorkerDependencies(repository, UnusedCommitter(), lambda: NOW, StructuredRunner()),
    )
    with pytest.raises(InvalidWorkerTransitionError):
        await fresh.stop()

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(worker.start)
        await all_done.wait()
        await worker.stop()

    assert max(maxima.values()) <= 4
    assert completed == 11
    assert peak_total == 4
    assert all(limit <= 4 for _kind, limit in repository.claim_limits)
    assert {kind for kind, _limit in repository.claim_limits} == set(RuntimeKind)
    assert worker.state is WorkerState.STOPPED
    await worker.stop()
    with pytest.raises(InvalidWorkerTransitionError):
        await worker.start()


@pytest.mark.anyio
async def test_claim_database_error_uses_backoff_and_worker_remains_stoppable() -> None:
    """Given claim 瞬时失败, When 调度, Then 退避且仍可有界停止."""
    repository = FailingClaimRepository()
    worker = Worker(
        ExecutionOwnerId("worker_database01"),
        WorkerDependencies(repository, UnusedCommitter(), lambda: NOW, StructuredRunner()),
        WorkerConfig(error_backoff=timedelta(milliseconds=1)),
    )

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(worker.start)
        await repository.failed.wait()
        await worker.stop()

    assert worker.state is WorkerState.STOPPED


@pytest.mark.anyio
async def test_successful_slow_claim_is_not_treated_as_backoff(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Given 慢速合法认领, When 超过退避间隔, Then Worker 仍执行租约."""
    repository = DelayedClaimRepository()
    repository.queues[RuntimeKind.SIMPLE] = repository.queues[RuntimeKind.SIMPLE][:1]
    repository.queues[RuntimeKind.DEEP] = []
    executed = anyio.Event()

    async def observe_execute(
        lease: RunLease,
        dependencies: WorkerDependencies,
        config: WorkerConfig,
    ) -> None:
        _ = (lease, dependencies, config)
        executed.set()

    monkeypatch.setattr("scyg_agent.worker.service.execute_lease", observe_execute)
    worker = Worker(
        ExecutionOwnerId("worker_slowclaim01"),
        WorkerDependencies(repository, UnusedCommitter(), lambda: NOW, StructuredRunner()),
        WorkerConfig(
            poll_interval=timedelta(milliseconds=1),
            error_backoff=timedelta(milliseconds=5),
        ),
    )

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(worker.start)
        with anyio.fail_after(0.2):
            await executed.wait()
        await worker.stop()

    assert worker.state is WorkerState.STOPPED


@pytest.mark.anyio
async def test_terminal_database_error_isolated_to_one_execution(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Given 单执行提交失败, When 调度, Then Worker 退避后仍可停止."""
    repository = QueueRepository()
    repository.queues[RuntimeKind.SIMPLE] = repository.queues[RuntimeKind.SIMPLE][:1]
    repository.queues[RuntimeKind.DEEP] = []
    failed = anyio.Event()

    async def failing_execute(
        lease: RunLease,
        dependencies: WorkerDependencies,
        config: WorkerConfig,
    ) -> None:
        _ = (lease, dependencies, config)
        failed.set()
        raise SQLAlchemyError

    monkeypatch.setattr("scyg_agent.worker.service.execute_lease", failing_execute)
    worker = Worker(
        ExecutionOwnerId("worker_database02"),
        WorkerDependencies(repository, UnusedCommitter(), lambda: NOW, StructuredRunner()),
        WorkerConfig(error_backoff=timedelta(milliseconds=1)),
    )

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(worker.start)
        await failed.wait()
        await worker.stop()

    assert worker.state is WorkerState.STOPPED
