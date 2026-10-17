"""公平、容量有界且可优雅停止的单进程 Worker."""

from contextlib import suppress
from datetime import timedelta
from typing import final

import anyio
from anyio.abc import TaskGroup
from sqlalchemy.exc import SQLAlchemyError

from scyg_agent.adapters.redis.public_controls import PublicControlsUnavailableError
from scyg_agent.domain.runs import ContentScope, ExecutionOwnerId, RuntimeKind
from scyg_agent.domain.runs.repository import ClaimRequest, RunLease

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
        config: WorkerConfig | None = None,
    ) -> None:
        """建立统一 Recipe 容量和停止信号."""
        resolved = config or WorkerConfig()
        self._owner = owner
        self._dependencies = dependencies
        self._config = resolved
        self._state = WorkerState.NEW
        self._stop_requested = anyio.Event()
        self._stopped = anyio.Event()
        self._heartbeat = anyio.Event()
        self._drained = anyio.Event()
        self._drained.set()
        self._active = 0
        self._limit = anyio.Semaphore(resolved.capacity)

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
                    public_claimed = await self._poll_public(tasks)
                    claimed = await self._poll(kind, tasks) or public_claimed
                    # Public gets first opportunity without monopolizing author capacity.
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
        free = self._config.capacity - self._active
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
                        content_scope=ContentScope.AUTHOR,
                    )
                )
            )
        except SQLAlchemyError:
            await anyio.sleep(self._config.error_backoff.total_seconds())
            return False
        if leases and not self._active:
            self._drained = anyio.Event()
        self._active += len(leases)
        for lease in leases:
            _ = tasks.start_soon(self._execute, lease)
        return bool(leases)

    async def _renew_public(
        self, token: str, scope: anyio.CancelScope, alive: dict[str, bool]
    ) -> None:
        controls = self._dependencies.public_controls
        if controls is None:
            return
        while True:
            await anyio.sleep(controls.settings.heartbeat_seconds)
            try:
                valid = await controls.renew_execution(token)
            except PublicControlsUnavailableError:
                valid = False
            if not valid:
                alive["valid"] = False
                scope.cancel()
                return

    async def _release_public(self, token: str) -> None:
        controls = self._dependencies.public_controls
        if controls is None:
            return
        with anyio.CancelScope(shield=True):
            # Failed release expires; never release another holder's lease.
            with suppress(PublicControlsUnavailableError):
                await controls.release_execution(token)

    async def _poll_public(self, tasks: TaskGroup) -> bool:
        controls = self._dependencies.public_controls
        if controls is None or self._active >= self._config.capacity - 1:
            return False
        try:
            token = await controls.acquire_execution()
        except PublicControlsUnavailableError:
            return False
        if token is None:
            return False
        transferred = False
        alive = {"valid": True}
        try:
            with anyio.CancelScope() as scope:
                async with anyio.create_task_group() as renewal:
                    _ = renewal.start_soon(self._renew_public, token, scope, alive)
                    try:
                        leases = await finish_persistence(
                            self._dependencies.repository.claim(
                                ClaimRequest(
                                    1,
                                    self._owner,
                                    RuntimeKind.DEEP,
                                    self._dependencies.clock(),
                                    self._config.lease_duration,
                                    content_scope=ContentScope.PUBLIC,
                                    public_timeout=timedelta(
                                        seconds=controls.settings.run_timeout_seconds
                                    ),
                                )
                            )
                        )
                        if leases and alive["valid"] and await controls.renew_execution(token):
                            if not self._active:
                                self._drained = anyio.Event()
                            self._active += 1
                            _ = tasks.start_soon(self._execute_public, leases[0], token)
                            transferred = True
                    except (SQLAlchemyError, PublicControlsUnavailableError):
                        return False
                    finally:
                        renewal.cancel_scope.cancel()
        finally:
            if not transferred:
                await self._release_public(token)
        return transferred

    async def _execute_public(self, lease: RunLease, token: str) -> None:
        alive = {"valid": True}
        started = False
        try:
            with anyio.CancelScope() as scope:
                async with anyio.create_task_group() as renewal:
                    _ = renewal.start_soon(self._renew_public, token, scope, alive)
                    try:
                        controls = self._dependencies.public_controls
                        if controls is not None and await controls.renew_execution(token):
                            started = True
                            await self._execute(lease)
                    except PublicControlsUnavailableError:
                        pass
                    finally:
                        renewal.cancel_scope.cancel()
        finally:
            if not started:
                self._active -= 1
                if not self._active:
                    self._drained.set()
            # All runner children have exited before freeing the distributed slot.
            await self._release_public(token)

    async def _execute(self, lease: RunLease) -> None:
        """占用统一信号量并在所有退出路径释放预留."""
        try:
            async with self._limit:
                try:
                    await execute_lease(lease, self._dependencies, self._config)
                except SQLAlchemyError:
                    await anyio.sleep(self._config.error_backoff.total_seconds())
        finally:
            self._active -= 1
            if not self._active:
                self._drained.set()
