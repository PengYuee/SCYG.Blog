import asyncio  # noqa: RUF100  # noqa: ANYIO_OK
from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta

import anyio
import asyncpg
import pytest
from anyio.lowlevel import checkpoint
from sqlalchemy import delete, text
from sqlalchemy.ext.asyncio import AsyncEngine, async_sessionmaker, create_async_engine

from scyg_agent.adapters.database.event_store import (
    NotificationChannel,
    PostgreSQLEventStore,
)
from scyg_agent.adapters.database.event_subscription import open_listener_connection
from scyg_agent.adapters.database.journal_records import EventRecord
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.domain.ports.event_store import (
    Appended,
    AppendRequest,
    AppendResult,
    CursorTooOld,
    EventCursor,
    EventIdentityConflict,
    EventRunNotFound,
    FutureCursor,
    PartialEventBatchConflict,
    ReplayPage,
)
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    OperationId,
    Run,
    RunId,
    RunStatus,
    RunSucceeded,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.domain.runs.repository import CreateRunRequest
from tests.acceptance_settings import require_test_settings

NOW = datetime(2026, 7, 12, 9, 0, tzinfo=UTC)
RUN_ID = RunId("run_t11event0")
TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""


@pytest.fixture
def anyio_backend() -> str:
    """Use the asyncio backend required by SQLAlchemy asyncpg."""
    return "asyncio"


@pytest.fixture
async def event_environment() -> AsyncIterator[tuple[PostgreSQLEventStore, AsyncEngine]]:
    """Create a clean event store over the shared PostgreSQL database."""
    settings = require_test_settings()
    database_url = settings.normal_url
    listener_dsn = settings.listener_dsn("normal")
    engine = create_async_engine(database_url, pool_size=8, max_overflow=0, pool_timeout=5)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    async with engine.begin() as connection:
        _ = await connection.execute(text(TRUNCATE_SQL))
    repository = PostgreSQLRunRepository(sessions)
    run = Run(
        RUN_ID,
        UserId("user-t11"),
        TaskType.SUMMARY,
        RuntimeSelection(RuntimeKind.SIMPLE, "v1"),
        1,
        RunStatus.PENDING,
        NOW,
        NOW,
        0,
        None,
        None,
    )
    _ = await repository.create(
        CreateRunRequest(run, OperationId("t11:create"), NOW, RunInput("测试输入", "article-1"))
    )
    store = PostgreSQLEventStore(
        sessions,
        listener_dsn,
        NotificationChannel.parse("scyg_t11_events"),
        open_listener_connection,
        replay_limit=20,
    )
    try:
        yield store, engine
    finally:
        await engine.dispose()


@pytest.mark.anyio
async def test_concurrent_appends_are_gap_free_and_duplicates_are_idempotent(
    event_environment: tuple[PostgreSQLEventStore, AsyncEngine],
) -> None:
    # Given: twenty independent events for one Run and two identical event identities.
    store, _ = event_environment
    results: list[AppendResult] = []

    # When: every event is committed in an independent concurrent transaction.
    async with anyio.create_task_group() as task_group:
        for index in range(20):
            _ = task_group.start_soon(_append_into, store, _event(index), results)

    # Then: committed sequence is exactly contiguous and replay has no duplicate.
    replay = await store.replay(RUN_ID, EventCursor(0), 100)
    assert isinstance(replay, ReplayPage)
    assert [stored.cursor.sequence for stored in replay.events] == list(range(1, 21))
    duplicate = await store.append(AppendRequest(RUN_ID, (_event(0),)))
    assert isinstance(duplicate, Appended)
    assert duplicate.events[0].cursor in {stored.cursor for stored in replay.events}
    conflict = await store.append(AppendRequest(RUN_ID, (_event(0, revision=2),)))
    assert isinstance(conflict, EventIdentityConflict)


@pytest.mark.anyio
async def test_replay_reports_future_old_and_notification_independent_truth(
    event_environment: tuple[PostgreSQLEventStore, AsyncEngine],
) -> None:
    # Given: five committed events, with the first two later removed by retention.
    store, engine = event_environment
    _ = await store.append(AppendRequest(RUN_ID, tuple(_event(index) for index in range(5))))
    no_notify_replay = await store.replay(RUN_ID, EventCursor(2), 2)
    assert isinstance(no_notify_replay, ReplayPage)
    async with engine.begin() as connection:
        _ = await connection.execute(
            delete(EventRecord).where(EventRecord.run_id == str(RUN_ID), EventRecord.seq < 3)
        )

    # When: clients present retained, future, and retention-stale cursors.
    retained = await store.replay(RUN_ID, EventCursor(2), 100)
    future = await store.replay(RUN_ID, EventCursor(6), 100)
    old = await store.replay(RUN_ID, EventCursor(0), 100)

    # Then: DB replay remains complete without wakeups and gaps are typed.
    assert [stored.cursor.sequence for stored in no_notify_replay.events] == [3, 4]
    assert isinstance(retained, ReplayPage)
    assert [stored.cursor.sequence for stored in retained.events] == [3, 4, 5]
    assert isinstance(future, FutureCursor)
    assert isinstance(old, CursorTooOld)
    assert await store.minimum_retained_sequence(RUN_ID) == EventCursor(3)


