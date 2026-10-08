"""PostgreSQL 工具操作幂等终态存储。."""

from hashlib import sha256
from typing import final

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.ports.audit_store import AuditFact
from scyg_agent.domain.ports.semantic_identity import tool_semantic_digest
from scyg_agent.domain.ports.tool_store import (
    ClaimRequest,
    ToolClaimResult,
    ToolDataIntegrity,
    ToolFence,
    ToolFenceResult,
    ToolGetResult,
    ToolIdempotencyConflict,
    ToolOperation,
    ToolOperationNotFound,
    ToolOperationStored,
    ToolOutcomeStatus,
    ToolRunNotFound,
    ToolStoreResult,
)
from scyg_agent.domain.runs import OperationId, ToolCallId, UserId

from .audit_append import append_audit_locked
from .operation_records import ToolCallRecord
from .run_records import RunRecord
from .tool_codec import decode_tool_operation
from .tool_execution import ToolExecutionTransactions


@final
class PostgreSQLToolOperationStore:
    """用 operation_id 唯一约束仲裁工具结果写入。."""

    def __init__(self, sessions: async_sessionmaker[AsyncSession]) -> None:
        """绑定短事务会话工厂。."""
        self._sessions = sessions

    async def apply(self, request: ToolOperation) -> ToolStoreResult:
        """保存一个终态或重放同一操作的原始终态。."""
        async with self._sessions.begin() as session:
            return await self.apply_in_session(session, request)

    async def apply_in_session(
        self, session: AsyncSession, request: ToolOperation
    ) -> ToolStoreResult:
        """在调用方会话中保存或重放工具终态。."""
        run = (
            await session.execute(
                select(RunRecord).where(RunRecord.run_id == str(request.run_id)).with_for_update()
            )
        ).scalar_one_or_none()
        if run is None:
            return ToolRunNotFound(request.run_id)
        inserted = (
            await session.execute(
                insert(ToolCallRecord)
                .values(
                    tool_call_id=str(request.tool_call_id),
                    operation_id=str(request.operation_id),
                    run_id=str(request.run_id),
                    tool_name=request.tool_name,
                    status=request.status.value,
                    request_digest=str(request.request_digest),
                    semantic_digest=str(tool_semantic_digest(request)),
                    intent_semantic_digest=None,
                    approval_interaction_id=None,
                    approval_resolution_digest=None,
                    request_audit_metadata={
                        request.audit_metadata.key: request.audit_metadata.value
                    },
                    result_reference=(
                        request.result_reference.value
                        if request.result_reference is not None
                        else None
                    ),
                    result_metadata=(
                        {request.metadata.key: request.metadata.value}
                        if request.status is ToolOutcomeStatus.SUCCEEDED
                        else None
                    ),
                    error_code=request.error_code,
                    error_metadata=(
                        {request.metadata.key: request.metadata.value}
                        if request.status is ToolOutcomeStatus.FAILED
                        else None
                    ),
                    started_at=request.occurred_at,
                    completed_at=request.occurred_at,
                    claim_token=None,
                    claim_version=0,
                    claim_expires_at=None,
                    rpc_started_at=None,
                )
                .on_conflict_do_nothing(index_elements=[ToolCallRecord.operation_id])
                .returning(ToolCallRecord.tool_call_id)
            )
        ).scalar_one_or_none()
        record = (
            await session.execute(
                select(ToolCallRecord)
                .where(ToolCallRecord.operation_id == str(request.operation_id))
                .with_for_update()
            )
        ).scalar_one_or_none()
        if record is None:
            return ToolDataIntegrity(request.operation_id)
        if inserted is None:
            if (
                record.run_id != str(request.run_id)
                or record.tool_name != request.tool_name
                or record.request_digest != str(request.request_digest)
                or record.semantic_digest != str(tool_semantic_digest(request))
            ):
                return ToolIdempotencyConflict(request.operation_id)
            decoded = decode_tool_operation(record)
            if isinstance(decoded, ToolDataIntegrity):
                return decoded
            return ToolOperationStored(decoded, replayed=True)
        _ = await append_audit_locked(
            session,
            AuditFact(
                _tool_audit_id(request.tool_call_id),
                request.run_id,
                UserId(run.owner_user_id),
                None,
                request.tool_call_id,
                request.tool_name,
                request.status.value,
                request.occurred_at,
                request.audit_metadata,
            ),
        )
        return ToolOperationStored(request, replayed=False)

    async def get(self, operation_id: OperationId) -> ToolGetResult:
        """读取原始工具终态或返回类型化未知结果。."""
        async with self._sessions() as session:
            record = (
                await session.execute(
                    select(ToolCallRecord).where(ToolCallRecord.operation_id == str(operation_id))
                )
            ).scalar_one_or_none()
            if record is None:
                return ToolOperationNotFound(operation_id)
            decoded = decode_tool_operation(record)
            if isinstance(decoded, ToolDataIntegrity):
                return decoded
            return ToolOperationStored(decoded, replayed=True)

    async def claim(self, request: ClaimRequest) -> ToolClaimResult:
        """在短事务中竞争或恢复工具执行租约。."""
        return await ToolExecutionTransactions(self._sessions).claim(request)

    async def complete(self, fence: ToolFence, outcome: ToolOperation) -> ToolFenceResult:
        """围栏完成工具终态并追加唯一审计事实。."""
        return await ToolExecutionTransactions(self._sessions).complete(fence, outcome)


def _tool_audit_id(tool_call_id: ToolCallId) -> str:
    """构造与命令审计命名空间隔离的稳定身份。."""
    digest = sha256(f"tool:{tool_call_id}".encode()).hexdigest()
    return f"audit_t_{digest[:56]}"
