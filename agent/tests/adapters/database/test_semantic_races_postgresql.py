from dataclasses import replace

import anyio
import pytest
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.tool_store import PostgreSQLToolOperationStore
from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandSubmission,
    IdempotencyConflict,
)
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.tool_store import (
    ToolIdempotencyConflict,
    ToolOperation,
    ToolOperationStored,
    ToolOutcomeStatus,
    ToolStoreResult,
)
from scyg_agent.domain.runs import OperationId, ToolCallId

from .t12_postgres_support import NOW, RUN_ID, RowCounts, row_counts, seed_run, submission


@pytest.mark.anyio
async def test_differing_concurrent_commands_choose_one_semantic_winner(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 同一 command_id 的两组不同规范语义, 各十个请求。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLCommandStore(sessions, "scyg_t12_events")
    original = submission("semantic0", digest="1")
    changed = replace(original, request_digest=RequestDigest.parse("2" * 64))
    results: list[CommandApplied | IdempotencyConflict] = []

    async def apply(request: CommandSubmission) -> None:
        """提交一个并发命令并收集限定结果。"""
        result = await store.apply(request)
        assert isinstance(result, CommandApplied | IdempotencyConflict)
        results.append(result)

    # When: 二十个独立事务并发竞争命令身份。
    async with anyio.create_task_group() as task_group:
        for _ in range(10):
            _ = task_group.start_soon(apply, original)
            _ = task_group.start_soon(apply, changed)

    # Then: 一个语义组获得首次结果和九次重放, 另一组十次冲突。
    applied = [result for result in results if isinstance(result, CommandApplied)]
    conflicts = [result for result in results if isinstance(result, IdempotencyConflict)]
    assert len(applied) == len(conflicts) == 10
    assert sum(not result.replayed for result in applied) == 1
    async with sessions() as session:
        assert await row_counts(session) == RowCounts(1, 1, 1, 0, 0, 1)


@pytest.mark.anyio
async def test_differing_concurrent_tools_choose_one_semantic_winner(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 同一 operation_id 的两组不同 tool_call/result 语义。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLToolOperationStore(sessions)
    original = _tool("a", "tool_semantic0", "article:1")
    changed = _tool("b", "tool_semantic1", "article:2")
    results: list[ToolStoreResult] = []

    async def apply(request: ToolOperation) -> None:
        """提交一个并发工具终态。"""
        results.append(await store.apply(request))

    # When: 二十个事务竞争同一 operation_id。
    async with anyio.create_task_group() as task_group:
        for _ in range(10):
            _ = task_group.start_soon(apply, original)
            _ = task_group.start_soon(apply, changed)

    # Then: 胜者语义十个结果, 败者十个冲突, 副作用仅一次。
    stored = [result for result in results if isinstance(result, ToolOperationStored)]
    conflicts = [result for result in results if isinstance(result, ToolIdempotencyConflict)]
    assert len(stored) == len(conflicts) == 10
    assert sum(not result.replayed for result in stored) == 1
    async with sessions() as session:
        assert await row_counts(session) == RowCounts(1, 0, 0, 0, 1, 1)


def _tool(digest: str, tool_call_id: str, reference: str) -> ToolOperation:
    """构造同 operation_id 的不同完整工具语义。"""
    return ToolOperation(
        ToolCallId(tool_call_id),
        OperationId("t12:semantic:race"),
        RUN_ID,
        "update_article",
        RequestDigest.parse(digest * 64),
        ToolOutcomeStatus.SUCCEEDED,
        ResultReference(reference),
        ResultMetadata("article_version", reference[-1]),
        None,
        NOW,
        AuditMetadata("source", digest),
    )
