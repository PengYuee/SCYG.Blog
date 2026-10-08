"""工具操作幂等结果存储的纯领域端口。."""

from dataclasses import dataclass
from datetime import datetime, timedelta
from enum import StrEnum
from typing import Protocol

from scyg_agent.domain.runs import InteractionId, OperationId, RunId, ToolCallId
from scyg_agent.domain.runs.repository import LeaseToken

from .idempotency import AuditMetadata, RequestDigest, ResultMetadata, ResultReference, require_utc


class ToolOutcomeStatus(StrEnum):
    """限定工具操作的终态。."""

    SUCCEEDED = "succeeded"
    FAILED = "failed"
    EXTERNAL_OUTCOME_UNKNOWN = "external_outcome_unknown"


@dataclass(frozen=True, slots=True)
class ToolIntent:
    """保存外部调用前即可持久化的完整语义身份。."""

    tool_call_id: ToolCallId
    operation_id: OperationId
    run_id: RunId
    tool_name: str
    request_digest: RequestDigest
    approval_interaction_id: InteractionId
    prepared_at: datetime
    audit_metadata: AuditMetadata

    def __post_init__(self) -> None:
        """要求准备时间为确定性 UTC 时间。."""
        require_utc(self.prepared_at, "prepared_at")


@dataclass(frozen=True, slots=True)
class ClaimRequest:
    """携带一次短事务工具租约竞争。."""

    operation_id: OperationId
    token: LeaseToken
    now: datetime
    lease_duration: timedelta

    def __post_init__(self) -> None:
        """拒绝非 UTC 时间和非正租期。."""
        require_utc(self.now, "now")
        if self.lease_duration <= timedelta(0):
            raise InvalidToolOperationError


@dataclass(frozen=True, slots=True)
class ToolFence:
    """绑定完成操作必须持有的 token 和单调版本。."""

    operation_id: OperationId
    token: LeaseToken
    version: int
    occurred_at: datetime

    def __post_init__(self) -> None:
        """拒绝无效版本和非 UTC 时间。."""
        require_utc(self.occurred_at, "occurred_at")
        if self.version <= 0:
            raise InvalidToolOperationError


@dataclass(frozen=True, slots=True)
class ToolOperation:
    """保存工具操作身份和清洗后的原始终态。."""

    tool_call_id: ToolCallId
    operation_id: OperationId
    run_id: RunId
    tool_name: str
    request_digest: RequestDigest
    status: ToolOutcomeStatus
    result_reference: ResultReference | None
    metadata: ResultMetadata
    error_code: str | None
    occurred_at: datetime
    audit_metadata: AuditMetadata

    def __post_init__(self) -> None:
        """保证成功和失败字段互斥且时间确定。."""
        require_utc(self.occurred_at, "occurred_at")
        if (self.status is ToolOutcomeStatus.SUCCEEDED) != (self.result_reference is not None):
            raise InvalidToolOperationError
        if (self.status is not ToolOutcomeStatus.SUCCEEDED) != (self.error_code is not None):
            raise InvalidToolOperationError


@dataclass(frozen=True, slots=True)
class InvalidToolOperationError(ValueError):
    """拒绝不一致的工具终态。."""


@dataclass(frozen=True, slots=True)
class ToolOperationStored:
    """返回首次写入或重试获得的原始工具结果。."""

    operation: ToolOperation
    replayed: bool


@dataclass(frozen=True, slots=True)
class ToolIdempotencyConflict:
    """报告操作身份被不同请求重用。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class ToolRunNotFound:
    """报告工具操作关联的 Run 不存在。."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class ToolOperationNotFound:
    """报告未知工具操作身份。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class ToolDataIntegrity:
    """报告无法安全恢复的持久化工具操作。."""

    operation_id: OperationId | None


@dataclass(frozen=True, slots=True)
class FirstClaim:
    """返回首次获得的执行围栏。."""

    fence: ToolFence


@dataclass(frozen=True, slots=True)
class ExistingInFlight:
    """报告一个仍有效的执行租约。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class TerminalReplay:
    """返回已持久化终态且不追加副作用。."""

    operation: ToolOperation


@dataclass(frozen=True, slots=True)
class SemanticIdentityConflict:
    """报告 operation_id 对应的意图语义不同。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class DecisionConflict:
    """报告意图绑定的审批解析身份不同。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class StaleLeaseRecovered:
    """返回仅在 RPC 尚未开始时替换的围栏。."""

    fence: ToolFence


@dataclass(frozen=True, slots=True)
class ClaimLost:
    """报告 token 或版本不再拥有执行权。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class ExternalOutcomeUnknown:
    """报告 RPC 已开始但无法安全确定外部结果。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class ToolIntentPrepared:
    """确认审批解析和待执行意图已在同一事务提交。."""

    operation_id: OperationId
    replayed: bool


type ToolStoreResult = (
    ToolOperationStored | ToolIdempotencyConflict | ToolRunNotFound | ToolDataIntegrity
)
type ToolGetResult = ToolOperationStored | ToolOperationNotFound | ToolDataIntegrity
type ToolClaimResult = (
    FirstClaim
    | ExistingInFlight
    | TerminalReplay
    | SemanticIdentityConflict
    | DecisionConflict
    | StaleLeaseRecovered
    | ExternalOutcomeUnknown
    | ToolOperationNotFound
    | ToolDataIntegrity
)
type ToolFenceResult = ClaimLost | ExternalOutcomeUnknown | TerminalReplay | ToolDataIntegrity


class ToolOperationStore(Protocol):
    """原子保存或重放一个工具操作终态。."""

    async def apply(self, request: ToolOperation) -> ToolStoreResult:
        """保存首次终态, 重复调用返回原始终态。."""
        ...  # pragma: no cover

    async def get(self, operation_id: OperationId) -> ToolGetResult:
        """读取已保存的原始工具终态。."""
        ...  # pragma: no cover

    async def claim(self, request: ClaimRequest) -> ToolClaimResult:
        """竞争或保守恢复一次工具调用租约。."""
        ...  # pragma: no cover

    async def complete(self, fence: ToolFence, outcome: ToolOperation) -> ToolFenceResult:
        """使用围栏写入终态和唯一审计事实。."""
        ...  # pragma: no cover
