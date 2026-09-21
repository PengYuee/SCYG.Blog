from collections.abc import AsyncIterator
from datetime import UTC, datetime

import anyio
import pytest
from sqlalchemy import func, select, text
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from scyg_agent.adapters.database.interaction_store import PostgreSQLInteractionStore
from scyg_agent.adapters.database.journal_records import CommandRecord, EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord, ToolCallRecord
from scyg_agent.adapters.database.run_records import InteractionRecord, RunRecord
from scyg_agent.adapters.database.tool_store import PostgreSQLToolOperationStore
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.interaction_store import (
    InteractionIdempotencyConflict,
    InteractionPending,
    InteractionRequest,
    InteractionResolution,
    InteractionResolveResult,
)
from scyg_agent.domain.ports.tool_store import ToolOperation, ToolOperationStored, ToolOutcomeStatus
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    RunId,
    SubmitInput,
    ToolCallId,
)
from tests.acceptance_settings import require_test_settings

NOW = datetime(2026, 7, 12, 12, 0, tzinfo=UTC)
RUN_ID = RunId("run_t12idem00")
INTERACTION_ID = InteractionId("int_t12idem00")
TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.fixture
async def t12_database() -> AsyncIterator[tuple[AsyncEngine, async_sessionmaker[AsyncSession]]]:
    # Given: 用户明确授权且已迁移的隔离 PostgreSQL 数据库。
    database_url = require_test_settings().normal_url
    engine = create_async_engine(database_url, pool_size=24, max_overflow=0, pool_timeout=10)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    async with engine.begin() as connection:
        _ = await connection.execute(text(TRUNCATE_SQL))
    try:
        yield engine, sessions
    finally:
        await engine.dispose()


@pytest.mark.anyio
async def test_twenty_interaction_responses_have_one_winner_and_original_result_losers(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个无执行租约的 WAITING_INPUT Run 及其唯一待处理交互。
    engine, sessions = t12_database
    async with sessions.begin() as session:
        session.add(_waiting_run())
    store = PostgreSQLInteractionStore(sessions, "scyg_t12_events")
    pending = await store.request(InteractionRequest(INTERACTION_ID, RUN_ID, "approval", NOW))
    assert isinstance(pending, InteractionPending)
    results: list[InteractionResolveResult] = []

    # When: 二十个独立事务同时尝试解析同一交互。
    async with anyio.create_task_group() as task_group:
        for index in range(20):
            _ = task_group.start_soon(_resolve_into, store, index, results)

    # Then: 一个命令获胜, 其余十九个返回赢家保存的原始引用。
    losers = [result for result in results if isinstance(result, InteractionIdempotencyConflict)]
    assert len(losers) == 19
    assert {loser.interaction_id for loser in losers} == {INTERACTION_ID}
    async with sessions() as session:
        run = (
            await session.execute(select(RunRecord).where(RunRecord.run_id == str(RUN_ID)))
        ).scalar_one()
        assert run.status == "pending_resume"
        assert run.revision == 2
        assert run.lease_token is None
        assert run.pending_interaction_id is None
        assert await _count(session, EventRecord) == 1
        assert await _count(session, CommandRecord) == 1
        assert await _count(session, AuditEventRecord) == 1
        interaction = (await session.execute(select(InteractionRecord))).scalar_one()
        assert interaction.status == "resolved"
    await engine.dispose()


@pytest.mark.anyio
async def test_twenty_identical_tool_operations_store_one_result_and_one_audit(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个 Run 和完全相同的清洗工具终态。
    _, sessions = t12_database
    async with sessions.begin() as session:
        session.add(_pending_run())
    store = PostgreSQLToolOperationStore(sessions)
    operation = _tool_operation()
    results: list[ToolOperationStored] = []

    # When: 二十个独立事务同时写入同一 operation_id。
    async with anyio.create_task_group() as task_group:
        for _ in range(20):
            _ = task_group.start_soon(_tool_into, store, operation, results)

    # Then: 一个首次结果和十九个重放, 且只有一条工具和审计事实。
    assert sum(not result.replayed for result in results) == 1
    assert sum(result.replayed for result in results) == 19
    assert {result.operation.result_reference for result in results} == {
        ResultReference("article:42:version:7")
    }
    async with sessions() as session:
        assert await _count(session, ToolCallRecord) == 1
        assert await _count(session, AuditEventRecord) == 1


async def _resolve_into(
    store: PostgreSQLInteractionStore,
    index: int,
    results: list[InteractionResolveResult],
) -> None:
    """提交一个稳定身份的并发交互响应。."""
    suffix = f"{index:08d}"
    command_id = CommandId(f"cmd_{suffix}")
    command = SubmitInput(command_id, EventId(f"evt_{suffix}"), 1, NOW, INTERACTION_ID)
    request = CommandSubmission(
        command_id,
        RUN_ID,
        1,
        0,
        "submit_input",
        RequestDigest.parse(f"{index:064x}"),
        NOW,
        command,
        AuditMetadata("source", "interaction"),
    )
    result = await store.resolve(
        InteractionResolution(
            INTERACTION_ID,
            RequestDigest.parse(f"{index + 100:064x}"),
            ResultReference("approval:accepted"),
            request,
        )
    )
    results.append(result)


async def _tool_into(
    store: PostgreSQLToolOperationStore,
    operation: ToolOperation,
    results: list[ToolOperationStored],
) -> None:
    """保存一个并发工具终态并收集强类型结果。."""
    result = await store.apply(operation)
    assert isinstance(result, ToolOperationStored)
    results.append(result)


async def _count(
    session: AsyncSession,
    record_type: type[EventRecord]
    | type[CommandRecord]
    | type[AuditEventRecord]
    | type[ToolCallRecord],
) -> int:
    """读取一个验收表的精确行数。."""
    return (await session.execute(select(func.count()).select_from(record_type))).scalar_one()


def _waiting_run() -> RunRecord:
    """构造满足数据库约束的等待输入记录。."""
    record = _pending_run()
    record.status = "waiting_input"
    record.pending_interaction_id = str(INTERACTION_ID)
    return record


def _pending_run() -> RunRecord:
    """构造最小可持久化 Run 记录。."""
    return RunRecord(
        run_id=str(RUN_ID),
        owner_user_id="user-t12",
        operation_id="t12:create",
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


def _tool_operation() -> ToolOperation:
    """构造清洗且可重放的工具终态。."""
    return ToolOperation(
        ToolCallId("tool_t12idem00"),
        OperationId("t12:tool:article"),
        RUN_ID,
        "update_article",
        RequestDigest.parse("a" * 64),
        ToolOutcomeStatus.SUCCEEDED,
        ResultReference("article:42:version:7"),
        ResultMetadata("article_version", "7"),
        None,
        NOW,
        AuditMetadata("source", "runtime"),
    )
