import anyio
import pytest
from sqlalchemy import delete, select, text, update
from sqlalchemy.exc import DBAPIError
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.interaction_store import PostgreSQLInteractionStore
from scyg_agent.adapters.database.operation_records import AuditEventRecord
from scyg_agent.adapters.database.run_records import InteractionRecord, RunRecord
from scyg_agent.adapters.database.tool_store import PostgreSQLToolOperationStore
from scyg_agent.domain.ports.command_store import CommandRejected, CommandSubmission
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.interaction_store import (
    InteractionPending,
    InteractionRequest,
    InteractionResolution,
)
from scyg_agent.domain.ports.tool_store import ToolOperation, ToolOperationStored, ToolOutcomeStatus
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    SubmitInput,
    ToolCallId,
)

from .t12_postgres_support import NOW, RUN_ID, RowCounts, row_counts, seed_run, submission

INTERACTION_ID = InteractionId("int_t12rollback")
CREATE_REJECT_FUNCTION_SQL = """
CREATE OR REPLACE FUNCTION reject_t12_audit_insert() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 't12 audit insert rejected'; END;
$$ LANGUAGE plpgsql;
"""
CREATE_REJECT_TRIGGER_SQL = """
CREATE TRIGGER t12_reject_audit_insert BEFORE INSERT ON agent_audit_events
FOR EACH ROW EXECUTE FUNCTION reject_t12_audit_insert();
"""
DROP_REJECT_TRIGGER_SQL = "DROP TRIGGER IF EXISTS t12_reject_audit_insert ON agent_audit_events"
DROP_REJECT_FUNCTION_SQL = "DROP FUNCTION IF EXISTS reject_t12_audit_insert()"


