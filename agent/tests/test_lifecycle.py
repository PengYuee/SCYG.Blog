"""???????????"""

from dataclasses import dataclass, field
from typing import override

import anyio
import pytest

from scyg_agent.lifecycle import (
    AgentApplication,
    ApplicationState,
    ComponentDiagnostic,
    LifecycleComponents,
)


@dataclass(slots=True)
class Recorder:
    events: list[str] = field(default_factory=list)
    ready: bool = True

    @property
    def name(self) -> str:
        return "component"

    async def start(self) -> None:
        self.events.append("start")

    async def stop(self) -> None:
        self.events.append("stop")

    async def probe(self) -> ComponentDiagnostic:
        return ComponentDiagnostic("component", self.ready, "??" if self.ready else "???")


def test_start_stop_are_ordered_and_idempotent() -> None:
    async def scenario() -> None:
        first, second, third = Recorder(), Recorder(), Recorder()
        app = AgentApplication(LifecycleComponents((first, second, third)), 1.0)
        await app.start()
        await app.start()
        assert app.state is ApplicationState.RUNNING
        assert (await app.readiness()).ready
        await app.stop()
        await app.stop()
        assert first.events == ["start", "stop"]
        assert second.events == ["start", "stop"]
        assert third.events == ["start", "stop"]
        assert app.close_order == ("component", "component", "component")

    anyio.run(scenario)


def test_start_failure_closes_opened_components_in_reverse() -> None:
    @dataclass(slots=True)
    class Failing(Recorder):
        @override
        async def start(self) -> None:
            self.events.append("start")
            message = "??????"
            raise RuntimeError(message)

    async def scenario() -> None:
        first, failing, unopened = Recorder(), Failing(), Recorder()
        app = AgentApplication(LifecycleComponents((first, failing, unopened)), 1.0)
        with pytest.raises(RuntimeError):
            await app.start()
        assert first.events == ["start", "stop"]
        assert failing.events == ["start"]
        assert unopened.events == []
        assert app.state is ApplicationState.FAILED

    anyio.run(scenario)


def test_readiness_degrades_and_recovers_without_changing_liveness() -> None:
    async def scenario() -> None:
        component = Recorder()
        app = AgentApplication(LifecycleComponents((component,)), 1.0)
        await app.start()
        component.ready = False
        assert app.liveness().alive
        assert not (await app.readiness()).ready
        component.ready = True
        assert (await app.readiness()).ready
        await app.stop()

    anyio.run(scenario)
