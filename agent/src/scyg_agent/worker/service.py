"""公平、容量有界且可优雅停止的单进程 Worker."""

from typing import final

import anyio
from anyio.abc import TaskGroup
from sqlalchemy.exc import SQLAlchemyError

from scyg_agent.domain.runs import ExecutionOwnerId, RuntimeKind
from scyg_agent.domain.runs.repository import ClaimRequest, RunLease
from scyg_agent.runtimes.router import RuntimeRouter

from .config import WorkerConfig
from .contracts import WorkerDependencies
from .errors import InvalidWorkerTransitionError, WorkerState
from .executor import execute_lease
from .persistence import finish_persistence


@final
class Worker:
    """只拥有调度与执行任务, 不拥有注入的持久化资源."""

    def __init__(
        self,
        owner: ExecutionOwnerId,
        dependencies: WorkerDependencies,
        router: RuntimeRouter,
        config: WorkerConfig | None = None,
    ) -> None:
        """建立互不共享的 SIMPLE/DEEP 容量和停止信号."""
        resolved = config or WorkerConfig()
        self._owner = owner
        self._dependencies = dependencies
        self._router = router
        self._config = resolved
        self._state = WorkerState.NEW
        self._stop_requested = anyio.Event()
        self._stopped = anyio.Event()
        self._heartbeat = anyio.Event()
        self._drained = anyio.Event()
        self._drained.set()
        self._active = {RuntimeKind.SIMPLE: 0, RuntimeKind.DEEP: 0}
        self._limits = {
            RuntimeKind.SIMPLE: anyio.Semaphore(resolved.simple_capacity),
            RuntimeKind.DEEP: anyio.Semaphore(resolved.deep_capacity),
        }

    @property
    def state(self) -> WorkerState:
        """返回当前生命周期状态."""
        return self._state

    @property
    def has_heartbeat(self) -> bool:
        """报告调度循环是否完成过真实轮询."""
        return self._heartbeat.is_set()

    async def wait_for_heartbeat(self) -> None:
        """等待首轮数据库认领尝试完成."""
        await self._heartbeat.wait()

    async def start(self) -> None:
        """启动唯一调度循环并拥有全部子任务."""
        if self._state is not WorkerState.NEW:
            raise InvalidWorkerTransitionError(self._state, "start")
        self._state = WorkerState.RUNNING
        kinds = (RuntimeKind.SIMPLE, RuntimeKind.DEEP)
        index = 0
        try:
            async with anyio.create_task_group() as tasks:
                while not self._stop_requested.is_set():
                    kind = kinds[index]
                    index = (index + 1) % len(kinds)
                    claimed = await self._poll(kind, tasks)
                    self._heartbeat.set()
                    if not claimed:
                        await anyio.sleep(self._config.poll_interval.total_seconds())
                self._state = WorkerState.STOPPING
                with anyio.move_on_after(self._config.drain_timeout.total_seconds()) as scope:
                    await self._drained.wait()
                if scope.cancelled_caught:
                    tasks.cancel_scope.cancel()
        finally:
            self._state = WorkerState.STOPPED
            self._stopped.set()

    async def stop(self) -> None:
        """停止认领, 等待有界排空并允许并发调用共享结果."""
        if self._state is WorkerState.NEW:
            raise InvalidWorkerTransitionError(self._state, "stop")
        if self._state is WorkerState.STOPPED:
            return
        self._stop_requested.set()
        await self._stopped.wait()

    async def _poll(self, kind: RuntimeKind, tasks: TaskGroup) -> bool:
        """只按已预留空闲槽认领并立即启动."""
        capacity = (
            self._config.simple_capacity
            if kind is RuntimeKind.SIMPLE
            else self._config.deep_capacity
        )
        free = capacity - self._active[kind]
        if free <= 0:
            return False
        leases: tuple[RunLease, ...] = ()
        try:
            leases = await finish_persistence(
                self._dependencies.repository.claim(
                    ClaimRequest(
                        free,
                        self._owner,
                        kind,
                        self._dependencies.clock(),
                        self._config.lease_duration,
                    )
                )
            )
        except SQLAlchemyError:
            await anyio.sleep(self._config.error_backoff.total_seconds())
            return False
        if leases and not any(self._active.values()):
            self._drained = anyio.Event()
        self._active[kind] += len(leases)
        for lease in leases:
            _ = tasks.start_soon(self._execute, kind, lease)
        return bool(leases)

    async def _execute(self, kind: RuntimeKind, lease: RunLease) -> None:
        """占用对应信号量并在所有退出路径释放预留."""
        async with self._limits[kind]:
            try:
                try:
                    await execute_lease(lease, self._dependencies, self._router, self._config)
                except SQLAlchemyError:
                    await anyio.sleep(self._config.error_backoff.total_seconds())
            finally:
                self._active[kind] -= 1
                if not any(self._active.values()):
                    self._drained.set()