@pytest.mark.anyio
async def test_mixed_concurrent_command_tool_audits_are_contiguous(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个 Run、十个 stale 命令和十个不同工具操作。
    _, sessions = t12_database
    await seed_run(sessions)
    command_store = PostgreSQLCommandStore(sessions, "scyg_t12_events")
    tool_store = PostgreSQLToolOperationStore(sessions)
    command_results: list[CommandRejected] = []
    tool_results: list[ToolOperationStored] = []

    async def apply_command(index: int) -> None:
        """提交一个唯一 stale sequence 命令。"""
        result = await command_store.apply(submission(f"mixc{index:04d}", expected_sequence=1))
        assert isinstance(result, CommandRejected)
        command_results.append(result)

    async def apply_tool(index: int) -> None:
        """提交一个唯一工具终态。"""
        result = await tool_store.apply(_tool(index))
        assert isinstance(result, ToolOperationStored)
        tool_results.append(result)

    # When: 二十个独立事务混合竞争同一 Run 审计序列。
    async with anyio.create_task_group() as task_group:
        for index in range(10):
            _ = task_group.start_soon(apply_command, index)
            _ = task_group.start_soon(apply_tool, index)

    # Then: 无死锁、重复或间隙, 且业务表增量精确。
    assert len(command_results) == len(tool_results) == 10
    async with sessions() as session:
        audits = (
            await session.execute(select(AuditEventRecord).order_by(AuditEventRecord.seq))
        ).scalars()
        assert [audit.seq for audit in audits] == list(range(1, 21))
        assert await row_counts(session) == RowCounts(1, 0, 10, 0, 10, 20)


@pytest.mark.anyio
@pytest.mark.parametrize("mutation", ["update", "delete"])
async def test_database_trigger_rejects_audit_update_and_delete(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
    mutation: str,
) -> None:
    # Given: 一个已提交工具审计事实。
    _, sessions = t12_database
    await seed_run(sessions)
    _ = await PostgreSQLToolOperationStore(sessions).apply(_tool(0))

    # When: 绕过适配器直接尝试 UPDATE 或 DELETE。
    with pytest.raises(DBAPIError, match="immutable"):
        await _mutate_audit(sessions, mutation)

    # Then: 触发器保留原事实和计数。
    async with sessions() as session:
        audit = (await session.execute(select(AuditEventRecord))).scalar_one()
        assert audit.outcome == "succeeded"
        assert await row_counts(session) == RowCounts(1, 0, 0, 0, 1, 1)


@pytest.mark.anyio
async def test_pg_notify_failure_rolls_back_command_transaction(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个会令 asyncpg 编码失败的 NUL 通知通道。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLCommandStore(sessions, "invalid\x00channel")

    # When: Run 和事件已写入后 pg_notify 失败。
    with pytest.raises((DBAPIError, ValueError, UnicodeError)):
        _ = await store.apply(submission("notifybad"))

    # Then: Run、事件、命令、审计全部回滚。
    async with sessions() as session:
        assert await row_counts(session) == RowCounts(1, 0, 0, 0, 0, 0)


@pytest.mark.anyio
async def test_audit_failure_rolls_back_tool_transaction(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    """验证审计插入失败不会留下工具终态。"""
    _, sessions = t12_database
    await seed_run(sessions)
    await _install_reject_insert_trigger(sessions)
    try:
        with pytest.raises(DBAPIError, match="t12 audit insert rejected"):
            _ = await PostgreSQLToolOperationStore(sessions).apply(_tool(0))
    finally:
        await _drop_reject_insert_trigger(sessions)
    async with sessions() as session:
        assert await row_counts(session) == RowCounts(1, 0, 0, 0, 0, 0)


@pytest.mark.anyio
async def test_audit_failure_rolls_back_interaction_resolution(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    """验证命令审计失败会连同交互解析和通知一起回滚。"""
    _, sessions = t12_database
    await seed_run(sessions)
    async with sessions.begin() as session:
        _ = await session.execute(
            update(RunRecord)
            .where(RunRecord.run_id == str(RUN_ID))
            .values(status="waiting_input", pending_interaction_id=str(INTERACTION_ID))
        )
    store = PostgreSQLInteractionStore(sessions, "scyg_t12_events")
    pending = await store.request(InteractionRequest(INTERACTION_ID, RUN_ID, "approval", NOW))
    assert isinstance(pending, InteractionPending)
    await _install_reject_insert_trigger(sessions)
    try:
        with pytest.raises(DBAPIError, match="t12 audit insert rejected"):
            _ = await store.resolve(_interaction_resolution())
    finally:
        await _drop_reject_insert_trigger(sessions)
    async with sessions() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        interaction = (await session.execute(select(InteractionRecord))).scalar_one()
        assert run.status == "waiting_input"
        assert interaction.status == "pending"
        assert await row_counts(session) == RowCounts(1, 0, 0, 1, 0, 0)


def _tool(index: int) -> ToolOperation:
    """构造唯一且清洗后的工具终态。"""
    return ToolOperation(
        ToolCallId(f"tool_mix{index:08d}"),
        OperationId(f"t12:mixed:{index}"),
        RUN_ID,
        "update_article",
        RequestDigest.parse(f"{index + 1:064x}"),
        ToolOutcomeStatus.SUCCEEDED,
        ResultReference(f"article:{index}:version:1"),
        ResultMetadata("article_version", "1"),
        None,
        NOW,
        AuditMetadata("source", "mixed"),
    )


async def _mutate_audit(sessions: async_sessionmaker[AsyncSession], mutation: str) -> None:
    """直接执行被触发器禁止的审计变更。"""
    async with sessions.begin() as session:
        statement = (
            update(AuditEventRecord).values(outcome="changed")
            if mutation == "update"
            else delete(AuditEventRecord)
        )
        _ = await session.execute(statement)


async def _install_reject_insert_trigger(
    sessions: async_sessionmaker[AsyncSession],
) -> None:
    """安装事务回滚探针使用的审计拒绝触发器。"""
    async with sessions.begin() as session:
        _ = await session.execute(text(CREATE_REJECT_FUNCTION_SQL))
        _ = await session.execute(text(CREATE_REJECT_TRIGGER_SQL))


async def _drop_reject_insert_trigger(sessions: async_sessionmaker[AsyncSession]) -> None:
    """移除事务回滚探针使用的审计拒绝触发器。"""
    async with sessions.begin() as session:
        _ = await session.execute(text(DROP_REJECT_TRIGGER_SQL))
        _ = await session.execute(text(DROP_REJECT_FUNCTION_SQL))


def _interaction_resolution() -> InteractionResolution:
    """构造绑定完整命令语义的交互解析。"""
    command_id = CommandId("cmd_t12rollback")
    command = SubmitInput(command_id, EventId("evt_t12rollback"), 1, NOW, INTERACTION_ID)
    request = CommandSubmission(
        command_id,
        RUN_ID,
        1,
        0,
        "submit_input",
        RequestDigest.parse("a" * 64),
        NOW,
        command,
        AuditMetadata("source", "interaction"),
    )
    return InteractionResolution(
        INTERACTION_ID,
        RequestDigest.parse("b" * 64),
        ResultReference("approval:accepted"),
        request,
    )
