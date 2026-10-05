"""Redis lifecycle component tests."""

from dataclasses import dataclass
from typing import cast

import pytest

from scyg_agent.adapters.redis import RedisStreamStore, RedisUnavailableError
from scyg_agent.composition_redis import RedisResource
from scyg_agent.lifecycle import ComponentDiagnostic


@dataclass
class FakeRedis:
    ping_count: int = 0
    close_count: int = 0
    fail_ping: bool = False

    async def ping(self) -> None:
        self.ping_count += 1
        if self.fail_ping:
            raise RedisUnavailableError

    async def close(self) -> None:
        self.close_count += 1


@pytest.mark.anyio
async def test_redis_resource_start_probe_and_stop() -> None:
    client = FakeRedis()
    resource = RedisResource(cast("RedisStreamStore", cast("object", client)))

    await resource.start()
    diagnostic = await resource.probe()
    await resource.stop()

    assert diagnostic == ComponentDiagnostic(component="redis", ready=True, detail="已就绪")
    assert client.ping_count == 2
    assert client.close_count == 1


@pytest.mark.anyio
async def test_redis_resource_probe_reports_unavailable_without_endpoint_details() -> None:
    client = FakeRedis()
    resource = RedisResource(cast("RedisStreamStore", cast("object", client)))
    await resource.start()
    client.fail_ping = True

    diagnostic = await resource.probe()

    assert diagnostic == ComponentDiagnostic(component="redis", ready=False, detail="不可用")
