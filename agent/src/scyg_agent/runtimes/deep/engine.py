"""Deep Runtime 的 T12/T16 围栏执行引擎."""

from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Protocol, runtime_checkable

from scyg_agent.adapters.blog_grpc.contracts import BlogCommand, BlogResult
from scyg_agent.domain.ports.interaction_store import (
    AlreadyResolved,
    InteractionResolution,
    InteractionResolveResult,
)
from scyg_agent.domain.ports.tool_store import (
    ClaimLost,
    ClaimRequest,
    DecisionConflict,
    ExistingInFlight,
    ExternalOutcomeUnknown,
    FirstClaim,
    SemanticIdentityConflict,
    StaleLeaseRecovered,
    TerminalReplay,
    ToolClaimResult,
    ToolDataIntegrity,
    ToolFence,
    ToolFenceResult,
    ToolIntent,
    ToolIntentPrepared,
    ToolOperation,
    ToolOperationNotFound,
)
from scyg_agent.domain.runs import RunId
from scyg_agent.domain.runs.repository import LeaseToken
from scyg_agent.runtimes.profiles import RuntimeProfile

from .models import (
    DeepFailureKind,
    DeepRuntimeError,
    ExecutionInFlight,
    ExternalOutcomeUnknownResult,
    ProposalEnvelope,
    ToolFinished,
)


@runtime_checkable
class ApprovalPersistence(Protocol):
    """组合 T12 审批解析与工具围栏能力."""

    async def resolve(self, request: InteractionResolution) -> InteractionResolveResult:
        """持久化拒绝或重放既有审批决定."""
        ...

    async def resolve_and_prepare(
        self, request: InteractionResolution, intent: ToolIntent
    ) -> (
        ToolIntentPrepared | InteractionResolveResult | SemanticIdentityConflict | DecisionConflict
    ):
        """原子持久化批准决定和 pending 工具意图."""
        ...

    async def claim(self, request: ClaimRequest) -> ToolClaimResult:
        """领取或恢复 pending 工具意图."""
        ...

    async def mark_rpc_started(self, fence: ToolFence) -> ToolFenceResult | FirstClaim:
        """在外部 I/O 前持久化 RPC 提交点."""
        ...

    async def complete(self, fence: ToolFence, outcome: ToolOperation) -> ToolFenceResult:
        """以当前围栏保存唯一清洗终态."""
        ...


class BlogClient(Protocol):
    """限定唯一允许的 T16 调用表面."""

    async def invoke(self, tool_name: str, version: str, command: BlogCommand) -> BlogResult:
        """通过 T16 静态允许列表执行命令."""
        ...


class OutcomeFactory(Protocol):
    """把 T16 封闭结果映射为清洗后的 T12 终态."""

    def from_blog_result(self, proposal: ProposalEnvelope, result: BlogResult) -> ToolOperation:
        """生成不含提供方正文的持久终态."""
        ...


@dataclass(frozen=True, slots=True)
class DeepDependencies:
    """保存执行所需的冻结端口和确定性工厂."""

    profile: RuntimeProfile
    persistence: ApprovalPersistence
    client: BlogClient
    outcomes: OutcomeFactory
    token_factory: Callable[[], LeaseToken]
    clock: Callable[[], datetime]
    lease_duration: timedelta
    is_rejection_persisted: Callable[[InteractionResolveResult], bool]


async def approve_and_execute(
    proposal: ProposalEnvelope, profile: RuntimeProfile, dependencies: DeepDependencies
) -> ToolFinished | ExternalOutcomeUnknownResult | ExecutionInFlight:
    """提交意图, 领取围栏并在 RPC 提交点后保守完成."""
    prepared = await dependencies.persistence.resolve_and_prepare(
        proposal.resolution, proposal.intent
    )
    if not isinstance(prepared, (ToolIntentPrepared, AlreadyResolved)):
        raise DeepRuntimeError(DeepFailureKind.APPROVAL_CONFLICT)
    claimed = await dependencies.persistence.claim(
        ClaimRequest(
            proposal.intent.operation_id,
            dependencies.token_factory(),
            dependencies.clock(),
            dependencies.lease_duration,
        )
    )
    match claimed:  # noqa: RUF100  # noqa: MATCH_OK - ToolClaimResult 的全部闭集分支已映射。
        case FirstClaim(fence=fence) | StaleLeaseRecovered(fence=fence):
            marked = await dependencies.persistence.mark_rpc_started(fence)
            if not isinstance(marked, FirstClaim):
                return _fence_result(proposal.intent.run_id, marked)
            result = await dependencies.client.invoke(
                proposal.intent.tool_name, profile.selection.version, proposal.command
            )
            outcome = dependencies.outcomes.from_blog_result(proposal, result)
            completed = await dependencies.persistence.complete(fence, outcome)
            return _fence_result(proposal.intent.run_id, completed)
        case ExistingInFlight():
            return ExecutionInFlight(proposal.intent.run_id)
        case TerminalReplay(operation=operation):
            return ToolFinished(proposal.intent.run_id, operation)
        case ExternalOutcomeUnknown():
            return ExternalOutcomeUnknownResult(proposal.intent.run_id)
        case (
            SemanticIdentityConflict()
            | DecisionConflict()
            | ToolOperationNotFound()
            | ToolDataIntegrity()
        ):
            raise DeepRuntimeError(DeepFailureKind.PERSISTENCE_FAILURE)


def _fence_result(
    run_id: RunId, result: ToolFenceResult
) -> ToolFinished | ExternalOutcomeUnknownResult:
    """将围栏结果收窄为运行时终态."""
    match result:  # noqa: RUF100  # noqa: MATCH_OK - ToolFenceResult 的全部闭集分支已映射。
        case TerminalReplay(operation=operation):
            return ToolFinished(run_id, operation)
        case ExternalOutcomeUnknown():
            return ExternalOutcomeUnknownResult(run_id)
        case ClaimLost() | ToolDataIntegrity():
            raise DeepRuntimeError(DeepFailureKind.PERSISTENCE_FAILURE)
