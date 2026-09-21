"""Agent 进程的类型化生命周期与就绪状态."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - 生产传输固定使用 asyncio 后端。
from dataclasses import dataclass
from enum import StrEnum
from typing import Protocol, final, override

import anyio


class ApplicationState(StrEnum):
    """限定应用允许的生命周期状态."""

    NEW = "new"
    STARTING = "starting"
    RUNNING = "running"
    STOPPING = "stopping"
    STOPPED = "stopped"
    FAILED = "failed"


@dataclass(frozen=True, slots=True)
class ComponentDiagnostic:
    """返回不含配置值的单组件就绪诊断."""

    component: str
    ready: bool
    detail: str


@dataclass(frozen=True, slots=True)
class ReadinessReport:
    """聚合所有已启用组件的就绪结果."""

    ready: bool
    components: tuple[ComponentDiagnostic, ...]


@dataclass(frozen=True, slots=True)
class LivenessReport:
    """仅报告进程生命周期, 不探测下游."""

    alive: bool


class LifecycleComponent(Protocol):
    """定义生产资源统一的启动、探测和关闭能力."""

    @property
    def name(self) -> str:
        """返回安全稳定的组件名称."""
        ...

    async def start(self) -> None:
        """启动并确认组件已接受工作."""
        ...

    async def stop(self) -> None:
        """停止接受工作并释放组件资源."""
        ...

    async def probe(self) -> ComponentDiagnostic:
        """返回不包含配置的组件诊断."""
        ...


@dataclass(frozen=True, slots=True)
class LifecycleComponents:
    """按权威启动顺序保存可选生产组件."""

    enabled: tuple[LifecycleComponent, ...]


@dataclass(frozen=True, slots=True)
class InvalidApplicationTransitionError(RuntimeError):
    """拒绝不安全的应用生命周期转换."""

    state: ApplicationState
    operation: str

    @override
    def __str__(self) -> str:
        return f"应用生命周期转换无效: {self.state.value}/{self.operation}"


@final
class AgentApplication:
    """严格顺序启动并以共享屏蔽任务逆序关闭全部资源."""

    def __init__(self, components: LifecycleComponents, shutdown_seconds: float) -> None:
        """建立尚未启动的单进程组合."""
        self._components = components.enabled
        self._shutdown_seconds = shutdown_seconds
        self._state = ApplicationState.NEW
        self._opened: list[LifecycleComponent] = []
        self._stop_lock = asyncio.Lock()
        self._stop_task: asyncio.Task[None] | None = None
        self._close_order: list[str] = []

    @property
    def state(self) -> ApplicationState:
        """返回当前生命周期状态."""
        return self._state

    @property
    def component_names(self) -> tuple[str, ...]:
        """返回脱敏组件启动计划."""
        return tuple(component.name for component in self._components)

    @property
    def close_order(self) -> tuple[str, ...]:
        """返回已完成关闭的脱敏组件顺序."""
        return tuple(self._close_order)

    async def start(self) -> None:
        """依次启动组件, 失败时逆序清理已成功资源."""
        if self._state is ApplicationState.RUNNING:
            return
        if self._state is not ApplicationState.NEW:
            raise InvalidApplicationTransitionError(self._state, "start")
        self._state = ApplicationState.STARTING
        try:
            for component in self._components:
                await component.start()
                self._opened.append(component)
            report = await self.readiness()
            self._require_ready(report)
            self._state = ApplicationState.RUNNING
        except BaseException:  # noqa: RUF100  # noqa: BROAD_EXCEPT_OK - 组合根清理任意启动失败后原样传播。
            self._state = ApplicationState.FAILED
            await self._request_cleanup()
            self._state = ApplicationState.FAILED
            raise

    def _require_ready(self, report: ReadinessReport) -> None:
        """拒绝尚未通过全部组件探针的启动."""
        if not report.ready:
            raise InvalidApplicationTransitionError(self._state, "ready")

    def liveness(self) -> LivenessReport:
        """不访问下游地报告进程是否仍可服务探针."""
        return LivenessReport(
            self._state not in {ApplicationState.STOPPED, ApplicationState.FAILED}
        )

    async def readiness(self) -> ReadinessReport:
        """仅在运行资源全部通过类型化探针时报告就绪."""
        if self._state not in {ApplicationState.STARTING, ApplicationState.RUNNING}:
            return ReadinessReport(ready=False, components=())
        diagnostics = tuple([await component.probe() for component in self._opened])
        return ReadinessReport(
            bool(diagnostics) and all(item.ready for item in diagnostics), diagnostics
        )

    async def stop(self) -> None:
        """撤销就绪并让并发调用共享同一个有界清理任务."""
        if self._state in {ApplicationState.NEW, ApplicationState.STOPPED}:
            self._state = ApplicationState.STOPPED
            return
        await self._request_cleanup()

    async def _request_cleanup(self) -> None:
        """创建一次清理任务并屏蔽调用方取消."""
        async with self._stop_lock:
            if self._stop_task is None:
                self._state = ApplicationState.STOPPING
                self._stop_task = asyncio.create_task(self._cleanup())
            task = self._stop_task
        await asyncio.shield(task)

    async def _cleanup(self) -> None:
        """在总截止期内继续尝试每个逆序关闭步骤."""
        errors: list[RuntimeError | OSError | TimeoutError] = []
        with anyio.move_on_after(self._shutdown_seconds, shield=True) as scope:
            for component in reversed(self._opened):
                try:
                    await component.stop()
                    self._close_order.append(component.name)
                except (RuntimeError, OSError, TimeoutError) as error:
                    errors.append(error)
        self._opened.clear()
        self._state = (
            ApplicationState.FAILED
            if errors or scope.cancelled_caught
            else ApplicationState.STOPPED
        )
        if errors:
            message = "应用资源关闭失败"
            raise ExceptionGroup(message, errors)
        if scope.cancelled_caught:
            message = "应用关闭超过配置截止时间"
            raise TimeoutError(message)
