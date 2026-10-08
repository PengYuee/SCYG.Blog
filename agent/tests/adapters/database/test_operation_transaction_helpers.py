import pytest
from sqlalchemy.ext.asyncio import async_sessionmaker

from scyg_agent.adapters.database.audit_append import append_audit_locked
from scyg_agent.adapters.database.audit_store import PostgreSQLAuditStore
from scyg_agent.adapters.database.interaction_store import PostgreSQLInteractionStore
from scyg_agent.adapters.database.operation_records import AuditEventRecord
from scyg_agent.adapters.database.tool_codec import decode_tool_operation
from scyg_agent.adapters.database.tool_store import (
    PostgreSQLToolOperationStore,
)
from scyg_agent.domain.ports.interaction_store import (
    InteractionConflict,
    InteractionDataIntegrity,
    InteractionIdempotencyConflict,
    InteractionPending,
    InteractionRequest,
)
from scyg_agent.domain.ports.tool_store import (
    ToolDataIntegrity,
    ToolIdempotencyConflict,
    ToolOperation,
    ToolOperationStored,
    ToolOutcomeStatus,
    ToolRunNotFound,
)

from .scripted_session_support import (
    INTERACTION_ID,
    NOW,
    RUN_ID,
    FakeResult,
    ScriptedSession,
    audit_fact,
    interaction,
    run,
    session,
    tool_operation,
    tool_record,
)


@pytest.fixture
def anyio_backend() -> str:
    """使用 asyncio 后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_interaction_create_unknown_duplicate_and_conflict(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """覆盖交互请求的未知 Run、首次请求、重放与冲突。"""
    store = PostgreSQLInteractionStore(async_sessionmaker(), "scyg_t12_test")
    request = InteractionRequest(INTERACTION_ID, RUN_ID, "approval", NOW)
    assert await store.request_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(None)])), request
    ) == InteractionConflict(INTERACTION_ID)
    waiting = run("waiting_input", INTERACTION_ID)
    first = ScriptedSession([FakeResult(waiting), FakeResult(str(INTERACTION_ID))])
    assert await store.request_in_session(
        session(monkeypatch, first), request
    ) == InteractionPending(INTERACTION_ID, replayed=False)
    duplicate = ScriptedSession(
        [
            FakeResult(waiting),
            FakeResult(None),
            FakeResult(interaction("pending", None)),
        ]
    )
    assert await store.request_in_session(
        session(monkeypatch, duplicate), request
    ) == InteractionPending(INTERACTION_ID, replayed=True)
    missing_duplicate = ScriptedSession([FakeResult(waiting), FakeResult(None), FakeResult(None)])
    assert await store.request_in_session(
        session(monkeypatch, missing_duplicate), request
    ) == InteractionDataIntegrity(INTERACTION_ID)
    conflicting = interaction("pending", None)
    conflicting.request_semantic_digest = "0" * 64
    conflict = ScriptedSession([FakeResult(waiting), FakeResult(None), FakeResult(conflicting)])
    assert await store.request_in_session(
        session(monkeypatch, conflict), request
    ) == InteractionIdempotencyConflict(INTERACTION_ID)


@pytest.mark.anyio
async def test_audit_and_tool_helpers(monkeypatch: pytest.MonkeyPatch) -> None:
    """覆盖审计序列、工具成功、冲突和未知 Run。"""
    fact = audit_fact()
    audit_script = ScriptedSession([FakeResult(7)])
    stored = await append_audit_locked(session(monkeypatch, audit_script), fact)
    assert stored.sequence == 8
    audit_store = PostgreSQLAuditStore(async_sessionmaker())
    locked = ScriptedSession([FakeResult(str(RUN_ID)), FakeResult(2)])
    assert (await audit_store.append_in_session(session(monkeypatch, locked), fact)).sequence == 3

    operation = tool_operation()
    tool_store = PostgreSQLToolOperationStore(async_sessionmaker())
    missing = ScriptedSession([FakeResult(None)])
    assert await tool_store.apply_in_session(
        session(monkeypatch, missing), operation
    ) == ToolRunNotFound(RUN_ID)
    conflicting = tool_record(operation)
    conflicting.request_digest = "0" * 64
    conflict = ScriptedSession([FakeResult(run()), FakeResult(None), FakeResult(conflicting)])
    assert await tool_store.apply_in_session(
        session(monkeypatch, conflict), operation
    ) == ToolIdempotencyConflict(operation.operation_id)
    success = ScriptedSession(
        [
            FakeResult(run()),
            FakeResult(str(operation.tool_call_id)),
            FakeResult(tool_record(operation)),
            FakeResult(0),
        ]
    )
    assert await tool_store.apply_in_session(
        session(monkeypatch, success), operation
    ) == ToolOperationStored(operation, replayed=False)
    assert isinstance(success.added[0], AuditEventRecord)


def test_decode_failed_tool_and_corruption() -> None:
    """覆盖失败工具结果恢复和损坏元数据。"""
    record = tool_record(tool_operation(ToolOutcomeStatus.FAILED))
    decoded = decode_tool_operation(record)
    assert isinstance(decoded, ToolOperation)
    assert decoded.error_code == "tool_timeout"
    record.error_metadata = None
    assert decode_tool_operation(record) == ToolDataIntegrity(None)
