"""T12 真实 PostgreSQL 测试支持。"""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from datetime import UTC, datetime

import pytest
from sqlalchemy import func, select, text
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from scyg_agent.adapters.database.journal_records import CommandRecord, EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord, ToolCallRecord
from scyg_agent.adapters.database.run_records import InteractionRecord, RunRecord
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest
from scyg_agent.domain.runs import CancelRun, CommandId, EventId, RunId
from tests.acceptance_settings import require_test_settings

NOW = datetime(2026, 7, 12, 14, tzinfo=UTC)
RUN_ID = RunId("run_t12matrix0")
TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""
type RecordType = type[
    RunRecord | EventRecord | CommandRecord | InteractionRecord | ToolCallRecord | AuditEventRecord
]


@dataclass(frozen=True, slots=True)
class RowCounts:
    """记录六张 Agent 真值表的精确行数。"""

    runs: int
    events: int
    commands: int
    interactions: int
    tools: int
    audits: int


@pytest.fixture
def anyio_backend() -> str:
    """使用 asyncpg 所需 asyncio 后端。"""
    return "asyncio"


@pytest.fixture
async def t12_database() -> AsyncIterator[tuple[AsyncEngine, async_sessionmaker[AsyncSession]]]:
    """创建连接到共享测试 PostgreSQL 的清洁测试会话。"""
    database_url = require_test_settings().normal_url
    engine = create_async_engine(database_url, pool_size=24, max_overflow=0, pool_timeout=5)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    async with engine.begin() as connection:
        _ = await connection.execute(text(TRUNCATE_SQL))
    try:
        yield engine, sessions
    finally:
        await engine.dispose()


async def seed_run(sessions: async_sessionmaker[AsyncSession]) -> None:
    """插入一个初始 PENDING Run。"""
    async with sessions.begin() as session:
        session.add(
            RunRecord(
                run_id=str(RUN_ID),
                owner_user_id="user-t12",
                operation_id="t12:matrix",
                task_type="summary",
                runtime_kind="simple",
                runtime_version="v1",
                revision=1,
                status="pending",
                created_at=NOW,
                updated_at=NOW,
                attempt=0,
                next_attempt_at=NOW,
                lease_owner=None,
                lease_token=None,
                lease_expires_at=None,
                pending_interaction_id=None,
                terminal_at=None,
                terminal_metadata=None,
                error_code=None,
                error_message=None,
                error_metadata=None,
            )
        )


def submission(
    suffix: str,
    *,
    expected_revision: int = 1,
    expected_sequence: int = 0,
    digest: str = "a",
) -> CommandSubmission:
    """构造稳定取消命令提交。"""
    command_id = CommandId(f"cmd_{suffix}")
    command = CancelRun(command_id, EventId(f"evt_{suffix}"), expected_revision, NOW)
    return CommandSubmission(
        command_id,
        RUN_ID,
        expected_revision,
        expected_sequence,
        "cancel",
        RequestDigest.parse(digest * 64),
        NOW,
        command,
        AuditMetadata("source", "test"),
    )


async def row_counts(session: AsyncSession) -> RowCounts:
    """读取所有 T12 副作用表的精确计数。"""
    return RowCounts(
        await _count(session, RunRecord),
        await _count(session, EventRecord),
        await _count(session, CommandRecord),
        await _count(session, InteractionRecord),
        await _count(session, ToolCallRecord),
        await _count(session, AuditEventRecord),
    )


async def _count(session: AsyncSession, record: RecordType) -> int:
    """读取一张 ORM 表的行数。"""
    return (await session.execute(select(func.count()).select_from(record))).scalar_one()
