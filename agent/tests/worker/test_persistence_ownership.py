"""Worker 取消与外部持久化资源所有权回归测试."""

from datetime import timedelta
from typing import final

import anyio
import pytest

from scyg_agent.domain.runs import ExecutionOwnerId, Run, RunId, RuntimeKind
from scyg_agent.domain.runs.repository import ClaimRequest, LeaseLost, RenewRequest, RunLease
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRouter
from scyg_agent.worker import Worker, WorkerConfig, WorkerDependencies
from scyg_agent.worker.executor import execute_lease
from tests.runtimes.fakes import FakeRuntimeAdapter

from .test_executor import NOW, BlockingAdapter, RecordingCommitter, lease, running_run


@final
class CancellationTrackingRepository:
    """模拟共享连接在续租取消时被提前关闭的持久化所有者."""

    def __init__(self, run: Run) -> None:
        self.run = run
        self.renew_entered = anyio.Event()
        self.release_renew = anyio.Event()
        self.connection_closed = False

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """单执行回归不经过调度认领."""
        raise AssertionError(request)

    async def get(self, run_id: RunId) -> Run:
        """返回共享持久化中的活动 Run."""
        assert run_id == self.run.id
        return self.run

    async def renew(self, request: RenewRequest) -> LeaseLost:
        """仅当 Worker 取消穿透事务边界时标记连接关闭."""
        _ = request
        self.renew_entered.set()
        try:
            await self.release_renew.wait()
        except anyio.get_cancelled_exc_class():
            self.connection_closed = True
            raise
        return LeaseLost(self.run.id)


@final
class ClaimCancellationRepository:
    """模拟可继续读取的外部共享 claim 事务所有者."""

    def __init__(self, run: Run) -> None:
        self.run = run
        self.claim_entered = anyio.Event()
        self.release_claim = anyio.Event()
        self.connection_closed = False

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """等待测试释放事务并记录取消是否穿透边界."""
        _ = request
        self.claim_entered.set()
        try:
            await self.release_claim.wait()
        except anyio.get_cancelled_exc_class():
            self.connection_closed = True
            raise
        return ()

    async def get(self, run_id: RunId) -> Run:
        """证明 Worker 取消后共享仓储仍可使用."""
        assert run_id == self.run.id
        return self.run

    async def renew(self, request: RenewRequest) -> LeaseLost:
        """无租约时不应续租."""
        raise AssertionError(request)


@pytest.mark.anyio
async def test_worker_cancellation_does_not_close_external_persistence() -> None:
    """Given 外部共享持久化, When Worker 停止, Then 活动事务先清理再取消."""
    run = running_run()
    repository = CancellationTrackingRepository(run)
    dependencies = WorkerDependencies(repository, RecordingCommitter(run), lambda: NOW)
    runtime_router = RuntimeRouter(
        default_registry(
            BlockingAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-external-db", RuntimeKind.DEEP)),
        )
    )

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(
            execute_lease,
            lease(),
            dependencies,
            runtime_router,
            WorkerConfig(lease_duration=timedelta(milliseconds=10), renewal_fraction=0.1),
        )
        await repository.renew_entered.wait()
        tasks.cancel_scope.cancel()
        repository.release_renew.set()

    assert repository.connection_closed is False
    assert await repository.get(run.id) == run


@pytest.mark.anyio
async def test_worker_cancellation_waits_for_claim_transaction_cleanup() -> None:
    """Given 活动 claim, When Worker 被取消, Then 仓储清理后仍可读取."""
    run = running_run()
    repository = ClaimCancellationRepository(run)
    runtime_router = RuntimeRouter(
        default_registry(
            FakeRuntimeAdapter(AdapterIdentity("simple-claim-owner", RuntimeKind.SIMPLE)),
            FakeRuntimeAdapter(AdapterIdentity("deep-claim-owner", RuntimeKind.DEEP)),
        )
    )
    worker = Worker(
        ExecutionOwnerId("worker_claimowner01"),
        WorkerDependencies(repository, RecordingCommitter(run), lambda: NOW),
        runtime_router,
        WorkerConfig(poll_interval=timedelta(milliseconds=1)),
    )

    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(worker.start)
        await repository.claim_entered.wait()
        tasks.cancel_scope.cancel()
        repository.release_claim.set()

    assert repository.connection_closed is False
    assert await repository.get(run.id) == run
