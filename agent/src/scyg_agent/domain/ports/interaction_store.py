"""一次性交互创建和解析的纯领域端口。."""

from dataclasses import dataclass
from datetime import datetime
from typing import Protocol

from scyg_agent.domain.runs import InteractionId, RunId, SubmitInput

from .command_store import CommandApplyResult, CommandSubmission
from .idempotency import RequestDigest, ResultReference, require_utc


@dataclass(frozen=True, slots=True)
class InteractionRequest:
    """创建绑定到一个 Run 的待处理交互。."""

    interaction_id: InteractionId
    run_id: RunId
    kind: str
    requested_at: datetime

    def __post_init__(self) -> None:
        """验证确定性请求时间。."""
        require_utc(self.requested_at, "requested_at")


@dataclass(frozen=True, slots=True)
class InteractionResolution:
    """携带一次 SubmitInput 解析及其响应摘要。."""

    interaction_id: InteractionId
    response_digest: RequestDigest
    result_reference: ResultReference
    command: CommandSubmission

    def __post_init__(self) -> None:
        """要求解析命令精确绑定当前交互。."""
        if (
            not isinstance(self.command.command, SubmitInput)
            or self.command.command.interaction_id != self.interaction_id
        ):
            raise InvalidInteractionResolutionError


@dataclass(frozen=True, slots=True)
class InvalidInteractionResolutionError(ValueError):
    """拒绝不匹配的交互解析命令。."""


@dataclass(frozen=True, slots=True)
class InteractionPending:
    """确认待处理交互已创建或已存在。."""

    interaction_id: InteractionId
    replayed: bool


@dataclass(frozen=True, slots=True)
class AlreadyResolved:
    """返回并发赢家持久化的原始结果。."""

    interaction_id: InteractionId
    result_reference: ResultReference


@dataclass(frozen=True, slots=True)
class InteractionConflict:
    """报告交互身份或当前 Run 待处理身份冲突。."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class InteractionNotFound:
    """报告未知交互。."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class InteractionIdempotencyConflict:
    """报告交互身份被不同创建或解析语义复用。."""

    interaction_id: InteractionId


@dataclass(frozen=True, slots=True)
class InteractionDataIntegrity:
    """报告无法安全恢复的持久化交互数据。."""

    interaction_id: InteractionId


type InteractionCreateResult = (
    InteractionPending
    | InteractionConflict
    | InteractionIdempotencyConflict
    | InteractionDataIntegrity
)
type InteractionResolveResult = (
    CommandApplyResult
    | AlreadyResolved
    | InteractionNotFound
    | InteractionIdempotencyConflict
    | InteractionDataIntegrity
)


class InteractionStore(Protocol):
    """持久化并一次性解析用户交互。."""

    async def request(self, request: InteractionRequest) -> InteractionCreateResult:
        """创建唯一待处理交互。."""
        ...  # pragma: no cover

    async def resolve(self, request: InteractionResolution) -> InteractionResolveResult:
        """选择一个解析赢家并重放其结果。."""
        ...  # pragma: no cover
