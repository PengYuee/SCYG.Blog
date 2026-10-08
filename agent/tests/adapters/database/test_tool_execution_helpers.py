from datetime import datetime, timedelta
from uuid import UUID

import pytest
from sqlalchemy.ext.asyncio import async_sessionmaker

from scyg_agent.adapters.database.operation_records import ToolCallRecord
from scyg_agent.adapters.database.tool_execution import ToolExecutionTransactions
from scyg_agent.domain.ports.tool_store import (
    ClaimLost,
    ClaimRequest,
    ExistingInFlight,
    ExternalOutcomeUnknown,
    FirstClaim,
    StaleLeaseRecovered,
    TerminalReplay,
    ToolFence,
    ToolOperationNotFound,
)
from scyg_agent.domain.runs import OperationId
from scyg_agent.domain.runs.repository import LeaseToken

from .scripted_session_support import (
    NOW,
    RUN_ID,
    FakeResult,
    ScriptedSession,
    run,
    session,
    tool_operation,
    tool_record,
)

LEASE = timedelta(minutes=5)


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.mark.anyio
async def test_claim_helper_closes_pending_live_stale_and_terminal_branches(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: 调用方持有的脚本事务和四种工具状态。
    executor = ToolExecutionTransactions(async_sessionmaker())
    request = _claim(1, NOW)
    pending = tool_record(tool_operation())
    pending.status = "pending"
    pending.completed_at = None
    pending.claim_version = 0
    live = tool_record(tool_operation())
    live.status = "in_flight"
    live.completed_at = None
    live.claim_token = UUID(int=2)
    live.claim_version = 1
    live.claim_expires_at = NOW + LEASE
    stale = tool_record(tool_operation())
    stale.status = "in_flight"
    stale.completed_at = None
    stale.claim_token = UUID(int=3)
    stale.claim_version = 1
    stale.claim_expires_at = NOW

    # When: 分别领取未知、pending、有效、过期及终态记录。
    missing = await executor.claim_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(None)])), request
    )
    first = await executor.claim_in_session(session(monkeypatch, _locked(pending)), request)
    existing = await executor.claim_in_session(session(monkeypatch, _locked(live)), request)
    recovered = await executor.claim_in_session(session(monkeypatch, _locked(stale)), request)
    terminal = await executor.claim_in_session(
        session(monkeypatch, _locked(tool_record(tool_operation()))), request
    )

    # Then: 每个分支返回关闭类型且恢复版本单调增加。
    assert missing == ToolOperationNotFound(request.operation_id)
    assert isinstance(first, FirstClaim)
    assert existing == ExistingInFlight(request.operation_id)
    assert isinstance(recovered, StaleLeaseRecovered)
    assert recovered.fence.version == 2
    assert isinstance(terminal, TerminalReplay)


@pytest.mark.anyio
async def test_completion_rejects_stale_fence_and_appends_once(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: 一个当前 token/version 的 in-flight 记录。
    executor = ToolExecutionTransactions(async_sessionmaker())
    record = tool_record(tool_operation())
    record.status = "in_flight"
    record.completed_at = None
    record.claim_token = UUID(int=4)
    record.claim_version = 2
    record.claim_expires_at = NOW + LEASE
    fence = ToolFence(record_operation(record), LeaseToken(UUID(int=4)), 2, NOW)

    # A stale completion must not alter the current in-flight operation.
    lost_fence = ToolFence(fence.operation_id, fence.token, 1, NOW)
    lost = await executor.complete_in_session(
        session(monkeypatch, _locked(record)),
        lost_fence,
        tool_operation(),
    )
    assert record.status == "in_flight"
    assert record.claim_version == 2
    completed_script = _locked(record)
    completed_script.results.extend([FakeResult(run()), FakeResult(0)])
    completed = await executor.complete_in_session(
        session(monkeypatch, completed_script), fence, tool_operation()
    )

    # Then: 旧围栏丢失, 当前围栏保存终态并生成一个审计行。
    assert lost == ClaimLost(fence.operation_id)
    assert isinstance(completed, TerminalReplay)
    assert len(completed_script.added) == 1


@pytest.mark.anyio
async def test_post_rpc_expiry_closes_as_unknown(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: RPC 已开始且精确过期的 in-flight 记录。
    executor = ToolExecutionTransactions(async_sessionmaker())
    record = tool_record(tool_operation())
    record.status = "in_flight"
    record.completed_at = None
    record.claim_token = UUID(int=5)
    record.claim_version = 1
    record.claim_expires_at = NOW
    record.rpc_started_at = NOW - LEASE
    script = _locked(record)
    script.results.extend([FakeResult(run()), FakeResult(0)])

    # When: 新 token 在截止点尝试恢复。
    result = await executor.claim_in_session(session(monkeypatch, script), _claim(6, NOW))

    # Then: 操作关闭为未知外部结果且审计仅追加一次。
    assert result == ExternalOutcomeUnknown(record_operation(record))
    assert len(script.added) == 1


def _claim(token: int, now: datetime) -> ClaimRequest:
    """构造确定性工具 claim。"""
    operation = tool_operation().operation_id
    return ClaimRequest(operation, LeaseToken(UUID(int=token)), now, LEASE)


def _locked(record: ToolCallRecord) -> ScriptedSession:
    """构造固定 Run 后 operation 的加锁结果。"""
    return ScriptedSession([FakeResult(record), FakeResult(str(RUN_ID)), FakeResult(record)])


def record_operation(record: ToolCallRecord) -> OperationId:
    """把 ORM operation 文本恢复为测试领域身份。"""
    return OperationId(record.operation_id)
