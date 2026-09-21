"""调用方事务内的不可变审计追加辅助函数。."""

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from scyg_agent.domain.ports.audit_store import AuditFact, StoredAuditFact

from .command_failpoints import NO_COMMAND_FAILPOINT, CommandFailpoint, CommandStage
from .operation_records import AuditEventRecord


async def append_audit_locked(
    session: AsyncSession,
    fact: AuditFact,
    failpoint: CommandFailpoint = NO_COMMAND_FAILPOINT,
) -> StoredAuditFact:
    """在调用方已持有 Run 锁时分配稳定审计序列。."""
    latest = (
        await session.execute(
            select(func.coalesce(func.max(AuditEventRecord.seq), 0)).where(
                AuditEventRecord.run_id == str(fact.run_id)
            )
        )
    ).scalar_one()
    sequence = latest + 1
    await failpoint.reach(CommandStage.BEFORE_AUDIT_ADD)
    session.add(
        AuditEventRecord(
            audit_id=fact.audit_id,
            seq=sequence,
            occurred_at=fact.occurred_at,
            run_id=str(fact.run_id),
            user_id=str(fact.user_id),
            command_id=str(fact.command_id) if fact.command_id is not None else None,
            tool_call_id=str(fact.tool_call_id) if fact.tool_call_id is not None else None,
            action=fact.action,
            outcome=fact.outcome,
            audit_metadata={fact.metadata.key: fact.metadata.value},
        )
    )
    await failpoint.reach(CommandStage.AFTER_AUDIT_ADD)
    await session.flush()
    await failpoint.reach(CommandStage.AFTER_AUDIT_FLUSH)
    return StoredAuditFact(sequence, fact)
