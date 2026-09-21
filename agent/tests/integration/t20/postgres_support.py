"""T20 gRPC 持久化重启验收的脱敏资源支持。"""

import subprocess
import sys
from collections.abc import AsyncIterator, Mapping
from contextlib import asynccontextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, Protocol, Self, override

import asyncpg
from grpc import aio
from sqlalchemy import func, select, text
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)
from tests.acceptance_settings import require_test_settings

from scyg_agent.adapters.auth import JwtVerifier
from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.event_store import NotificationChannel, PostgreSQLEventStore
from scyg_agent.adapters.database.interaction_store import PostgreSQLInteractionStore
from scyg_agent.adapters.database.journal_records import CommandRecord, EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.application import ApplicationFacade
from scyg_agent.domain.runs import Run, RunId
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc
from scyg_agent.transport.grpc import AgentControlServicer

AGENT_ROOT = Path(__file__).parents[3]
TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""


class ControlStub(Protocol):
    """声明验收使用的生成 stub 方法。"""

    def CreateRun(  # noqa: N802
        self,
        request: service_pb2.CreateRunRequest,
        *,
        metadata: tuple[tuple[str, str], ...],
    ) -> aio.UnaryUnaryCall[service_pb2.CreateRunRequest, service_pb2.CreateRunResponse]: ...


if TYPE_CHECKING:

    def add_servicer(_servicer: AgentControlServicer, _server: aio.Server) -> None: ...
else:
    add_servicer = service_grpc.add_AgentControlServiceServicer_to_server


@dataclass(frozen=True, slots=True, repr=False)
class T20Database:
    """隐藏连接信息并拥有每次重建所需资源。"""

    _engine: AsyncEngine
    sessions: async_sessionmaker[AsyncSession]
    _listener_dsn: str

    @classmethod
    def create(cls, database_url: str, listener_dsn: str) -> Self:
        engine = create_async_engine(database_url, pool_size=4, max_overflow=0, pool_timeout=5)
        return cls(engine, async_sessionmaker(engine, expire_on_commit=False), listener_dsn)

    def facade(self) -> ApplicationFacade:
        """组合全部生产 PostgreSQL 应用端口。"""
        runs = PostgreSQLRunRepository(self.sessions)
        events = PostgreSQLEventStore(
            self.sessions,
            self._listener_dsn,
            NotificationChannel.parse("agent_events"),
            asyncpg.connect,
        )
        return ApplicationFacade(
            runs,
            events,
            PostgreSQLCommandStore(self.sessions, "agent_events"),
            PostgreSQLInteractionStore(self.sessions, "agent_events"),
        )

    async def reset(self) -> None:
        async with self._engine.begin() as connection:
            _ = await connection.execute(text(TRUNCATE_SQL))

    async def counts(self) -> tuple[int, int, int, int]:
        async with self.sessions() as session:
            return (
                await _count(session, RunRecord),
                await _count(session, CommandRecord),
                await _count(session, AuditEventRecord),
                await _count(session, EventRecord),
            )

    async def run(self, run_id: RunId) -> Run:
        """通过生产仓储重建领域 Run。"""
        result = await PostgreSQLRunRepository(self.sessions).get(run_id)
        assert isinstance(result, Run)
        return result

    async def clear_input(self, run_id: RunId) -> None:
        """模拟 revision 05 前历史行的可空输入。"""
        async with self.sessions.begin() as session:
            _ = await session.execute(
                text("""UPDATE agent_runs
SET initial_message = NULL, article_id = NULL
WHERE run_id = :run_id"""),
                {"run_id": str(run_id)},
            )

    async def close(self) -> None:
        await self._engine.dispose()

    @override
    def __repr__(self) -> str:
        return "T20Database(redacted=True)"


def migration_environment() -> Mapping[str, str]:
    """读取统一配置并构造 migration 子进程环境。"""
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
async def grpc_stub(facade: ApplicationFacade, verifier: JwtVerifier) -> AsyncIterator[ControlStub]:
    """启动并确定关闭真实本地 grpc.aio 服务。"""
    server = aio.server()
    add_servicer(AgentControlServicer(facade, verifier), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = aio.insecure_channel(f"127.0.0.1:{port}")
    try:
        yield service_grpc.AgentControlServiceStub(channel)
    finally:
        await channel.close()
        await server.stop(None)


async def _count(
    session: AsyncSession,
    record: type[RunRecord | CommandRecord | AuditEventRecord | EventRecord],
) -> int:
    return (await session.execute(select(func.count()).select_from(record))).scalar_one()
