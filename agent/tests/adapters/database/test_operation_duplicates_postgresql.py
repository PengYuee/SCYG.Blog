from dataclasses import replace

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.interaction_store import PostgreSQLInteractionStore
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.tool_store import PostgreSQLToolOperationStore
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.interaction_store import (
    AlreadyResolved,
    InteractionRequest,
    InteractionResolution,
)
from scyg_agent.domain.ports.tool_store import (
    ToolIdempotencyConflict,
    ToolOperation,
    ToolOperationStored,
    ToolOutcomeStatus,
)
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    SubmitInput,
    ToolCallId,
)

from .t12_postgres_support import NOW, RUN_ID, RowCounts, row_counts, seed_run

INTERACTION_ID = InteractionId("int_t12duplct0")


@pytest.mark.anyio
async def test_resolved_interaction_duplicate_has_exact_zero_delta(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个已解析成功的唯一交互。
    _, sessions = t12_database
    await seed_run(sessions)
    async with sessions.begin() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        run.status = "waiting_input"
        run.pending_interaction_id = str(INTERACTION_ID)
    store = PostgreSQLInteractionStore(sessions, "scyg_t12_events")
    _ = await store.request(InteractionRequest(INTERACTION_ID, RUN_ID, "approval", NOW))
    resolution = _resolution()
    first = await store.resolve(resolution)
    async with sessions() as session:
        before = await row_counts(session)

    # When: 相同 interaction 再次解析。
    duplicate = await store.resolve(resolution)

    # Then: 返回赢家原始引用且六张表均无新增。
    assert duplicate == AlreadyResolved(INTERACTION_ID, ResultReference("approval:accepted"))
    async with sessions() as session:
        assert await row_counts(session) == before == RowCounts(1, 1, 1, 1, 0, 1)
    assert first != duplicate


@pytest.mark.anyio
async def test_tool_duplicate_and_conflict_have_exact_zero_delta(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个已保存的工具操作终态。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLToolOperationStore(sessions)
    operation = _operation()
    first = await store.apply(operation)
    assert first == ToolOperationStored(operation, replayed=False)
    async with sessions() as session:
        before = await row_counts(session)

    # When: 相同请求重试, 随后用不同摘要复用 operation_id。
    duplicate = await store.apply(operation)
    conflict = await store.apply(replace(operation, request_digest=RequestDigest.parse("4" * 64)))

    # Then: 重放原始结果, 冲突类型化, 二者均无行增量。
    assert isinstance(duplicate, ToolOperationStored)
    assert replace(duplicate.operation, audit_metadata=operation.audit_metadata) == operation
    assert duplicate.replayed
    assert conflict == ToolIdempotencyConflict(operation.operation_id)
    async with sessions() as session:
        assert await row_counts(session) == before == RowCounts(1, 0, 0, 0, 1, 1)


def _resolution() -> InteractionResolution:
    """构造固定 SubmitInput 解析。"""
    command_id = CommandId("cmd_t12duplct0")
    command = SubmitInput(
        command_id,
        EventId("evt_t12duplct0"),
        1,
        NOW,
        INTERACTION_ID,
    )
    submission = CommandSubmission(
        command_id,
        RUN_ID,
        1,
        0,
        "submit_input",
        RequestDigest.parse("3" * 64),
        NOW,
        command,
        AuditMetadata("source", "interaction"),
    )
    return InteractionResolution(
        INTERACTION_ID,
        RequestDigest.parse("5" * 64),
        ResultReference("approval:accepted"),
        submission,
    )


def _operation() -> ToolOperation:
    """构造固定成功工具终态。"""
    return ToolOperation(
        ToolCallId("tool_t12duplct0"),
        OperationId("t12:duplicate:tool"),
        RUN_ID,
        "update_article",
        RequestDigest.parse("6" * 64),
        ToolOutcomeStatus.SUCCEEDED,
        ResultReference("article:42:version:7"),
        ResultMetadata("article_version", "7"),
        None,
        NOW,
        AuditMetadata("source", "runtime"),
    )
