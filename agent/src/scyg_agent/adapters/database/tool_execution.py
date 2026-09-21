"""工具意图的短事务租约、RPC 标记和围栏完成。."""

from datetime import datetime
from hashlib import sha256
from typing import final

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.ports.audit_store import AuditFact
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest, ResultMetadata
from scyg_agent.domain.ports.semantic_identity import tool_semantic_digest
from scyg_agent.domain.ports.tool_store import (
    ClaimLost,
    ClaimRequest,
    ExistingInFlight,
    ExternalOutcomeUnknown,
    FirstClaim,
    StaleLeaseRecovered,
    TerminalReplay,
    ToolClaimResult,
    ToolDataIntegrity,
    ToolFence,
    ToolFenceResult,
    ToolOperation,
    ToolOperationNotFound,
    ToolOutcomeStatus,
)
from scyg_agent.domain.runs import OperationId, RunId, ToolCallId, UserId

from .audit_append import append_audit_locked
from .operation_records import ToolCallRecord
from .run_records import RunRecord
from .tool_codec import decode_tool_operation


@final
class ToolExecutionTransactions:
    """在独立短事务中执行工具操作状态转换。."""

    def __init__(self, sessions: async_sessionmaker[AsyncSession]) -> None:
        """绑定共享会话工厂。."""
        self._sessions = sessions

    async def claim(self, request: ClaimRequest) -> ToolClaimResult:
        """领取 pending, 或按 RPC 提交点保守处理过期租约。."""
        async with self._sessions.begin() as session:
            return await self.claim_in_session(session, request)

    async def claim_in_session(
        self, session: AsyncSession, request: ClaimRequest
    ) -> ToolClaimResult:
        """在调用方短事务中执行一次关闭的 claim 状态转换。."""
        record = await _lock_record_in_run_order(session, request.operation_id)
        if record is None:
            return ToolOperationNotFound(request.operation_id)
        if record.status in _TERMINAL_STATUSES:
            return _terminal(record)
        if record.status == "pending":
            return _set_claim(record, request, recovered=False)
        if record.claim_expires_at is None or record.claim_expires_at > request.now:
            return ExistingInFlight(request.operation_id)
        if record.rpc_started_at is None:
            return _set_claim(record, request, recovered=True)
        return await _mark_unknown(session, record, request.now)

    async def mark_rpc_started(self, fence: ToolFence) -> ToolFenceResult | FirstClaim:
        """只允许当前 token/version 标记 RPC 开始。."""
        async with self._sessions.begin() as session:
            return await self.mark_rpc_started_in_session(session, fence)

    async def mark_rpc_started_in_session(
        self, session: AsyncSession, fence: ToolFence
    ) -> ToolFenceResult | FirstClaim:
        """在调用方短事务中围栏标记 RPC 已开始。."""
        record = await _lock_record_in_run_order(session, fence.operation_id)
        if record is None:
            return ToolDataIntegrity(fence.operation_id)
        if record.status in _TERMINAL_STATUSES:
            return _terminal(record)
        if not _owns(record, fence):
            return ClaimLost(fence.operation_id)
        record.rpc_started_at = fence.occurred_at
        return FirstClaim(fence)

    async def complete(self, fence: ToolFence, outcome: ToolOperation) -> ToolFenceResult:
        """验证身份与围栏后原子保存终态和审计。."""
        async with self._sessions.begin() as session:
            return await self.complete_in_session(session, fence, outcome)

    async def complete_in_session(
        self, session: AsyncSession, fence: ToolFence, outcome: ToolOperation
    ) -> ToolFenceResult:
        """在调用方短事务中围栏保存终态和唯一审计。."""
        record = await _lock_record_in_run_order(session, fence.operation_id)
        if record is None:
            return ToolDataIntegrity(fence.operation_id)
        if record.status in _TERMINAL_STATUSES:
            return _terminal(record)
        if not _owns(record, fence) or not _same_intent(record, outcome):
            return ClaimLost(fence.operation_id)
        record.status = outcome.status.value
        record.semantic_digest = str(tool_semantic_digest(outcome))
        record.result_reference = (
            outcome.result_reference.value if outcome.result_reference is not None else None
        )
        record.result_metadata = (
            {outcome.metadata.key: outcome.metadata.value}
            if outcome.status is ToolOutcomeStatus.SUCCEEDED
            else None
        )
        record.error_code = outcome.error_code
        record.error_metadata = (
            None
            if outcome.status is ToolOutcomeStatus.SUCCEEDED
            else {outcome.metadata.key: outcome.metadata.value}
        )
        record.completed_at = outcome.occurred_at
        record.claim_token = None
        record.claim_expires_at = None
        await _append_audit(session, record, outcome)
        return TerminalReplay(outcome)


