"""HTTP 与 gRPC 共用的薄应用门面。."""

from collections.abc import AsyncGenerator
from typing import Protocol, final

from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandApplyResult,
    CommandDataIntegrity,
    CommandRejected,
    CommandRunNotFound,
    CommandSubmission,
    IdempotencyConflict,
    UnsupportedCommand,
)
from scyg_agent.domain.ports.event_store import (
    CursorTooOld,
    EventCursor,
    FutureCursor,
    ReplayPage,
    ReplayResult,
    StoredEvent,
)
from scyg_agent.domain.ports.interaction_store import (
    AlreadyResolved,
    InteractionDataIntegrity,
    InteractionIdempotencyConflict,
    InteractionNotFound,
    InteractionResolution,
    InteractionResolveResult,
)
from scyg_agent.domain.runs import Run, RunId, RunStatus
from scyg_agent.domain.runs.cancellation import (
    CancellationQueued,
    CancellationRequested,
    CancellationTerminal,
)
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.domain.runs.repository import (
    CancellationRequest,
    CancellationResult,
    CreateConflict,
    Created,
    CreateResult,
    CreateRunRequest,
    DataIntegrityError,
    DuplicateOperation,
    GetResult,
    NotFound,
)

from .models import (
    CancelRequest,
    CreateRunInput,
    FacadeConflict,
    FacadeInternal,
    FacadeNotFound,
    FacadePrecondition,
    FacadeSuccess,
    FollowOpened,
    OwnedCommand,
    OwnerContext,
    ReplaySuccess,
    SnapshotSuccess,
    SubmitInputRequest,
)


class RunApplications(Protocol):
    """声明门面使用的最小 Run 应用能力。."""

    async def create(self, request: CreateRunRequest) -> CreateResult:
        """创建或重放 Run。."""
        ...  # pragma: no cover - protocol declaration.

    async def get(self, run_id: RunId) -> GetResult:
        """读取 Run。."""
        ...  # pragma: no cover - protocol declaration.

    async def request_cancellation(self, request: CancellationRequest) -> CancellationResult:
        """持久化取消。."""
        ...  # pragma: no cover - protocol declaration.


class EventApplications(Protocol):
    """声明门面使用的最小事件应用能力。."""

    async def replay(self, run_id: RunId, cursor: EventCursor, limit: int) -> ReplayResult:
        """回放事件。."""
        ...  # pragma: no cover - protocol declaration.

    def subscribe(self, run_id: RunId, cursor: EventCursor) -> AsyncGenerator[StoredEvent, None]:
        """跟随事件。."""
        ...  # pragma: no cover - protocol declaration.


class CommandApplications(Protocol):
    """声明门面使用的最小命令应用能力。."""

    async def apply(self, request: CommandSubmission) -> CommandApplyResult:
        """应用命令。."""
        ...  # pragma: no cover - protocol declaration.


class InteractionApplications(Protocol):
    """声明门面使用的最小交互应用能力。."""

    async def resolve(self, request: InteractionResolution) -> InteractionResolveResult:
        """解析交互。."""
        ...  # pragma: no cover - protocol declaration.


type ReadFailure = FacadeNotFound | FacadeInternal
type MutationResult = (
    FacadeSuccess | FacadeNotFound | FacadeConflict | FacadePrecondition | FacadeInternal
)


