"""Deep Runtime 的封闭输入、审批和结果类型."""

from dataclasses import dataclass
from enum import StrEnum
from typing import Final, override

from scyg_agent.adapters.blog_grpc.contracts import BlogCommand
from scyg_agent.domain.ports.interaction_store import InteractionResolution
from scyg_agent.domain.ports.tool_store import ToolIntent, ToolOperation
from scyg_agent.domain.runs import RunId, TaskType

GRAPH_NAME: Final = "scyg-deep-runtime"
GRAPH_VERSION: Final = "deep.v1"


class ApprovalDecision(StrEnum):
    """限定允许持久化的审批决定."""

    APPROVE = "approve"
    REJECT = "reject"


@dataclass(frozen=True, slots=True)
class ApprovalReply:
    """绑定一次恢复必须匹配的审批令牌和决定."""

    token: str
    decision: ApprovalDecision


@dataclass(frozen=True, slots=True)
class ProposalEnvelope:
    """保存审批前已确定的调用和持久化身份."""

    approval_token: str
    intent: ToolIntent
    resolution: InteractionResolution
    command: BlogCommand
    fallback_outcome: ToolOperation


@dataclass(frozen=True, slots=True)
class DeepGraphInput:
    """定义一个 T14 Deep 任务的确定性图输入."""

    task_type: TaskType
    proposal: ProposalEnvelope


@dataclass(frozen=True, slots=True)
class ApprovalRequired:
    """表示检查点已停在工具 I/O 之前."""

    run_id: RunId
    approval_token: str


@dataclass(frozen=True, slots=True)
class Rejected:
    """表示审批已拒绝且工具未执行."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class ToolFinished:
    """表示工具终态已由 T12 围栏持久化."""

    run_id: RunId
    outcome: ToolOperation


@dataclass(frozen=True, slots=True)
class ExternalOutcomeUnknownResult:
    """表示 RPC 已开始但外部结果不可安全确定."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class ExecutionInFlight:
    """表示另一有效围栏仍拥有执行权."""

    run_id: RunId


type DeepResult = (
    ApprovalRequired | Rejected | ToolFinished | ExternalOutcomeUnknownResult | ExecutionInFlight
)


class DeepFailureKind(StrEnum):
    """限定不携带秘密的运行时失败类别."""

    INVALID_INPUT = "invalid_input"
    INVALID_CHECKPOINT = "invalid_checkpoint"
    MISSING_THREAD = "missing_thread"
    IDENTITY_MISMATCH = "identity_mismatch"
    APPROVAL_CONFLICT = "approval_conflict"
    PERSISTENCE_FAILURE = "persistence_failure"


class DeepRuntimeError(RuntimeError):
    """向调用方暴露稳定且无值的失败类别."""

    __slots__: tuple[str, ...] = ("kind",)
    kind: DeepFailureKind

    def __init__(self, kind: DeepFailureKind) -> None:
        """保存封闭失败类别且不接收敏感上下文."""
        super().__init__()
        self.kind = kind

    @override
    def __str__(self) -> str:
        return f"Deep Runtime 失败: {self.kind.value}"
