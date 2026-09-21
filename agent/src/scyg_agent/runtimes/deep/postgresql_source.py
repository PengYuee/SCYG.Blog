"""Deep 提案与恢复决定的 PostgreSQL/checkpoint 生产来源。."""

from collections.abc import Callable
from dataclasses import dataclass
from enum import StrEnum
from hashlib import sha256
from typing import override

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.blog_grpc import CorrelationId, RequestId, RequestIdentity, SearchArticles
from scyg_agent.adapters.database.operation_records import ToolCallRecord
from scyg_agent.adapters.database.run_records import InteractionRecord
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.interaction_store import InteractionResolution
from scyg_agent.domain.ports.tool_store import ToolIntent, ToolOperation, ToolOutcomeStatus
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    Run,
    SubmitInput,
    ToolCallId,
)
from scyg_agent.domain.runs.input import RunInput

from .models import (
    ApprovalDecision,
    ApprovalReply,
    DeepFailureKind,
    DeepRuntimeError,
    ProposalEnvelope,
)
from .runtime import DeepRuntime

APPROVAL_ACCEPTED = "approval:accepted"
APPROVAL_REJECTED = "approval:rejected"


class ResumeStateKind(StrEnum):
    """关闭所有可持久恢复的审批/操作状态。."""

    PENDING = "pending"
    REJECTED = "rejected"
    APPROVED = "approved"
    TERMINAL = "terminal"
    EXTERNAL_OUTCOME_UNKNOWN = "external_outcome_unknown"


@dataclass(frozen=True, slots=True)
class PersistedResumeState:
    """返回 Agent 真相确定的封闭恢复状态。."""

    kind: ResumeStateKind
    reply: ApprovalReply | None


@dataclass(frozen=True, slots=True)
class IncompleteDeepTruthError(RuntimeError):
    """报告无法安全恢复的不完整持久化事实。."""

    @override
    def __str__(self) -> str:
        """返回不包含记录值的稳定中文错误。."""
        return "Deep 持久化事实不完整"


@dataclass(frozen=True, slots=True)
class PersistedProposalSource:
    """优先读取 checkpoint 精确封套,无快照时从 Run 真相确定性构造。."""

    sessions: async_sessionmaker[AsyncSession]
    runtime_for: Callable[[Run], DeepRuntime]

    async def proposal_for(self, run: Run, run_input: RunInput) -> ProposalEnvelope:
        """恢复精确 checkpoint 提案或构造首次执行提案。."""
        runtime: DeepRuntime = self.runtime_for(run)
        try:
            state, _next_nodes = await runtime.snapshot(run.id)
        except DeepRuntimeError as error:
            if error.kind is not DeepFailureKind.INVALID_CHECKPOINT:
                raise
        else:
            proposal = state["proposal"]
            if proposal.intent.run_id != run.id:
                raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
            return proposal
        reference = await self._resolution_reference(run)
        return _build_proposal(run, run_input, reference)

    async def _resolution_reference(self, run: Run) -> ResultReference:
        """读取已解析交互引用,首次执行则使用批准模板。."""
        if run.pending_interaction_id is None:
            return ResultReference(APPROVAL_ACCEPTED)
        async with self.sessions() as session:
            record = (
                await session.execute(
                    select(InteractionRecord).where(
                        InteractionRecord.interaction_id == str(run.pending_interaction_id)
                    )
                )
            ).scalar_one_or_none()
        if record is None or record.result_reference is None:
            return ResultReference(APPROVAL_ACCEPTED)
        return ResultReference(record.result_reference)


