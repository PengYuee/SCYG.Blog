"""Agent exposes health HTTP only, never browser business endpoints."""

import anyio
from httpx import ASGITransport, AsyncClient

from scyg_agent.composition import ProductionApplicationFactory
from scyg_agent.lifecycle import AgentApplication, LifecycleComponents
from scyg_agent.transport.http import create_http_app


def test_health_only_route_table() -> None:
    async def scenario() -> None:
        app = create_http_app()
        lifecycle = AgentApplication(LifecycleComponents(()), 1.0)
        ProductionApplicationFactory.mount_health(app, [lifecycle])
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            assert (await client.get("/health/live")).json() == {"alive": True}
            assert (await client.get("/health/ready")).json() == {"ready": False, "components": []}
            assert (await client.get("/api/runs/safe-run")).status_code == 404

    anyio.run(scenario)
