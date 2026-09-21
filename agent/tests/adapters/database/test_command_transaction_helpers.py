from collections.abc import AsyncIterator, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager

import pytest
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.command_replay import replay_command
from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.journal_records import EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord
from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandDataIntegrity,
    CommandRejected,
    CommandRunNotFound,
    IdempotencyConflict,
    UnsupportedCommand,
)
from scyg_agent.domain.ports.event_store import EventCursor
from scyg_agent.domain.ports.idempotency import RequestDigest
from scyg_agent.domain.runs import CommandKind, EventId

from .scripted_session_support import (
    COMMAND_ID,
    NOW,
    RUN_ID,
    FakeResult,
    ScriptedSession,
    command_record,
    run,
    session,
    submission,
)


@pytest.fixture
def anyio_backend() -> str:
    """使用 asyncio 后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_command_success_and_sequence_rejection(monkeypatch: pytest.MonkeyPatch) -> None:
    """覆盖首次成功及 stale sequence 审计路径。"""
    request = submission()
    success_script = ScriptedSession(
        [
            FakeResult("pending"),
            FakeResult(str(COMMAND_ID)),
            FakeResult(command_record(request)),
            FakeResult(run()),
            FakeResult(0),
            FakeResult("updated"),
            FakeResult(rows=()),
            FakeResult(0),
            FakeResult("notified"),
            FakeResult(0),
        ]
    )
    store = PostgreSQLCommandStore(async_sessionmaker(), "scyg_t12_test")
    result = await store.apply_in_session(session(monkeypatch, success_script), request)
    assert isinstance(result, CommandApplied)
    assert [type(value) for value in success_script.added] == [EventRecord, AuditEventRecord]

    stale_script = ScriptedSession(
        [
            FakeResult("pending"),
            FakeResult(str(COMMAND_ID)),
            FakeResult(command_record(request)),
            FakeResult(run()),
            FakeResult(4),
            FakeResult("user-t12"),
            FakeResult(0),
        ]
    )
    rejected = await store.apply_in_session(session(monkeypatch, stale_script), request)
    assert rejected == CommandRejected("sequence_mismatch", 1, 4, replayed=False)


@pytest.mark.anyio
async def test_command_owned_transaction_contains_missing_rows(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """覆盖命令自有事务的 Run 缺失和命令行损坏结果。"""
    request = submission()
    sessions = async_sessionmaker()
    store = PostgreSQLCommandStore(sessions, "scyg_t12_test")
    missing_run = ScriptedSession([FakeResult(None)])
    monkeypatch.setattr(sessions, "begin", _begin(session(monkeypatch, missing_run)))
    assert await store.apply(request) == CommandRunNotFound(RUN_ID)

    running = ScriptedSession([FakeResult("running")])
    monkeypatch.setattr(sessions, "begin", _begin(session(monkeypatch, running)))
    assert await store.apply(request) == UnsupportedCommand(COMMAND_ID, CommandKind.CANCEL)

    corrupt_command = ScriptedSession([FakeResult("pending"), FakeResult(None), FakeResult(None)])
    monkeypatch.setattr(sessions, "begin", _begin(session(monkeypatch, corrupt_command)))
    assert await store.apply(request) == CommandDataIntegrity(COMMAND_ID)


@pytest.mark.anyio
async def test_replay_success_conflict_rejection_and_corruption(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """覆盖原始结果重放的所有无数据库分支。"""
    request = submission()
    record = command_record(request, completed=True)
    event = EventRecord(
        event_id="evt_t12script0",
        run_id=str(RUN_ID),
        seq=1,
        revision=2,
        kind="run_cancelled",
        occurred_at=NOW,
        payload={"command_id": str(COMMAND_ID)},
    )
    cancelled = run("cancelled")
    cancelled.revision = 2
    scripted = ScriptedSession([FakeResult(cancelled), FakeResult(rows=(event,))])
    result = await replay_command(session(monkeypatch, scripted), record, request)
    assert isinstance(result, CommandApplied)
    assert result.events[0].cursor == EventCursor(1)
    assert result.events[0].event.event_id == EventId("evt_t12script0")

    empty = session(monkeypatch, ScriptedSession([]))
    assert await replay_command(
        empty, record, submission(RequestDigest.parse("8" * 64))
    ) == IdempotencyConflict(COMMAND_ID)
    record.result_status = "rejected"
    record.result_reference = "reject:revision_mismatch:3:5"
    assert await replay_command(empty, record, request) == CommandRejected(
        "revision_mismatch", 3, 5, replayed=True
    )
    record.result_reference = None
    assert await replay_command(empty, record, request) == CommandDataIntegrity(COMMAND_ID)


def _begin(value: AsyncSession) -> Callable[[], AbstractAsyncContextManager[AsyncSession]]:
    """构造仅供 async_sessionmaker.begin 替换使用的事务上下文工厂。"""

    @asynccontextmanager
    async def begin() -> AsyncIterator[AsyncSession]:
        yield value

    return begin