@dataclass(frozen=True, slots=True)
class PersistedResumeSource:
    """按 tool operation、interaction、checkpoint 顺序解析恢复决定。."""

    sessions: async_sessionmaker[AsyncSession]
    runtime_for: Callable[[Run], DeepRuntime]

    async def state_for(self, run: Run, proposal: ProposalEnvelope) -> PersistedResumeState:
        """让 Agent 操作事实覆盖 checkpoint 的任何歧义。."""
        async with self.sessions() as session:
            operation = (
                await session.execute(
                    select(ToolCallRecord).where(
                        ToolCallRecord.operation_id == str(proposal.intent.operation_id)
                    )
                )
            ).scalar_one_or_none()
            interaction = (
                await session.execute(
                    select(InteractionRecord).where(
                        InteractionRecord.interaction_id == str(proposal.resolution.interaction_id)
                    )
                )
            ).scalar_one_or_none()
        if operation is not None:
            return self.operation_state(proposal, operation)
        if interaction is None or interaction.status == "pending":
            state, _next_nodes = await self.runtime_for(run).snapshot(run.id)
            decision = state.get("decision")
            if decision is None:
                return PersistedResumeState(ResumeStateKind.PENDING, None)
            parsed = ApprovalDecision(decision)
            kind = (
                ResumeStateKind.APPROVED
                if parsed is ApprovalDecision.APPROVE
                else ResumeStateKind.REJECTED
            )
            return PersistedResumeState(kind, ApprovalReply(proposal.approval_token, parsed))
        return self.decision_state(proposal, interaction.result_reference)

    @staticmethod
    def decision_state(proposal: ProposalEnvelope, reference: str | None) -> PersistedResumeState:
        """把交互结果引用解析为精确审批决定。."""
        if reference == APPROVAL_REJECTED:
            decision = ApprovalDecision.REJECT
            kind = ResumeStateKind.REJECTED
        elif reference == APPROVAL_ACCEPTED:
            decision = ApprovalDecision.APPROVE
            kind = ResumeStateKind.APPROVED
        else:
            raise IncompleteDeepTruthError
        return PersistedResumeState(kind, ApprovalReply(proposal.approval_token, decision))

    async def reply_for(self, run: Run, proposal: ProposalEnvelope) -> ApprovalReply:
        """返回可推进图的决定,pending 则类型化失败。."""
        state = await self.state_for(run, proposal)
        if state.reply is None:
            raise IncompleteDeepTruthError
        return state.reply

    @staticmethod
    def operation_state(
        proposal: ProposalEnvelope, operation: ToolCallRecord
    ) -> PersistedResumeState:
        """把操作状态映射为不重复外部调用的恢复决定。."""
        reply = ApprovalReply(proposal.approval_token, ApprovalDecision.APPROVE)
        if operation.status == ToolOutcomeStatus.EXTERNAL_OUTCOME_UNKNOWN.value:
            return PersistedResumeState(ResumeStateKind.EXTERNAL_OUTCOME_UNKNOWN, reply)
        if operation.status in {ToolOutcomeStatus.SUCCEEDED.value, ToolOutcomeStatus.FAILED.value}:
            return PersistedResumeState(ResumeStateKind.TERMINAL, reply)
        if operation.status in {"pending", "in_flight"}:
            return PersistedResumeState(ResumeStateKind.APPROVED, reply)
        raise IncompleteDeepTruthError


def _build_proposal(run: Run, run_input: RunInput, reference: ResultReference) -> ProposalEnvelope:
    """以 Run 输入和品牌身份构造可重启的固定搜索提案。."""
    digest = sha256(
        f"{run.id}:{run.task_type.value}:{run_input.initial_message}".encode()
    ).hexdigest()
    interaction_id = run.pending_interaction_id or InteractionId(f"int_{digest[:16]}")
    command_id = CommandId(f"cmd_{digest[16:32]}")
    tool_call_id = ToolCallId(f"tool_{digest[32:48]}")
    operation_id = OperationId(f"deep:{run.task_type.value}:{digest[:32]}")
    request_digest = RequestDigest.parse(digest)
    command = SubmitInput(
        command_id, EventId(f"evt_{digest[48:64]}"), run.revision, run.updated_at, interaction_id
    )
    submission = CommandSubmission(
        command_id,
        run.id,
        run.revision,
        0,
        "submit_input",
        request_digest,
        run.updated_at,
        command,
        AuditMetadata("source", "deep-runtime"),
    )
    resolution = InteractionResolution(interaction_id, request_digest, reference, submission)
    intent = ToolIntent(
        tool_call_id,
        operation_id,
        run.id,
        "search_articles",
        request_digest,
        interaction_id,
        run.updated_at,
        AuditMetadata("source", "deep-runtime"),
    )
    identity = RequestIdentity(
        RequestId(f"request-{digest[:16]}"),
        CorrelationId(f"correlation-{digest[16:32]}"),
        run.id,
        tool_call_id,
    )
    fallback = ToolOperation(
        tool_call_id,
        operation_id,
        run.id,
        intent.tool_name,
        request_digest,
        ToolOutcomeStatus.FAILED,
        None,
        ResultMetadata("classification", "unstarted"),
        "unstarted",
        run.updated_at,
        intent.audit_metadata,
    )
    return ProposalEnvelope(
        f"approval-{digest[:32]}",
        intent,
        resolution,
        SearchArticles(identity, run_input.initial_message, 10),
        fallback,
    )