_TERMINAL_STATUSES = frozenset(
    ("succeeded", "failed", ToolOutcomeStatus.EXTERNAL_OUTCOME_UNKNOWN.value)
)


async def _lock_record_in_run_order(
    session: AsyncSession, operation_id: OperationId
) -> ToolCallRecord | None:
    """先读取身份, 再按 Run、operation 固定顺序加锁。."""
    candidate = (
        await session.execute(
            select(ToolCallRecord).where(ToolCallRecord.operation_id == str(operation_id))
        )
    ).scalar_one_or_none()
    if candidate is None:
        return None
    _ = await session.execute(
        select(RunRecord).where(RunRecord.run_id == candidate.run_id).with_for_update()
    )
    return (
        await session.execute(
            select(ToolCallRecord)
            .where(ToolCallRecord.operation_id == str(operation_id))
            .with_for_update()
        )
    ).scalar_one_or_none()


def _set_claim(
    record: ToolCallRecord, request: ClaimRequest, *, recovered: bool
) -> FirstClaim | StaleLeaseRecovered:
    """替换围栏并单调增加版本。."""
    record.status = "in_flight"
    record.claim_token = request.token.value
    record.claim_version += 1
    record.claim_expires_at = request.now + request.lease_duration
    fence = ToolFence(request.operation_id, request.token, record.claim_version, request.now)
    return StaleLeaseRecovered(fence) if recovered else FirstClaim(fence)


def _owns(record: ToolCallRecord, fence: ToolFence) -> bool:
    """比较不可猜测 token 与单调版本。."""
    return record.claim_token == fence.token.value and record.claim_version == fence.version


def _same_intent(record: ToolCallRecord, outcome: ToolOperation) -> bool:
    """完成结果必须绑定持久化调用前身份。."""
    return (
        record.operation_id == str(outcome.operation_id)
        and record.tool_call_id == str(outcome.tool_call_id)
        and record.run_id == str(outcome.run_id)
        and record.tool_name == outcome.tool_name
        and record.request_digest == str(outcome.request_digest)
    )


def _terminal(
    record: ToolCallRecord,
) -> TerminalReplay | ExternalOutcomeUnknown | ToolDataIntegrity:
    """恢复关闭终态而不产生任何新写入。."""
    decoded = decode_tool_operation(record)
    if isinstance(decoded, ToolDataIntegrity):
        return decoded
    if decoded.status is ToolOutcomeStatus.EXTERNAL_OUTCOME_UNKNOWN:
        return ExternalOutcomeUnknown(decoded.operation_id)
    return TerminalReplay(decoded)


async def _mark_unknown(
    session: AsyncSession, record: ToolCallRecord, occurred_at: datetime
) -> ExternalOutcomeUnknown:
    """将已开始 RPC 的过期租约确定性关闭为未知外部结果。."""
    operation = ToolOperation(
        ToolCallId(record.tool_call_id),
        OperationId(record.operation_id),
        RunId(record.run_id),
        record.tool_name,
        RequestDigest.parse(record.request_digest),
        ToolOutcomeStatus.EXTERNAL_OUTCOME_UNKNOWN,
        None,
        ResultMetadata("classification", "external_outcome_unknown"),
        "external_outcome_unknown",
        occurred_at,
        _audit_metadata(record),
    )
    record.status = operation.status.value
    record.semantic_digest = str(tool_semantic_digest(operation))
    record.error_code = operation.error_code
    record.error_metadata = {operation.metadata.key: operation.metadata.value}
    record.completed_at = occurred_at
    record.claim_token = None
    record.claim_expires_at = None
    await _append_audit(session, record, operation)
    return ExternalOutcomeUnknown(operation.operation_id)


def _audit_metadata(record: ToolCallRecord) -> AuditMetadata:
    """恢复单个清洗审计属性。."""
    key, value = next(iter(record.request_audit_metadata.items()))
    return AuditMetadata(key, value)


async def _append_audit(
    session: AsyncSession, record: ToolCallRecord, operation: ToolOperation
) -> None:
    """仅在首次终态转换时追加工具审计事件。."""
    run = (
        await session.execute(select(RunRecord).where(RunRecord.run_id == record.run_id))
    ).scalar_one()
    digest = sha256(f"tool:{operation.tool_call_id}".encode()).hexdigest()
    _ = await append_audit_locked(
        session,
        AuditFact(
            f"audit_t_{digest[:56]}",
            operation.run_id,
            UserId(run.owner_user_id),
            None,
            operation.tool_call_id,
            operation.tool_name,
            operation.status.value,
            operation.occurred_at,
            operation.audit_metadata,
        ),
    )
