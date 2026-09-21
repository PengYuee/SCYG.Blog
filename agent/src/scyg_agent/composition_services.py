"""延迟服务器与 Worker 组件."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - Worker 固定 asyncio 后端。
from collections.abc import Callable
from typing import final

import anyio

from scyg_agent.lifecycle import ComponentDiagnostic, LifecycleComponent
from scyg_agent.worker import Worker


@final
class DeferredComponent:
    """前置资源就绪后创建并委托真实组件."""

    def __init__(self, name: str, factory: Callable[[], LifecycleComponent]) -> None:
        """保存安全名称与构造回调."""
        self._name, self._factory, self._component = name, factory, None

    @property
    def name(self) -> str:
        """返回稳定组件名."""
        return self._name

    async def start(self) -> None:
        """创建真实组件并等待启动."""
        component = self._factory()
        await component.start()
        self._component = component

    async def stop(self) -> None:
        """委托真实组件关闭."""
        if self._component is not None:
            await self._component.stop()

    async def probe(self) -> ComponentDiagnostic:
        """委托真实组件探针."""
        if self._component is None:
            return ComponentDiagnostic(component=self.name, ready=False, detail="未启动")
        return await self._component.probe()


@final
class WorkerResource:
    """拥有 Worker 任务并要求真实心跳."""

    def __init__(self, factory: Callable[[], Worker], timeout_seconds: float) -> None:
        """保存 Worker 工厂和心跳期限."""
        self._factory, self._timeout = factory, timeout_seconds
        self._worker: Worker | None = None
        self._task: asyncio.Task[None] | None = None

    @property
    def name(self) -> str:
        """返回稳定组件名."""
        return "worker"

    async def start(self) -> None:
        """启动 Worker 并等待首轮 claim."""
        worker = self._factory()
        self._worker = worker
        self._task = asyncio.create_task(worker.start())
        with anyio.fail_after(self._timeout):
            await worker.wait_for_heartbeat()

    async def stop(self) -> None:
        """停止认领并等待排空."""
        if self._worker is not None:
            await self._worker.stop()
        if self._task is not None:
            await self._task

    async def probe(self) -> ComponentDiagnostic:
        """仅在任务存活且已心跳时就绪."""
        worker, task = self._worker, self._task
        ready = worker is not None and worker.has_heartbeat and task is not None and not task.done()
        return ComponentDiagnostic(self.name, ready, "已就绪" if ready else "无心跳")
