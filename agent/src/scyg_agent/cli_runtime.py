"""CLI 唯一拥有的进程信号与生产生命周期运行器."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - Windows bridge 需要当前原生循环。
import signal
import sys
from collections.abc import Callable, Coroutine
from types import FrameType
from typing import Protocol, final

import anyio

from scyg_agent.composition import ProductionApplication, ProductionApplicationFactory
from scyg_agent.config import ApplicationSettings


@final
class ShutdownController:
    """把重复或并发关闭请求合并为一个事件."""

    __slots__ = ("_event", "_requested")

    def __init__(self) -> None:
        """创建不可清除的一次性关闭事件."""
        self._event: anyio.Event = anyio.Event()
        self._requested: bool = False

    @property
    def requested(self) -> bool:
        """报告是否已收到首个关闭请求."""
        return self._requested

    def request(self) -> None:
        """幂等发布关闭请求."""
        if not self._requested:
            self._requested = True
            self._event.set()

    async def wait(self) -> None:
        """等待首个关闭请求."""
        await self._event.wait()


class ApplicationBuilder(Protocol):
    """构造尚未启动的生产应用."""

    def build(self) -> ProductionApplication:
        """返回完整生产组件计划."""
        ...


SignalWaiter = Callable[[ShutdownController], Coroutine[object, object, None]]
SignalCallback = Callable[[int, FrameType | None], None]
type PreviousHandler = SignalCallback | int | None


class WindowsSignalApi(Protocol):
    """封装 Windows 同步信号注册和恢复."""

    def supported(self) -> tuple[signal.Signals, ...]:
        """返回当前解释器支持的 Windows 信号."""
        ...

    def install(self, value: signal.Signals, callback: SignalCallback) -> PreviousHandler:
        """安装回调并返回旧处理器."""
        ...

    def restore(self, value: signal.Signals, previous: PreviousHandler) -> None:
        """恢复一个旧处理器."""
        ...


@final
class _SystemWindowsSignalApi:
    """调用 CPython 主线程同步 signal API."""

    def supported(self) -> tuple[signal.Signals, ...]:
        """包含 SIGINT/SIGTERM 以及 Windows 可用的 SIGBREAK."""
        values = [signal.SIGINT, signal.SIGTERM]
        if sys.platform == "win32":
            values.append(signal.SIGBREAK)
        return tuple(values)

    def install(self, value: signal.Signals, callback: SignalCallback) -> PreviousHandler:
        """同步安装最小回调."""
        return signal.signal(value, callback)

    def restore(self, value: signal.Signals, previous: PreviousHandler) -> None:
        """同步恢复旧处理器."""
        _ = signal.signal(value, previous)


async def _wait_for_windows_signal(controller: ShutdownController, api: WindowsSignalApi) -> None:
    """使用同步 signal bridge 安全唤醒当前 Selector 循环."""
    loop = asyncio.get_running_loop()
    previous: list[tuple[signal.Signals, PreviousHandler]] = []

    def notify(_signum: int, _frame: FrameType | None) -> None:
        try:
            _ = loop.call_soon_threadsafe(controller.request)
        except RuntimeError:
            return

    try:
        for value in api.supported():
            previous.append((value, api.install(value, notify)))  # noqa: PERF401 - 每次安装后立即登记恢复责任。
        await controller.wait()
    finally:
        for value, handler in reversed(previous):
            api.restore(value, handler)


async def wait_for_process_signal(
    controller: ShutdownController,
    platform: str = sys.platform,
    windows_api: WindowsSignalApi | None = None,
) -> None:
    """按平台等待信号并只请求一次关闭."""
    if platform == "win32":
        await _wait_for_windows_signal(controller, windows_api or _SystemWindowsSignalApi())
        return
    with anyio.open_signal_receiver(signal.SIGINT, signal.SIGTERM) as signals:
        async for _received in signals:
            controller.request()
            return


async def run_application(
    settings: ApplicationSettings,
    *,
    builder: ApplicationBuilder | None = None,
    signal_waiter: SignalWaiter = wait_for_process_signal,
) -> None:
    """启动生产应用,等待关闭请求并屏蔽取消完成清理."""
    resolved = builder or ProductionApplicationFactory(settings)
    application = resolved.build()
    lifecycle = application.lifecycle
    started = False
    try:
        await lifecycle.start()
        started = True
        controller = ShutdownController()
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(signal_waiter, controller)
            await controller.wait()
            tasks.cancel_scope.cancel()
    finally:
        if started:
            with anyio.CancelScope(shield=True):
                await lifecycle.stop()
