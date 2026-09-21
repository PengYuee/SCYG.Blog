"""共享应用门面的冻结输入与闭合输出。."""

from collections.abc import AsyncGenerator
from dataclasses import dataclass
from datetime import datetime
from typing import override

from scyg_agent.domain.ports.command_store import CommandApplied, CommandRejected, CommandSubmission
from scyg_agent.domain.ports.event_store import EventCursor, ReplayPage, StoredEvent
from scyg_agent.domain.ports.interaction_store import AlreadyResolved, InteractionResolution
from scyg_agent.domain.runs import OperationId, Run, RunId, RuntimeSelection, TaskType, UserId
from scyg_agent.domain.runs.cancellation import (
    CancellationQueued,
    CancellationRequested,
    CancellationTerminal,
)


@dataclass(frozen=True, slots=True)
class OwnerContext:
    """携带已经认证且精确类型化的 Blog 用户主体。."""

    user_id: UserId

    def __post_init__(self) -> None:
        """拒绝非精确用户身份。."""
        if type(self.user_id) is not UserId:
            field = "owner_user_id"
            raise InvalidFacadeInputError(field)


@dataclass(frozen=True, slots=True)
class CreateRunInput:
    """携带创建 Run 所需的完整请求与 capability 快照。."""

    owner: OwnerContext
    operation_id: OperationId
    run_id: RunId
    task_type: TaskType
    runtime: RuntimeSelection
    article_id: str
    initial_message: str
    requested_at: datetime
    capability: str | None = None
    recipe_id: str | None = None
    recipe_version: str | None = None
    input_schema_version: str | None = None
    input_payload: dict[str, object] | None = None
    input_digest: str | None = None
    locale: str | None = None

    def __post_init__(self) -> None:
        """拒绝非精确创建请求类型。."""
        exact = (
            type(self.owner) is OwnerContext
            and type(self.operation_id) is OperationId
            and type(self.run_id) is RunId
            and type(self.task_type) is TaskType
            and type(self.runtime) is RuntimeSelection
        )
        if not exact:
            field = "create_run"
            raise InvalidFacadeInputError(field)


@dataclass(frozen=True, slots=True)
class OwnedCommand:
    """显式绑定命令提交与调用主体。."""

    owner: OwnerContext
    submission: CommandSubmission


@dataclass(frozen=True, slots=True)
class SubmitInputRequest:
    """显式绑定一次性交互解析与调用主体。."""

    owner: OwnerContext
    resolution: InteractionResolution


@dataclass(frozen=True, slots=True)
class CancelRequest:
    """显式绑定取消目标、主体和确定性时间。."""

    owner: OwnerContext
    run_id: RunId
    requested_at: datetime


type SuccessValue = (
    Run
    | CommandApplied
    | CommandRejected
    | AlreadyResolved
    | CancellationRequested
    | CancellationQueued
    | CancellationTerminal
)


@dataclass(frozen=True, slots=True)
class FacadeSuccess:
    """返回委托操作的清洗成功事实。."""

    value: SuccessValue
    replayed: bool = False


@dataclass(frozen=True, slots=True)
class SnapshotSuccess:
    """返回不含租约和检查点的稳定 Run 快照。."""

    run: Run
    cursor: EventCursor


@dataclass(frozen=True, slots=True)
class ReplaySuccess:
    """返回 T11 持久化事件页。."""

    page: ReplayPage


@dataclass(frozen=True, slots=True)
class FollowOpened:
    """返回由调用者消费并关闭的无缓冲持久化事件流。."""

    events: AsyncGenerator[StoredEvent, None]


@dataclass(frozen=True, slots=True)
class FacadeNotFound:
    """统一不存在与所有权不匹配并避免泄露 Run。."""

    run_id: RunId
    message: str = "未找到 Run"


@dataclass(frozen=True, slots=True)
class FacadeValidation:
    """返回可安全映射的请求校验失败。."""

    message: str = "请求参数无效"


@dataclass(frozen=True, slots=True)
class FacadeConflict:
    """返回幂等身份或游标冲突。."""

    message: str = "请求与已持久化事实冲突"


@dataclass(frozen=True, slots=True)
class FacadePrecondition:
    """返回当前 Run 状态不允许操作。."""

    message: str = "Run 当前状态不允许此操作"


@dataclass(frozen=True, slots=True)
class FacadeCancelled:
    """返回已取消的应用调用边界。."""

    message: str = "请求已取消"


@dataclass(frozen=True, slots=True)
class FacadeInternal:
    """隐藏持久化完整性细节。."""

    message: str = "应用内部状态异常"


@dataclass(frozen=True, slots=True)
class InvalidFacadeInputError(ValueError):
    """拒绝非精确类型的门面输入。."""

    field: str

    @override
    def __str__(self) -> str:
        """返回不回显输入值的中文诊断。."""
        return f"应用请求字段无效: {self.field}"
