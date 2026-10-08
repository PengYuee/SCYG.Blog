"""生产组合根无 I/O 构造测试."""

from dataclasses import dataclass

import anyio
import pytest
from fastapi import FastAPI
from httpx import ASGITransport, AsyncClient

from scyg_agent.composition import ProductionApplicationFactory
from scyg_agent.config import FeatureFlags, load_settings
from scyg_agent.lifecycle import (
    AgentApplication,
    ComponentDiagnostic,
    LifecycleComponent,
    LifecycleComponents,
)


def test_factory_builds_authoritative_real_component_order(
    configured_environment: None,
) -> None:
    assert configured_environment is None
    settings = load_settings()

    application = ProductionApplicationFactory(settings).build()
    assert application.lifecycle.component_names == (
        "database",
        "migration",
        "checkpoint",
        "redis",
        "agent_runner",
        "grpc",
        "worker",
        "http",
    )


def test_factory_preserves_typed_optional_surfaces(configured_environment: None) -> None:
    assert configured_environment is None
    settings = load_settings().model_copy(
        update={"feature_flags": FeatureFlags(grpc=False, worker=False, http=False)}
    )

    application = ProductionApplicationFactory(settings).build()
    assert application.lifecycle.component_names == (
        "database",
        "migration",
        "checkpoint",
        "redis",
        "agent_runner",
    )


def test_factory_and_settings_representations_are_secret_free(
    configured_environment: None,
) -> None:
    assert configured_environment is None
    settings = load_settings()

    rendered = repr(ProductionApplicationFactory(settings))

    assert "database-secret" not in rendered
    assert "provider-secret" not in rendered
    assert "provider.example" not in rendered


def test_health_routes_use_lifecycle_without_downstream_liveness() -> None:
    async def scenario() -> None:
        lifecycle = AgentApplication(LifecycleComponents(()), 1.0)
        app = FastAPI()
        ProductionApplicationFactory.mount_health(app, [lifecycle])
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            live = await client.get("/health/live")
            ready = await client.get("/health/ready")
        assert live.json() == {"alive": True}
        assert ready.json() == {"ready": False, "components": []}

    anyio.run(scenario)


@dataclass(slots=True)
class FailpointComponent:
    """记录工厂顺序并在指定组件启动前失败."""

    component_name: str
    failed_name: str
    events: list[str]

    @property
    def name(self) -> str:
        return self.component_name

    async def start(self) -> None:
        self.events.append(f"start:{self.name}")
        if self.name == self.failed_name:
            message = "注入组合启动失败"
            raise RuntimeError(message)

    async def stop(self) -> None:
        self.events.append(f"stop:{self.name}")

    async def probe(self) -> ComponentDiagnostic:
        return ComponentDiagnostic(component=self.name, ready=True, detail="已就绪")


@pytest.mark.parametrize(
    "failed_name",
    ["database", "migration", "checkpoint", "redis", "agent_runner", "grpc", "worker", "http"],
)
def test_each_factory_failpoint_closes_prior_components_in_reverse(
    configured_environment: None, failed_name: str
) -> None:
    assert configured_environment is None
    events: list[str] = []

    def decorate(component: LifecycleComponent) -> LifecycleComponent:
        return FailpointComponent(component.name, failed_name, events)

    async def scenario() -> None:
        lifecycle = ProductionApplicationFactory(load_settings(), decorate).build().lifecycle
        with pytest.raises(RuntimeError, match="注入组合启动失败"):
            await lifecycle.start()
        names = list(lifecycle.component_names)
        failed_index = names.index(failed_name)
        expected_stops = [f"stop:{name}" for name in reversed(names[:failed_index])]
        assert [event for event in events if event.startswith("stop:")] == expected_stops

    anyio.run(scenario)
