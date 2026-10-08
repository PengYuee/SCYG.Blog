"""PostgreSQL 一次性交互存储适配器。."""

from typing import final

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.ports.command_store import CommandApplied
from scyg_agent.domain.ports.idempotency import ResultReference
from scyg_agent.domain.ports.interaction_store import (
    AlreadyResolved,
    InteractionConflict,
    InteractionCreateResult,
    InteractionDataIntegrity,
    InteractionIdempotencyConflict,
    InteractionNotFound,
    InteractionPending,
    InteractionRequest,
    InteractionResolution,
    InteractionResolveResult,
)
from scyg_agent.domain.ports.semantic_identity import (
    command_semantic_digest,
    interaction_request_digest,
    interaction_resolution_digest,
    tool_intent_semantic_digest,
)
from scyg_agent.domain.ports.tool_store import (
    DecisionConflict,
    SemanticIdentityConflict,
    ToolIntent,
    ToolIntentPrepared,
)

from .command_failpoints import NO_COMMAND_FAILPOINT, CommandFailpoint
from .command_store import PostgreSQLCommandStore
from .operation_records import ToolCallRecord
from .run_records import InteractionRecord, RunRecord

type ResolveAndPrepareResult = (
    InteractionResolveResult | ToolIntentPrepared | SemanticIdentityConflict | DecisionConflict
)


