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
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest, ResultReference
from scyg_agent.domain.ports.interaction_store import (
    AlreadyResolved,
    InteractionConflict,
    InteractionDataIntegrity,
    InteractionIdempotencyConflict,
    InteractionNotFound,
    InteractionPending,
    InteractionRequest,
)
from scyg_agent.domain.ports.tool_store import (
    DecisionConflict,
    SemanticIdentityConflict,
    ToolDataIntegrity,
    ToolIdempotencyConflict,
    ToolIntent,
    ToolIntentPrepared,
    ToolOperation,
    ToolOperationStored,
    ToolOutcomeStatus,
    ToolRunNotFound,
)
from scyg_agent.domain.runs import InteractionId, OperationId, ToolCallId

from .scripted_session_support import (
    INTERACTION_ID,
    NOW,
    RUN_ID,
    FakeResult,
    ScriptedSession,
    audit_fact,
    interaction,
    resolution,
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
async def test_interaction_create_unknown_duplicate_and_resolved(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """覆盖交互调用方会话的全部幂等分支。"""
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
    resolved = ScriptedSession([FakeResult(interaction("resolved", "approval:accepted"))])
    assert await store.resolve_in_session(
        session(monkeypatch, resolved), resolution()
    ) == AlreadyResolved(INTERACTION_ID, ResultReference("approval:accepted"))
    corrupt = interaction("resolved", None)
    assert await store.resolve_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(corrupt)])), resolution()
    ) == InteractionDataIntegrity(INTERACTION_ID)
    changed = interaction("resolved", "approval:accepted")
    changed.resolution_semantic_digest = "0" * 64
    assert await store.resolve_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(changed)])), resolution()
    ) == InteractionIdempotencyConflict(INTERACTION_ID)
    unknown = ScriptedSession([FakeResult(None)])
    assert await store.resolve_in_session(
        session(monkeypatch, unknown), resolution()
    ) == InteractionNotFound(INTERACTION_ID)


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


@pytest.mark.anyio
async def test_resolve_and_prepare_binds_approval_and_intent_identity(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """覆盖审批解析与 pending 意图的原子首次、语义冲突和决策冲突。"""
    store = PostgreSQLInteractionStore(async_sessionmaker(), "scyg_t17_test")
    intent = ToolIntent(
        ToolCallId("tool_t17script0"),
        OperationId("t17:script:tool"),
        RUN_ID,
        "update_article",
        RequestDigest.parse("7" * 64),
        INTERACTION_ID,
        NOW,
        AuditMetadata("source", "runtime"),
    )
    prepared = ScriptedSession(
        [
            FakeResult(run()),
            FakeResult(interaction("resolved", "approval:accepted")),
            FakeResult(str(intent.operation_id)),
        ]
    )
    first = await store.resolve_and_prepare_in_session(
        session(monkeypatch, prepared), resolution(), intent
    )
    assert first == ToolIntentPrepared(intent.operation_id, replayed=False)

    conflicting_record = tool_record(tool_operation())
    conflicting_record.intent_semantic_digest = "0" * 64
    semantic = ScriptedSession(
        [
            FakeResult(run()),
            FakeResult(interaction("resolved", "approval:accepted")),
            FakeResult(None),
            FakeResult(conflicting_record),
        ]
    )
    conflict = await store.resolve_and_prepare_in_session(
        session(monkeypatch, semantic), resolution(), intent
    )
    assert conflict == SemanticIdentityConflict(intent.operation_id)

    changed = ToolIntent(
        intent.tool_call_id,
        intent.operation_id,
        intent.run_id,
        intent.tool_name,
        intent.request_digest,
        InteractionId("int_t17changed0"),
        intent.prepared_at,
        intent.audit_metadata,
    )
    decision = await store.resolve_and_prepare_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(run())])), resolution(), changed
    )
    assert decision == DecisionConflict(intent.operation_id)


def test_decode_failed_tool_and_corruption() -> None:
    """覆盖失败工具结果恢复和损坏元数据。"""
    record = tool_record(tool_operation(ToolOutcomeStatus.FAILED))
    decoded = decode_tool_operation(record)
    assert isinstance(decoded, ToolOperation)
    assert decoded.error_code == "tool_timeout"
    record.error_metadata = None
    assert decode_tool_operation(record) == ToolDataIntegrity(None)