@final
class ApplicationFacade:
    """组合现有应用真值并实施统一的 Run 所有权绑定。."""

    def __init__(
        self,
        runs: RunApplications,
        events: EventApplications,
        commands: CommandApplications,
        interactions: InteractionApplications,
    ) -> None:
        """绑定四个现有应用真值端口。."""
        self._runs, self._events = runs, events
        self._commands, self._interactions = commands, interactions

    async def create_run(
        self, request: CreateRunInput
    ) -> FacadeSuccess | FacadeConflict | FacadeInternal:
        """通过现有创建事务持久化 Run 与初始输入。."""
        run = Run(
            request.run_id,
            request.owner.user_id,
            request.task_type,
            request.runtime,
            1,
            RunStatus.PENDING,
            request.requested_at,
            request.requested_at,
            0,
            None,
            None,
        )
        result = await self._runs.create(
            CreateRunRequest(
                run,
                request.operation_id,
                request.requested_at,
                RunInput(
                    request.initial_message,
                    request.article_id,
                    request.capability,
                    request.recipe_id,
                    request.recipe_version,
                    request.input_schema_version,
                    request.input_payload,
                    request.input_digest,
                    request.locale,
                ),
            )
        )
        match result:  # noqa: RUF100  # noqa: MATCH_OK - 闭合结果已完整映射。
            case Created(run=created):
                return FacadeSuccess(created)
            case DuplicateOperation(run=duplicate):
                return FacadeSuccess(duplicate, replayed=True)
            case CreateConflict():
                return FacadeConflict()
            case DataIntegrityError():
                return FacadeInternal()

    async def get_snapshot(
        self, owner: OwnerContext, run_id: RunId
    ) -> SnapshotSuccess | ReadFailure:
        """返回最新合法 Run 与 T11 稳定游标。."""
        bound = await self._owned_run(owner, run_id)
        if isinstance(bound, FacadeNotFound | FacadeInternal):
            return bound
        replay = await self._events.replay(run_id, EventCursor(0), 1)
        match replay:  # noqa: RUF100  # noqa: MATCH_OK - 闭合结果已完整映射。
            case ReplayPage(latest=latest):
                return SnapshotSuccess(bound, latest)
            case FutureCursor() | CursorTooOld():
                return FacadeInternal()

    async def replay_events(
        self, owner: OwnerContext, run_id: RunId, cursor: EventCursor, limit: int
    ) -> ReplaySuccess | FacadeConflict | ReadFailure:
        """校验所有权后直接返回 T11 回放事实。."""
        bound = await self._owned_run(owner, run_id)
        if isinstance(bound, FacadeNotFound | FacadeInternal):
            return bound
        result = await self._events.replay(run_id, cursor, limit)
        match result:  # noqa: RUF100  # noqa: MATCH_OK - 闭合结果已完整映射。
            case ReplayPage() as page:
                return ReplaySuccess(page)
            case FutureCursor() | CursorTooOld():
                return FacadeConflict("事件游标不在可回放范围内")

    async def follow(
        self, owner: OwnerContext, run_id: RunId, cursor: EventCursor
    ) -> FollowOpened | ReadFailure:
        """暴露 T11 可关闭流且不持有网络缓冲。."""
        bound = await self._owned_run(owner, run_id)
        if isinstance(bound, FacadeNotFound | FacadeInternal):
            return bound
        return FollowOpened(self._events.subscribe(run_id, cursor))

    async def submit_input(self, request: SubmitInputRequest) -> MutationResult:
        """通过 T12 交互赢家事务提交输入。."""
        run_id = request.resolution.command.run_id
        bound = await self._owned_run(request.owner, run_id)
        if isinstance(bound, FacadeNotFound | FacadeInternal):
            return bound
        return self._command_outcome(await self._interactions.resolve(request.resolution), run_id)

    async def submit_command(self, request: OwnedCommand) -> MutationResult:
        """通过 T12 命令事务提交公共命令。."""
        run_id = request.submission.run_id
        bound = await self._owned_run(request.owner, run_id)
        if isinstance(bound, FacadeNotFound | FacadeInternal):
            return bound
        return self._command_outcome(await self._commands.apply(request.submission), run_id)

    async def cancel(self, request: CancelRequest) -> MutationResult:
        """通过 T19 持久化取消真值并保留幂等重放标记。."""
        bound = await self._owned_run(request.owner, request.run_id)
        if isinstance(bound, FacadeNotFound | FacadeInternal):
            return bound
        result = await self._runs.request_cancellation(
            CancellationRequest(request.run_id, request.requested_at)
        )
        match result:  # noqa: RUF100  # noqa: MATCH_OK - 闭合结果已完整映射。
            case CancellationRequested(replayed=replayed) | CancellationQueued(replayed=replayed):
                return FacadeSuccess(result, replayed)
            case CancellationTerminal():
                return FacadeSuccess(result, replayed=True)
            case NotFound():
                return FacadeNotFound(request.run_id)
            case DataIntegrityError():
                return FacadeInternal()

    async def _owned_run(self, owner: OwnerContext, run_id: RunId) -> Run | ReadFailure:
        result = await self._runs.get(run_id)
        match result:  # noqa: RUF100  # noqa: MATCH_OK - 闭合结果已完整映射。
            case Run(owner_user_id=actual) if actual == owner.user_id:
                return result
            case Run() | NotFound():
                return FacadeNotFound(run_id)
            case DataIntegrityError():
                return FacadeInternal()

    @staticmethod
    def _command_outcome(
        result: CommandApplyResult | InteractionResolveResult, run_id: RunId
    ) -> MutationResult:
        match result:  # noqa: RUF100  # noqa: MATCH_OK - 闭合结果已完整映射。
            case CommandApplied(replayed=replayed):
                return FacadeSuccess(result, replayed)
            case AlreadyResolved():
                return FacadeSuccess(result, replayed=True)
            case CommandRejected() | UnsupportedCommand():
                return FacadePrecondition()
            case IdempotencyConflict() | InteractionIdempotencyConflict():
                return FacadeConflict()
            case CommandRunNotFound() | InteractionNotFound():
                return FacadeNotFound(run_id)
            case CommandDataIntegrity() | InteractionDataIntegrity():
                return FacadeInternal()
