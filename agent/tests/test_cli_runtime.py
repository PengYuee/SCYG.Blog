"""CLI 信号合并与取消清理测试."""

import signal
from dataclasses import dataclass, field
from typing import final

import anyio
import pytest

from scyg_agent.cli_runtime import (
    PreviousHandler,
    ShutdownController,
    SignalCallback,
    run_application,
    wait_for_process_signal,
)
from scyg_agent.composition import ProductionApplication
from scyg_agent.config import load_settings
from scyg_agent.lifecycle import AgentApplication, ComponentDiagnostic, LifecycleComponents


@dataclass(slots=True)  # noqa: RUF100  # noqa: MUTABLE_OK - 记录生命周期事件。
class RecordingComponent:
    """记录真实生命周期调用的测试组件."""

    events: list[str] = field(default_factory=list)
    started: anyio.Event = field(default_factory=anyio.Event)

    @property
    def name(self) -> str:
        return "recording"

    async def start(self) -> None:
        self.events.append("start")
        self.started.set()

    async def stop(self) -> None:
        self.events.append("stop")

    async def probe(self) -> ComponentDiagnostic:
        return ComponentDiagnostic(component=self.name, ready=True, detail="已就绪")


@dataclass(frozen=True, slots=True)
class FixedBuilder:
    """返回一个真实 AgentApplication 生命周期."""

    application: ProductionApplication

    def build(self) -> ProductionApplication:
        return self.application


def test_shutdown_controller_coalesces_repeated_requests() -> None:
    async def scenario() -> None:
        controller = ShutdownController()
        controller.request()
        controller.request()
        await controller.wait()
        assert controller.requested

    anyio.run(scenario)


def test_run_application_starts_and_stops_once(configured_environment: None) -> None:
    assert configured_environment is None

    async def scenario() -> None:
        component = RecordingComponent()
        lifecycle = AgentApplication(LifecycleComponents((component,)), 1.0)

        async def signal_waiter(controller: ShutdownController) -> None:
            controller.request()
            controller.request()

        await run_application(
            load_settings(),
            builder=FixedBuilder(ProductionApplication(lifecycle)),
            signal_waiter=signal_waiter,
        )
        assert component.events == ["start", "stop"]

    anyio.run(scenario)


def test_cancellation_during_wait_still_closes(configured_environment: None) -> None:
    assert configured_environment is None

    async def scenario() -> None:
        component = RecordingComponent()
        lifecycle = AgentApplication(LifecycleComponents((component,)), 1.0)

        async def wait_forever(_controller: ShutdownController) -> None:
            await anyio.sleep_forever()

        async def run_until_cancelled() -> None:
            await run_application(
                load_settings(),
                builder=FixedBuilder(ProductionApplication(lifecycle)),
                signal_waiter=wait_forever,
            )

        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(run_until_cancelled)
            await component.started.wait()
            tasks.cancel_scope.cancel()
        assert component.events == ["start", "stop"]

    anyio.run(scenario)


@final
class FakeWindowsSignalApi:
    """记录 Windows signal bridge 的安装、触发和恢复."""

    def __init__(
        self,
        supported: tuple[signal.Signals, ...],
        *,
        fail_on: signal.Signals | None = None,
    ) -> None:
        self._supported = supported
        self._fail_on = fail_on
        self.callbacks: dict[signal.Signals, SignalCallback] = {}
        self.installed = anyio.Event()
        self.restored: list[tuple[signal.Signals, PreviousHandler]] = []

    def supported(self) -> tuple[signal.Signals, ...]:
        return self._supported

    def install(self, value: signal.Signals, callback: SignalCallback) -> PreviousHandler:
        if value is self._fail_on:
            message = "注入信号安装失败"
            raise RuntimeError(message)
        self.callbacks[value] = callback
        if len(self.callbacks) == len(self._supported):
            self.installed.set()
        return signal.SIG_DFL

    def restore(self, value: signal.Signals, previous: PreviousHandler) -> None:
        self.restored.append((value, previous))

    def trigger(self, value: signal.Signals) -> None:
        self.callbacks[value](int(value), None)


def test_windows_bridge_wakes_once_and_restores_all_handlers() -> None:
    async def scenario() -> None:
        values = (signal.SIGINT, signal.SIGTERM, signal.SIGABRT)
        api = FakeWindowsSignalApi(values)
        controller = ShutdownController()
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(wait_for_process_signal, controller, "win32", api)
            await api.installed.wait()
            api.trigger(signal.SIGINT)
            api.trigger(signal.SIGTERM)
        assert controller.requested
        assert [value for value, _handler in api.restored] == list(reversed(values))

    anyio.run(scenario)


def test_windows_bridge_supports_runtime_without_sigbreak() -> None:
    async def scenario() -> None:
        values = (signal.SIGINT, signal.SIGTERM)
        api = FakeWindowsSignalApi(values)
        controller = ShutdownController()
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(wait_for_process_signal, controller, "win32", api)
            await api.installed.wait()
            api.trigger(signal.SIGINT)
        assert set(api.callbacks) == set(values)
        assert [value for value, _handler in api.restored] == list(reversed(values))

    anyio.run(scenario)


def test_windows_bridge_restores_handlers_when_wait_is_cancelled() -> None:
    async def scenario() -> None:
        values = (signal.SIGINT, signal.SIGTERM)
        api = FakeWindowsSignalApi(values)
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(wait_for_process_signal, ShutdownController(), "win32", api)
            await api.installed.wait()
            tasks.cancel_scope.cancel()
        assert [value for value, _handler in api.restored] == list(reversed(values))

    anyio.run(scenario)


def test_windows_bridge_restores_prior_handlers_on_registration_failure() -> None:
    async def scenario() -> None:
        api = FakeWindowsSignalApi((signal.SIGINT, signal.SIGTERM), fail_on=signal.SIGTERM)
        with pytest.raises(RuntimeError, match="注入信号安装失败"):
            await wait_for_process_signal(ShutdownController(), platform="win32", windows_api=api)
        assert api.restored == [(signal.SIGINT, signal.SIG_DFL)]

    anyio.run(scenario)


def test_windows_bridge_ignores_callback_after_loop_closed() -> None:
    values = (signal.SIGINT,)
    api_holder: list[FakeWindowsSignalApi] = []

    async def scenario() -> None:
        api = FakeWindowsSignalApi(values)
        api_holder.append(api)
        controller = ShutdownController()
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(wait_for_process_signal, controller, "win32", api)
            await api.installed.wait()
            api.trigger(signal.SIGINT)
        assert controller.requested

    anyio.run(scenario)

    api_holder[0].trigger(signal.SIGINT)
