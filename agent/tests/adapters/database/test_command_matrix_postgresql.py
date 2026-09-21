from dataclasses import replace

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.interaction_store import PostgreSQLInteractionStore
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.tool_store import PostgreSQLToolOperationStore
from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandRejected,
    CommandRunNotFound,
    CommandSubmission,
    IdempotencyConflict,
)
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest, ResultReference
from scyg_agent.domain.ports.interaction_store import InteractionNotFound, InteractionResolution
from scyg_agent.domain.ports.tool_store import ToolOperationNotFound
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    RunId,
    SubmitInput,
)

from .t12_postgres_support import (
    RowCounts,
    row_counts,
    seed_run,
    submission,
)


@pytest.mark.anyio
async def test_command_duplicate_replays_original_without_row_delta(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个初始 Run 和首次取消命令。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLCommandStore(sessions, "scyg_t12_events")
    request = submission("t12dup000")
    first = await store.apply(request)
    assert isinstance(first, CommandApplied)
    async with sessions() as session:
        before = await row_counts(session)

    # When: 完全相同命令再次提交。
    duplicate = await store.apply(request)

    # Then: 原始结果相等且所有表行数不变。
    assert duplicate == replace(first, replayed=True)
    async with sessions() as session:
        assert await row_counts(session) == before == RowCounts(1, 1, 1, 0, 0, 1)


@pytest.mark.anyio
async def test_command_idempotency_conflict_has_zero_delta(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个已成功应用的命令身份。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLCommandStore(sessions, "scyg_t12_events")
    original = submission("t12conf00")
    _ = await store.apply(original)
    changed = replace(original, request_digest=RequestDigest.parse("b" * 64))
    async with sessions() as session:
        before = await row_counts(session)

    # When: 相同 command_id 携带不同摘要。
    result = await store.apply(changed)

    # Then: 返回冲突且没有任何副作用增量。
    assert result == IdempotencyConflict(original.command_id)
    async with sessions() as session:
        assert await row_counts(session) == before


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("command_request", "expected"),
    [
        (
            submission("t12strev0", expected_revision=2),
            CommandRejected("revision_mismatch", 1, 0, replayed=False),
        ),
        (
            submission("t12stseq0", expected_sequence=1),
            CommandRejected("sequence_mismatch", 1, 0, replayed=False),
        ),
    ],
)
async def test_stale_command_persists_only_replayable_rejection(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
    command_request: CommandSubmission,
    expected: CommandRejected,
) -> None:
    # Given: 一个 revision=1、event head=0 的 Run。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLCommandStore(sessions, "scyg_t12_events")

    # When: 提交 stale revision 或 stale sequence 命令。
    first = await store.apply(command_request)
    assert first == expected
    async with sessions() as session:
        after_first = await row_counts(session)
        run = (await session.execute(select(RunRecord))).scalar_one()

    # Then: 仅命令拒绝和审计各增加一行, Run/事件不变; 重复无增量。
    assert after_first == RowCounts(1, 0, 1, 0, 0, 1)
    assert (run.revision, run.status) == (1, "pending")
    replayed = await store.apply(command_request)
    assert replayed == replace(expected, replayed=True)
    async with sessions() as session:
        assert await row_counts(session) == after_first


@pytest.mark.anyio
async def test_unknown_run_interaction_and_tool_are_typed_without_delta(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 空数据库和三个未知身份。
    _, sessions = t12_database
    command_store = PostgreSQLCommandStore(sessions, "scyg_t12_events")
    interaction_store = PostgreSQLInteractionStore(sessions, "scyg_t12_events")
    tool_store = PostgreSQLToolOperationStore(sessions)
    request = replace(submission("t12unkn00"), run_id=RunId("run_t12unknown"))

    # When/Then: 每个查询返回类型化 NotFound 且六表保持空。
    assert await command_store.apply(request) == CommandRunNotFound(request.run_id)
    assert await tool_store.get(OperationId("t12:unknown")) == ToolOperationNotFound(
        OperationId("t12:unknown")
    )
    interaction_id = InteractionId("int_t12unknown")
    command_id = CommandId("cmd_t12intunkn")
    command = SubmitInput(
        command_id,
        EventId("evt_t12intunkn"),
        1,
        request.submitted_at,
        interaction_id,
    )
    interaction_command = CommandSubmission(
        command_id,
        RunId("run_t12unknown"),
        1,
        0,
        "submit_input",
        request.request_digest,
        request.submitted_at,
        command,
        AuditMetadata("source", "test"),
    )
    resolution = InteractionResolution(
        interaction_id,
        request.request_digest,
        ResultReference("interaction:unknown"),
        interaction_command,
    )
    assert await interaction_store.resolve(resolution) == InteractionNotFound(interaction_id)
    async with sessions() as session:
        assert await row_counts(session) == RowCounts(0, 0, 0, 0, 0, 0)
