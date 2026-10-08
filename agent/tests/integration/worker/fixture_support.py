"""双 Worker PostgreSQL 验收的脱敏资源所有者."""

from dataclasses import dataclass
from typing import Self, override

import anyio
import asyncpg
from sqlalchemy import text
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from scyg_agent.adapters.database.event_store import NotificationChannel, PostgreSQLEventStore

TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""


@dataclass(frozen=True, slots=True, repr=False)
class WorkerDatabaseFixture:
    """独占引擎和连接配置并只暴露脱敏类型化操作."""

    _sessions: async_sessionmaker[AsyncSession]
    _engine: AsyncEngine
    _listener_dsn: str

    @classmethod
    def create(cls, database_url: str) -> Self:
        """在资源 owner 内创建共享引擎和 session 工厂."""
        engine = create_async_engine(database_url, pool_size=12, max_overflow=0, pool_timeout=5)
        listener_dsn = database_url.replace("postgresql+asyncpg://", "postgresql://", 1)
        return cls(async_sessionmaker(engine, expire_on_commit=False), engine, listener_dsn)

    def session_factory(self) -> async_sessionmaker[AsyncSession]:
        """借出不携带连接配置表示的 session 工厂."""
        return self._sessions

    def event_store(self) -> PostgreSQLEventStore:
        """在 owner 内组合生产事件仓储并隐藏监听配置."""
        return PostgreSQLEventStore(
            self._sessions,
            self._listener_dsn,
            NotificationChannel.parse("agent_events"),
            asyncpg.connect,
        )

    async def prepare(self) -> None:
        """清理验收表并保持引擎所有权不外泄."""
        async with self._engine.begin() as connection:
            _ = await connection.execute(text(TRUNCATE_SQL))

    async def close(self) -> None:
        """在屏蔽取消的 owner 边界恰好释放一次共享引擎."""
        with anyio.CancelScope(shield=True):
            await self._engine.dispose()

    @override
    def __repr__(self) -> str:
        """只返回不含连接信息的稳定 pytest 表示."""
        return "WorkerDatabaseFixture(redacted=True)"
