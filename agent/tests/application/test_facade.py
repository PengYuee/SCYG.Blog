"""共享应用门面的聚焦行为测试。"""

from collections.abc import AsyncGenerator
from dataclasses import dataclass
from datetime import UTC, datetime

import pytest

from scyg_agent.application import (
    ApplicationFacade,
    CancelRequest,
    CreateRunInput,
    FacadeConflict,
    FacadeNotFound,
    FacadeSuccess,
    FollowOpened,
    OwnedCommand,
    OwnerContext,
    SnapshotSuccess,
    SubmitInputRequest,
)
from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandApplyResult,
    CommandSubmission,
)
from scyg_agent.domain.ports.event_store import (
    EventCursor,
    FutureCursor,
    ReplayPage,
    ReplayResult,
    StoredEvent,
)
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest, ResultReference
from scyg_agent.domain.ports.interaction_store import (
    AlreadyResolved,
    InteractionCreateResult,
    InteractionRequest,
    InteractionResolution,
    InteractionResolveResult,
)
from scyg_agent.domain.runs import (
    CancelRun,
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    Run,
    RunCancelled,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    SubmitInput,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.cancellation import CancellationQueued
from scyg_agent.domain.runs.repository import (
    CancellationRequest,
    CancellationResult,
    CreateConflict,
    Created,
    CreateResult,
    CreateRunRequest,
    DuplicateOperation,
    GetResult,
    NotFound,
)

NOW = datetime(2026, 7, 12, 13, tzinfo=UTC)
RUN_ID, OWNER, OTHER = RunId("run_t20facade0"), UserId("user-t20"), UserId("other-t20")
INTERACTION_ID = InteractionId("int_t20facade0")


@dataclass(slots=True)  # noqa: RUF100  # noqa: MUTABLE_OK
class FakeRuns:
    """保存测试 Run 并模拟现有仓储结果。"""

    run: Run | None = None
    operation: OperationId | None = None
    cancelled: bool = False

    async def create(self, request: CreateRunRequest) -> CreateResult:
        if self.operation is None:
            self.operation, self.run = request.operation_id, request.run
            return Created(request.run)
        if self.operation == request.operation_id and self.run == request.run:
            return DuplicateOperation(request.run)
        return CreateConflict(request.operation_id)

    async def get(self, run_id: RunId) -> GetResult:
        return NotFound(run_id) if self.run is None or self.run.id != run_id else self.run

    async def request_cancellation(self, request: CancellationRequest) -> CancellationResult:
        self.cancelled = True
        return CancellationQueued(request.run_id, request.requested_at, replayed=False)


@dataclass(slots=True)  # noqa: RUF100  # noqa: MUTABLE_OK
class FakeEvents:
    """提供确定性回放页与可关闭跟随流。"""

    latest: EventCursor
    closed: bool = False

    async def replay(self, run_id: RunId, cursor: EventCursor, limit: int) -> ReplayResult:
        del run_id, limit
        return (
            FutureCursor(cursor, self.latest)
            if cursor > self.latest
            else ReplayPage((), self.latest, EventCursor(0))
        )

    async def subscribe(
        self, run_id: RunId, cursor: EventCursor
    ) -> AsyncGenerator[StoredEvent, None]:
        del run_id, cursor
        try:
            yield StoredEvent(
                EventCursor(3),
                RunCancelled(
                    EventId("evt_t20follow0"),
                    CommandId("cmd_t20follow0"),
                    NOW,
                    RUN_ID,
                    2,
                ),
            )
        finally:
            self.closed = True


@dataclass(slots=True)  # noqa: RUF100  # noqa: MUTABLE_OK
class FakeCommands:
    """记录命令并返回现有 T12 成功结果。"""

    calls: int = 0
    result: CommandApplyResult | None = None

    async def apply(self, request: CommandSubmission) -> CommandApplyResult:
        del request
        self.calls += 1
        if self.result is not None:
            return self.result
        return CommandApplied(
            make_run(RunStatus.PENDING_RESUME, 2),
            (),
            ResultReference("run:t20:revision:2"),
            self.calls > 1,
        )


@dataclass(slots=True)  # noqa: RUF100  # noqa: MUTABLE_OK
class FakeInteractions:
    """让首次输入成功且重复输入返回原结果。"""

    calls: int = 0

    async def request(self, request: InteractionRequest) -> InteractionCreateResult:
        raise AssertionError(request)

    async def resolve(self, request: InteractionResolution) -> InteractionResolveResult:
        self.calls += 1
        if self.calls > 1:
            return AlreadyResolved(request.interaction_id, request.result_reference)
        return await FakeCommands().apply(request.command)


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


def make_run(status: RunStatus = RunStatus.PENDING, revision: int = 1) -> Run:
    interaction = INTERACTION_ID if status is RunStatus.WAITING_INPUT else None
    return Run(
        RUN_ID,
        OWNER,
        TaskType.SUMMARY,
        RuntimeSelection(RuntimeKind.SIMPLE, "v1"),
        revision,
        status,
        NOW,
        NOW,
        0,
        None,
        interaction,
    )


def facade(
    run: Run | None = None,
    latest: int = 0,
    commands: FakeCommands | None = None,
) -> tuple[ApplicationFacade, FakeRuns, FakeEvents]:
    runs, events = FakeRuns(run=run), FakeEvents(EventCursor(latest))
    return (
        ApplicationFacade(runs, events, commands or FakeCommands(), FakeInteractions()),
        runs,
        events,
    )


def create_input(run_id: RunId = RUN_ID, message: str = "初始消息") -> CreateRunInput:
    return CreateRunInput(
        OwnerContext(OWNER),
        OperationId("t20:create"),
        run_id,
        TaskType.SUMMARY,
        RuntimeSelection(RuntimeKind.SIMPLE, "v1"),
        "article-20",
        message,
        NOW,
    )


def submission(command: SubmitInput | CancelRun) -> CommandSubmission:
    return CommandSubmission(
        command.command_id,
        RUN_ID,
        command.expected_revision,
        0,
        "external",
        RequestDigest.parse("a" * 64),
        NOW,
        command,
        AuditMetadata("source", "t20"),
    )


@pytest.mark.anyio
async def test_duplicate_create_and_conflict_are_typed() -> None:
    app, _, _ = facade()
    first = await app.create_run(create_input())
    duplicate = await app.create_run(create_input())
    conflict = await app.create_run(create_input(RunId("run_t20other00"), "不同消息"))
    assert isinstance(first, FacadeSuccess)
    assert isinstance(duplicate, FacadeSuccess)
    assert duplicate.replayed
    assert isinstance(conflict, FacadeConflict)


@pytest.mark.anyio
async def test_owner_mismatch_and_cursor_boundary_are_hidden_and_typed() -> None:
    app, _, _ = facade(make_run(), 3)
    assert isinstance(await app.get_snapshot(OwnerContext(OTHER), RUN_ID), FacadeNotFound)
    assert isinstance(
        await app.replay_events(OwnerContext(OTHER), RUN_ID, EventCursor(0), 10), FacadeNotFound
    )
    assert isinstance(
        await app.replay_events(OwnerContext(OWNER), RUN_ID, EventCursor(4), 10), FacadeConflict
    )
    current = await app.get_snapshot(OwnerContext(OWNER), RUN_ID)
    assert isinstance(current, SnapshotSuccess)
    assert current.cursor == EventCursor(3)


@pytest.mark.anyio
async def test_command_cancel_replay_and_follow_cleanup_delegate() -> None:
    app, runs, events = facade(make_run(RunStatus.WAITING_INPUT), 2)
    command = SubmitInput(
        CommandId("cmd_t20input00"), EventId("evt_t20input00"), 1, NOW, INTERACTION_ID
    )
    resolution = InteractionResolution(
        INTERACTION_ID,
        RequestDigest.parse("b" * 64),
        ResultReference("input:t20"),
        submission(command),
    )
    first = await app.submit_input(SubmitInputRequest(OwnerContext(OWNER), resolution))
    replayed = await app.submit_input(SubmitInputRequest(OwnerContext(OWNER), resolution))
    cancel_command = CancelRun(CommandId("cmd_t20cancel0"), EventId("evt_t20cancel0"), 1, NOW)
    applied = await app.submit_command(
        OwnedCommand(OwnerContext(OWNER), submission(cancel_command))
    )
    cancelled = await app.cancel(CancelRequest(OwnerContext(OWNER), RUN_ID, NOW))
    followed = await app.follow(OwnerContext(OWNER), RUN_ID, EventCursor(2))
    assert isinstance(first, FacadeSuccess)
    assert isinstance(replayed, FacadeSuccess)
    assert replayed.replayed
    assert isinstance(applied, FacadeSuccess)
    assert isinstance(cancelled, FacadeSuccess)
    assert runs.cancelled
    assert isinstance(followed, FollowOpened)
    _ = await anext(followed.events)
    await followed.events.aclose()
    assert events.closed