@pytest.mark.anyio
async def test_live_tail_replays_then_wakes_and_closes_on_cancellation(
    event_environment: tuple[PostgreSQLEventStore, AsyncEngine],
) -> None:
    # Given: one durable event before subscription and a bounded collector.
    store, _ = event_environment
    _ = await store.append(AppendRequest(RUN_ID, (_event(0),)))
    received: list[int] = []
    replayed = anyio.Event()

    async def collect() -> None:
        """Collect exactly two durable records through the public subscription surface."""
        async for stored in store.subscribe(RUN_ID, EventCursor(0)):
            received.append(stored.cursor.sequence)
            if received == [1]:
                replayed.set()
            if len(received) == 2:
                return

    # When: subscription replays existing truth and receives a later transaction wakeup.
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(collect)
        with anyio.fail_after(5):
            await replayed.wait()
        _ = await store.append(AppendRequest(RUN_ID, (_event(1),)))

    # Then: replay/listen transition delivered each durable sequence exactly once.
    assert received == [1, 2]


@pytest.mark.anyio
async def test_append_request_rejects_duplicate_identity_before_database_io(
    event_environment: tuple[PostgreSQLEventStore, AsyncEngine],
) -> None:
    # Given: one invalid batch containing the same stable identity twice.
    _, _ = event_environment
    event = _event(0)

    # When/Then: the pure request boundary rejects it before SQL executes.
    with pytest.raises(ValueError, match="identities must be unique"):
        _ = AppendRequest(RUN_ID, (event, event))


@pytest.mark.anyio
async def test_missing_run_and_partial_idempotent_batch_return_typed_outcomes_without_mutation(
    event_environment: tuple[PostgreSQLEventStore, AsyncEngine],
) -> None:
    # Given: one committed event and a separate missing Run identity.
    store, _ = event_environment
    original = await store.append(AppendRequest(RUN_ID, (_event(0),)))
    assert isinstance(original, Appended)

    # When: append targets a missing Run and then mixes existing/new IDs in one batch.
    missing_run = RunId("run_missing00")
    missing = await store.append(AppendRequest(missing_run, (_event_for_run(1, missing_run),)))
    partial = await store.append(AppendRequest(RUN_ID, (_event(0), _event(1))))

    # Then: both outcomes are typed and the original journal is unchanged.
    assert missing == EventRunNotFound(missing_run)
    assert partial == PartialEventBatchConflict(RUN_ID)
    replay = await store.replay(RUN_ID, EventCursor(0), 100)
    assert isinstance(replay, ReplayPage)
    assert [stored.event.event_id for stored in replay.events] == [EventId("evt_00000000")]


@pytest.mark.anyio
async def test_replay_limit_is_rejected_before_database_io(
    event_environment: tuple[PostgreSQLEventStore, AsyncEngine],
) -> None:
    # Given: a real adapter whose database must not interpret invalid LIMIT values.
    store, _ = event_environment

    # When/Then: invalid request limits are rejected at the method boundary.
    with pytest.raises(ValueError, match="between 1 and 1000"):
        _ = await store.replay(RUN_ID, EventCursor(0), 0)


@pytest.mark.anyio
async def test_cancelled_subscription_aclose_releases_real_listener_and_cancel_tasks(
    event_environment: tuple[PostgreSQLEventStore, AsyncEngine],
) -> None:
    # Given: a real listener factory exposing the acquired asyncpg connection.
    _, engine = event_environment
    listener_dsn = require_test_settings().listener_dsn("normal")
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    connections: list[asyncpg.Connection] = []

    async def tracking_factory(dsn: str) -> asyncpg.Connection:
        connection = await asyncpg.connect(dsn)
        connections.append(connection)
        return connection

    store = PostgreSQLEventStore(
        sessions,
        listener_dsn,
        NotificationChannel.parse("scyg_t11_events"),
        tracking_factory,
        replay_limit=20,
    )
    before_cancel_tasks = _pending_cancel_tasks()
    subscription = store.subscribe(RUN_ID, EventCursor(0))
    waiting = anyio.Event()

    async def wait_for_event() -> None:
        """Enter listener wait and remain cancellable until the owner closes the generator."""
        waiting.set()
        _ = await anext(subscription)

    # When: the owner cancels the waiting task and explicitly closes the generator.
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(wait_for_event)
        await waiting.wait()
        while not connections:
            await checkpoint()
        task_group.cancel_scope.cancel()
    await subscription.aclose()
    for _ in range(3):
        await checkpoint()

    # Then: listener transport is closed and no asyncpg cancellation task was added.
    assert len(connections) == 1
    assert connections[0].is_closed()
    assert _pending_cancel_tasks() == before_cancel_tasks


async def _append_into(
    store: PostgreSQLEventStore,
    event: RunSucceeded,
    results: list[AppendResult],
) -> None:
    """Collect one independently committed append result."""
    results.append(await store.append(AppendRequest(RUN_ID, (event,))))


def _event(index: int, *, revision: int = 1) -> RunSucceeded:
    """Create one deterministic stable event identity and content."""
    identity = f"{index:08d}"
    return RunSucceeded(
        EventId(f"evt_{identity}"),
        CommandId(f"cmd_{identity}"),
        NOW + timedelta(seconds=index),
        RUN_ID,
        revision,
    )


def _event_for_run(index: int, run_id: RunId) -> RunSucceeded:
    """Create one deterministic event for an alternate Run identity."""
    event = _event(index)
    return RunSucceeded(
        event.event_id,
        event.command_id,
        event.occurred_at,
        run_id,
        event.revision,
    )


def _pending_cancel_tasks() -> int:
    """Count unfinished asyncpg internal cancellation tasks in the active event loop."""
    return sum(
        1
        for task in asyncio.all_tasks()
        if not task.done() and "Connection._cancel" in task.get_coro().__qualname__
    )
