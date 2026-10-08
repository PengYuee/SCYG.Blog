"""Real PostgreSQL resources for the internal five-RPC control plane."""

import subprocess
import sys
from collections.abc import AsyncIterator, Mapping
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, Self, override

import asyncpg
from grpc import aio
from sqlalchemy import text
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)
from tests.acceptance_settings import require_test_settings

from scyg_agent.adapters.database.event_store import NotificationChannel, PostgreSQLEventStore
from scyg_agent.application.control import ControlApplication
from scyg_agent.application.event_subscription import EventSubscriptionService
from scyg_agent.generated.proto.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc
from scyg_agent.transport.grpc import AgentControlServicer

AGENT_ROOT = Path(__file__).parents[3]
TRUNCATE_SQL = """TRUNCATE agent_successful_operations, agent_audit_events,
agent_tool_calls, agent_interactions, agent_commands, agent_events, agent_runs CASCADE"""

if TYPE_CHECKING:

    def add_servicer(_servicer: AgentControlServicer, _server: aio.Server) -> None: ...
else:
    add_servicer = service_grpc.add_AgentControlServiceServicer_to_server


@dataclass(frozen=True, slots=True, repr=False)
class T20Database:
    """Own each reconstructed engine without exposing connection credentials."""

    _engine: AsyncEngine
    sessions: async_sessionmaker[AsyncSession]
    _listener_dsn: str

    @classmethod
    def create(cls, database_url: str, listener_dsn: str) -> Self:
        engine = create_async_engine(database_url, pool_size=4, max_overflow=0, pool_timeout=5)
        return cls(engine, async_sessionmaker(engine, expire_on_commit=False), listener_dsn)

    def application(self) -> ControlApplication:
        events = PostgreSQLEventStore(
            self.sessions,
            self._listener_dsn,
            NotificationChannel.parse("agent_events"),
            asyncpg.connect,
        )
        return ControlApplication(self.sessions, "agent_events", EventSubscriptionService(events))

    async def reset(self) -> None:
        async with self._engine.begin() as connection:
            _ = await connection.execute(text(TRUNCATE_SQL))

    async def close(self) -> None:
        await self._engine.dispose()

    @override
    def __repr__(self) -> str:
        return "T20Database(redacted=True)"


def migration_environment() -> Mapping[str, str]:
    settings = require_test_settings()
    return settings.child_environment("migration") | {
        "SCYG_T20_LISTENER_DSN": settings.listener_dsn("migration"),
    }


def migrate(arguments: list[str], environment: Mapping[str, str]) -> None:
    _ = subprocess.run(  # noqa: S603
        [sys.executable, "-m", "alembic", *arguments],
        cwd=AGENT_ROOT,
        env=environment,
        check=True,
        capture_output=True,
        text=True,
    )


@asynccontextmanager
async def grpc_stub(
    application: ControlApplication,
) -> AsyncIterator[service_grpc.AgentControlServiceStub]:
    """Start a real grpc.aio server and close both transport endpoints."""
    server = aio.server()
    add_servicer(AgentControlServicer(application), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = aio.insecure_channel(f"127.0.0.1:{port}")
    try:
        yield service_grpc.AgentControlServiceStub(channel)
    finally:
        await channel.close()
        await server.stop(None)
