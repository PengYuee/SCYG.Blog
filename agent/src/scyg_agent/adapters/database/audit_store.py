"""PostgreSQL 不可变审计存储适配器。."""

from typing import final

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.ports.audit_store import AuditFact, StoredAuditFact

from .audit_append import append_audit_locked
from .run_records import RunRecord


@final
class PostgreSQLAuditStore:
    """仅提供创建语义并用 Run 行锁保护序列分配。."""

    def __init__(self, sessions: async_sessionmaker[AsyncSession]) -> None:
        """绑定短事务会话工厂。."""
        self._sessions = sessions

    async def append(self, fact: AuditFact) -> StoredAuditFact:
        """锁定所属 Run 后追加事实。."""
        async with self._sessions.begin() as session:
            return await self.append_in_session(session, fact)

    async def append_in_session(self, session: AsyncSession, fact: AuditFact) -> StoredAuditFact:
        """在调用方会话中锁定 Run 并追加事实。."""
        _ = (
            await session.execute(
                select(RunRecord.run_id)
                .where(RunRecord.run_id == str(fact.run_id))
                .with_for_update()
            )
        ).scalar_one()
        return await append_audit_locked(session, fact)