@final
class PostgreSQLInteractionStore:
    """以交互行锁选择唯一解析赢家。."""

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        notification_channel: str,
        failpoint: CommandFailpoint = NO_COMMAND_FAILPOINT,
    ) -> None:
        """绑定共享事务资源。."""
        self._sessions = sessions
        self._commands = PostgreSQLCommandStore(sessions, notification_channel, failpoint)

    async def request(self, request: InteractionRequest) -> InteractionCreateResult:
        """创建交互, Run 当前待处理身份必须与其一致。."""
        async with self._sessions.begin() as session:
            return await self.request_in_session(session, request)

    async def request_in_session(
        self, session: AsyncSession, request: InteractionRequest
    ) -> InteractionCreateResult:
        """在调用方会话中创建或重放待处理交互。."""
        run = (
            await session.execute(
                select(RunRecord).where(RunRecord.run_id == str(request.run_id)).with_for_update()
            )
        ).scalar_one_or_none()
        if run is None or run.pending_interaction_id != str(request.interaction_id):
            return InteractionConflict(request.interaction_id)
        inserted = (
            await session.execute(
                insert(InteractionRecord)
                .values(
                    interaction_id=str(request.interaction_id),
                    run_id=str(request.run_id),
                    kind=request.kind,
                    status="pending",
                    requested_at=request.requested_at,
                    request_semantic_digest=str(
                        interaction_request_digest(
                            str(request.interaction_id),
                            str(request.run_id),
                            request.kind,
                            request.requested_at.isoformat(),
                        )
                    ),
                    resolved_at=None,
                    response_digest=None,
                    resolution_semantic_digest=None,
                    result_reference=None,
                )
                .on_conflict_do_nothing(index_elements=[InteractionRecord.interaction_id])
                .returning(InteractionRecord.interaction_id)
            )
        ).scalar_one_or_none()
        if inserted is not None:
            return InteractionPending(request.interaction_id, replayed=False)
        existing = (
            await session.execute(
                select(InteractionRecord).where(
                    InteractionRecord.interaction_id == str(request.interaction_id)
                )
            )
        ).scalar_one_or_none()
        if existing is None:
            return InteractionDataIntegrity(request.interaction_id)
        expected_digest = interaction_request_digest(
            str(request.interaction_id),
            str(request.run_id),
            request.kind,
            request.requested_at.isoformat(),
        )
        if (
            existing.run_id == str(request.run_id)
            and existing.kind == request.kind
            and existing.request_semantic_digest == str(expected_digest)
        ):
            return InteractionPending(request.interaction_id, replayed=True)
        return InteractionIdempotencyConflict(request.interaction_id)

    async def resolve(self, request: InteractionResolution) -> InteractionResolveResult:
        """锁定交互并让一个响应进入命令事务逻辑。."""
        async with self._sessions.begin() as session:
            return await self.resolve_in_session(session, request)

    async def resolve_and_prepare(
        self, request: InteractionResolution, intent: ToolIntent
    ) -> ResolveAndPrepareResult:
        """原子解析审批并插入唯一待执行工具意图。."""
        async with self._sessions.begin() as session:
            return await self.resolve_and_prepare_in_session(session, request, intent)

    async def resolve_and_prepare_in_session(
        self, session: AsyncSession, request: InteractionResolution, intent: ToolIntent
    ) -> ResolveAndPrepareResult:
        """在调用方事务中解析审批并插入唯一 pending 意图。."""
        run = (
            await session.execute(
                select(RunRecord).where(RunRecord.run_id == str(intent.run_id)).with_for_update()
            )
        ).scalar_one_or_none()
        if run is None or intent.approval_interaction_id != request.interaction_id:
            return DecisionConflict(intent.operation_id)
        interaction = (
            await session.execute(
                select(InteractionRecord)
                .where(
                    InteractionRecord.interaction_id == str(request.interaction_id),
                    InteractionRecord.run_id == str(intent.run_id),
                )
                .with_for_update()
            )
        ).scalar_one_or_none()
        if (
            interaction is not None
            and interaction.status == "resolved"
            and interaction.decision is not None
        ):
            if interaction.decision != "approve" or interaction.resolution_semantic_digest is None:
                return DecisionConflict(intent.operation_id)
            resolution_digest = interaction.resolution_semantic_digest
        else:
            resolved = await self.resolve_in_session(session, request)
            if not isinstance(resolved, (CommandApplied, AlreadyResolved)):
                return resolved
            resolution_digest = str(
                interaction_resolution_digest(
                    request.response_digest,
                    request.result_reference.value,
                    command_semantic_digest(request.command),
                )
            )
        inserted = (
            await session.execute(
                insert(ToolCallRecord)
                .values(
                    tool_call_id=str(intent.tool_call_id),
                    operation_id=str(intent.operation_id),
                    run_id=str(intent.run_id),
                    tool_name=intent.tool_name,
                    status="pending",
                    request_digest=str(intent.request_digest),
                    semantic_digest="0" * 64,
                    intent_semantic_digest=str(tool_intent_semantic_digest(intent)),
                    approval_interaction_id=str(intent.approval_interaction_id),
                    approval_resolution_digest=str(resolution_digest),
                    request_audit_metadata={intent.audit_metadata.key: intent.audit_metadata.value},
                    result_reference=None,
                    result_metadata=None,
                    error_code=None,
                    error_metadata=None,
                    started_at=intent.prepared_at,
                    completed_at=None,
                    claim_token=None,
                    claim_version=0,
                    claim_expires_at=None,
                    rpc_started_at=None,
                )
                .on_conflict_do_nothing(index_elements=[ToolCallRecord.operation_id])
                .returning(ToolCallRecord.operation_id)
            )
        ).scalar_one_or_none()
        if inserted is not None:
            return ToolIntentPrepared(intent.operation_id, replayed=False)
        existing = (
            await session.execute(
                select(ToolCallRecord)
                .where(ToolCallRecord.operation_id == str(intent.operation_id))
                .with_for_update()
            )
        ).scalar_one_or_none()
        if existing is None or existing.intent_semantic_digest != str(
            tool_intent_semantic_digest(intent)
        ):
            prepared: ResolveAndPrepareResult = SemanticIdentityConflict(intent.operation_id)
        elif existing.approval_resolution_digest != str(resolution_digest):
            prepared = DecisionConflict(intent.operation_id)
        else:
            prepared = ToolIntentPrepared(intent.operation_id, replayed=True)
        return prepared

    async def resolve_in_session(
        self, session: AsyncSession, request: InteractionResolution
    ) -> InteractionResolveResult:
        """在调用方会话中选择解析赢家并应用命令。."""
        run = (
            await session.execute(
                select(RunRecord)
                .where(RunRecord.run_id == str(request.command.run_id))
                .with_for_update()
            )
        ).scalar_one_or_none()
        if run is None:
            return InteractionNotFound(request.interaction_id)
        interaction = (
            await session.execute(
                select(InteractionRecord)
                .where(InteractionRecord.interaction_id == str(request.interaction_id))
                .with_for_update()
            )
        ).scalar_one_or_none()
        if interaction is None:
            return InteractionNotFound(request.interaction_id)
        if interaction.status == "resolved":
            reference = interaction.result_reference
            persisted_digest = interaction.resolution_semantic_digest
            if reference is None or persisted_digest is None:
                return InteractionDataIntegrity(request.interaction_id)
            expected_digest = interaction_resolution_digest(
                request.response_digest,
                request.result_reference.value,
                command_semantic_digest(request.command),
            )
            if persisted_digest != str(expected_digest):
                return InteractionIdempotencyConflict(request.interaction_id)
            return AlreadyResolved(request.interaction_id, ResultReference(reference))
        result = await self._commands.apply_in_session(session, request.command)
        if isinstance(result, CommandApplied):
            interaction.status = "resolved"
            interaction.resolved_at = request.command.submitted_at
            interaction.response_digest = str(request.response_digest)
            interaction.resolution_semantic_digest = str(
                interaction_resolution_digest(
                    request.response_digest,
                    request.result_reference.value,
                    command_semantic_digest(request.command),
                )
            )
            interaction.result_reference = request.result_reference.value
        return result
