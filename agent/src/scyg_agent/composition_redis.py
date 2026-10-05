"""Redis lifecycle component for Agent readiness and shutdown."""

from typing import final

from scyg_agent.adapters.redis.streams import RedisStreamStore, RedisUnavailableError
from scyg_agent.lifecycle import ComponentDiagnostic


@final
class RedisResource:
    """Own one Redis client and require a successful startup ping."""

    def __init__(self, client: RedisStreamStore) -> None:
        """Take ownership of a not-yet-connected Redis client."""
        self.client = client
        self._ready = False

    @property
    def name(self) -> str:
        """Return the stable component name."""
        return "redis"

    async def start(self) -> None:
        """Fail startup when Redis cannot answer a ping."""
        await self.client.ping()
        self._ready = True

    async def stop(self) -> None:
        """Close the owned Redis client."""
        await self.client.close()
        self._ready = False

    async def probe(self) -> ComponentDiagnostic:
        """Report live Redis availability without exposing endpoint values."""
        if not self._ready:
            return ComponentDiagnostic(component=self.name, ready=False, detail="未启动")
        try:
            await self.client.ping()
        except RedisUnavailableError:
            return ComponentDiagnostic(component=self.name, ready=False, detail="不可用")
        return ComponentDiagnostic(component=self.name, ready=True, detail="已就绪")
