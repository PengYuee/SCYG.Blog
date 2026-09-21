"""延迟生产组件的生命周期测试."""

from dataclasses import dataclass, field

import anyio

from scyg_agent.composition_services import DeferredComponent, WorkerResource
from scyg_agent.lifecycle import ComponentDiagnostic
from scyg_agent.worker import Worker


@dataclass(slots=True)
class RecordingComponent:
    """记录委托调用的可变测试组件."""

    events: list[str] = field(default_factory=list)
    ready: bool = True

    @property
    def name(self) -> str:
        return "real"

    async def start(self) -> None:
        self.events.append("start")

    async def stop(self) -> None:
        self.events.append("stop")

    async def probe(self) -> ComponentDiagnostic:
        self.events.append("probe")
        return ComponentDiagnostic(self.name, self.ready, "状态")


def test_deferred_component_uses_real_probe_and_lifecycle_once() -> None:
    async def scenario() -> None:
        real = RecordingComponent()
        deferred = DeferredComponent("real", lambda: real)
        before = await deferred.probe()
        assert not before.ready
        await deferred.start()
        assert (await deferred.probe()).ready
        await deferred.stop()
        assert real.events == ["start", "probe", "stop"]

    anyio.run(scenario)


def test_worker_resource_is_not_ready_before_start_and_stop_is_safe() -> None:
    def unused_worker() -> Worker:
        message = "不应构造 Worker"
        raise AssertionError(message)

    async def scenario() -> None:
        resource = WorkerResource(unused_worker, 1.0)
        assert not (await resource.probe()).ready
        await resource.stop()

    anyio.run(scenario)
