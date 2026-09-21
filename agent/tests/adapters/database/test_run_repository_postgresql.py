"""Real PostgreSQL acceptance tests for fenced Run leases."""

from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from uuid import UUID

import anyio
import pytest
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine, async_sessionmaker, create_async_engine

from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.domain.runs import (
    ExecutionOwnerId,
    InteractionId,
    OperationId,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.cancellation import CancellationQueued, CancellationRequested
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.domain.runs.repository import (
    CancellationRequest,
    ClaimRequest,
    Completed,
    CompletionRequest,
    CreateConflict,
    Created,
    CreateRunRequest,
    DuplicateOperation,
    LeaseGuard,
    LeaseLost,
    ReleaseRequest,
    RenewRequest,
    RunLease,
)
from tests.acceptance_settings import require_test_settings

NOW = datetime(2026, 7, 12, 9, 0, tzinfo=UTC)
LEASE_DURATION = timedelta(minutes=5)
TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""


@pytest.fixture
def anyio_backend() -> str:
    """Use the asyncio backend required by SQLAlchemy asyncpg."""
    return "asyncio"


@pytest.fixture
async def repository_environment() -> AsyncIterator[tuple[PostgreSQLRunRepository, AsyncEngine]]:
    """Create a clean repository over the shared PostgreSQL database."""
    database_url = require_test_settings().normal_url
    engine = create_async_engine(database_url, pool_size=5, max_overflow=0, pool_timeout=5)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    async with engine.begin() as connection:
        _ = await connection.execute(text(TRUNCATE_SQL))
    try:
        yield PostgreSQLRunRepository(sessions), engine
    finally:
        await engine.dispose()


@pytest.mark.anyio
async def test_concurrent_claimers_receive_disjoint_capacity_bounded_sets(
    repository_environment: tuple[PostgreSQLRunRepository, AsyncEngine],
) -> None:
    # Given: eight deterministically ordered claimable runs and independent sessions.
    repository, engine = repository_environment
    await _seed(repository, 8)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    first = PostgreSQLRunRepository(sessions)
    second = PostgreSQLRunRepository(sessions)
    claims: list[tuple[RunLease, ...]] = []

    # When: two workers claim four units of free capacity concurrently.
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(_claim_into, first, "worker_first000", claims)
        _ = task_group.start_soon(_claim_into, second, "worker_second00", claims)

    # Then: ownership sets are disjoint and total allocation is capacity bounded.
    assert len(claims) == 2
    first_ids = {lease.run_id for lease in claims[0]}
    second_ids = {lease.run_id for lease in claims[1]}
    assert first_ids.isdisjoint(second_ids)
    assert len(first_ids | second_ids) == 8
    assert all(len(claim) <= 4 for claim in claims)


@pytest.mark.anyio
async def test_mixed_runtime_claims_and_cancellation_visibility(
    repository_environment: tuple[PostgreSQLRunRepository, AsyncEngine],
) -> None:
    # Given: SIMPLE 与 DEEP 混合队列以及两个独立容量查询。
    repository, engine = repository_environment
    await _seed(repository, 3)
    await _seed(repository, 3, runtime_kind=RuntimeKind.DEEP, start=3)
    sessions = async_sessionmaker(engine, expire_on_commit=False)
    simple_repository = PostgreSQLRunRepository(sessions)
    deep_repository = PostgreSQLRunRepository(sessions)
    claims: list[tuple[RunLease, ...]] = []

    # When: 两个 worker 并发领取不同运行时容量。
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(
            _claim_kind_into, simple_repository, RuntimeKind.SIMPLE, "worker_simple000", claims
        )
        _ = task_group.start_soon(
            _claim_kind_into, deep_repository, RuntimeKind.DEEP, "worker_deep00000", claims
        )

    # Then: 两组 Run 完全分离, 且租约内可观察后续取消请求。
    assert {lease.run_id for lease in claims[0]}.isdisjoint({lease.run_id for lease in claims[1]})
    assert all(len(group) == 3 for group in claims)
    lease = next(
        lease
        for group in claims
        for lease in group
        if lease.owner == ExecutionOwnerId("worker_simple000")
    )
    first = await repository.request_cancellation(CancellationRequest(lease.run_id, NOW))
    second = await repository.request_cancellation(CancellationRequest(lease.run_id, NOW))
    assert isinstance(first, CancellationRequested)
    assert isinstance(second, CancellationRequested)
    assert first.lease is not None
    assert first.lease.token == lease.token
    assert second.replayed
    renewal = await repository.renew(
        RenewRequest(
            LeaseGuard(lease.run_id, lease.owner, lease.token, lease.revision, NOW),
            timedelta(minutes=1),
        )
    )
    assert isinstance(renewal, CancellationRequested)


@pytest.mark.anyio
async def test_cancellation_before_claim_is_carried_by_new_lease(
    repository_environment: tuple[PostgreSQLRunRepository, AsyncEngine],
) -> None:
    # Given: 一个尚未领取的 Run 已被幂等请求取消。
    repository, _ = repository_environment
    await _seed(repository, 1)
    run_id = RunId("run_00000000")
    requested = await repository.request_cancellation(CancellationRequest(run_id, NOW))
    replayed = await repository.request_cancellation(CancellationRequest(run_id, NOW))
    assert isinstance(requested, CancellationQueued)
    assert isinstance(replayed, CancellationQueued)
    assert replayed.replayed

    # When: worker 按正确运行时领取该 Run。
    lease = (
        await repository.claim(
            ClaimRequest(
                1, ExecutionOwnerId("worker_cancel000"), RuntimeKind.SIMPLE, NOW, LEASE_DURATION
            )
        )
    )[0]

    # Then: 取消意图随围栏返回且未清除新租约。
    assert lease.cancellation_requested_at == NOW


@pytest.mark.anyio
async def test_expiry_reclaim_replaces_fence_and_rejects_every_stale_mutation(
    repository_environment: tuple[PostgreSQLRunRepository, AsyncEngine],
) -> None:
    # Given: a lease expiring exactly at the reclaim clock boundary.
    repository, _ = repository_environment
    await _seed(repository, 1)
    old = (
        await repository.claim(
            ClaimRequest(
                1, ExecutionOwnerId("worker_original"), RuntimeKind.SIMPLE, NOW, LEASE_DURATION
            )
        )
    )[0]
    boundary = old.expires_at

    # When: another owner claims at expiry where live means strictly expiry > now.
    reclaimed = (
        await repository.claim(
            ClaimRequest(
                1,
                ExecutionOwnerId("worker_reclaimer"),
                RuntimeKind.SIMPLE,
                boundary,
                LEASE_DURATION,
            )
        )
    )[0]

    # Then: revision and attempt advance once and every old-token mutation loses the lease.
    assert reclaimed.token != old.token
    assert reclaimed.owner != old.owner
    assert reclaimed.revision == old.revision + 1
    assert reclaimed.attempt == old.attempt + 1
    stale_guard = LeaseGuard(old.run_id, old.owner, old.token, old.revision, boundary)
    assert isinstance(
        await repository.renew(RenewRequest(stale_guard, timedelta(minutes=1))), LeaseLost
    )
    assert isinstance(await repository.release(ReleaseRequest(stale_guard, boundary)), LeaseLost)
    assert isinstance(
        await repository.complete(CompletionRequest(stale_guard, RunStatus.SUCCEEDED)), LeaseLost
    )
    current = await repository.get(old.run_id)
    assert isinstance(current, Run)
    assert (current.revision, current.attempt, current.execution_owner) == (
        reclaimed.revision,
        reclaimed.attempt,
        reclaimed.owner,
    )


@pytest.mark.anyio
async def test_claim_transaction_rolls_back_when_token_factory_fails(
    repository_environment: tuple[PostgreSQLRunRepository, AsyncEngine],
) -> None:
    # Given: two claimable rows and a token source that fails after one mutation.
    repository, engine = repository_environment
    await _seed(repository, 2)
    calls = 0

    def failing_token_factory() -> UUID:
        """Fail on the second token to exercise transaction rollback."""
        nonlocal calls
        calls += 1
        if calls == 2:
            message = "injected token failure"
            raise RuntimeError(message)
        return UUID("12345678-1234-5678-9234-567812345678")

    sessions = async_sessionmaker(engine, expire_on_commit=False)
    failing_repository = PostgreSQLRunRepository(sessions, failing_token_factory)

    # When: claim processing fails after touching the first locked record.
    with pytest.raises(RuntimeError, match="injected token failure"):
        _ = await failing_repository.claim(
            ClaimRequest(
                2, ExecutionOwnerId("worker_failure00"), RuntimeKind.SIMPLE, NOW, LEASE_DURATION
            )
        )

    # Then: the short transaction rolled back and both rows remain claimable.
    leases = await repository.claim(
        ClaimRequest(
            2, ExecutionOwnerId("worker_recovery0"), RuntimeKind.SIMPLE, NOW, LEASE_DURATION
        )
    )
    assert len(leases) == 2
    assert all(lease.revision == 2 and lease.attempt == 1 for lease in leases)


@pytest.mark.anyio
async def test_create_is_idempotent_and_waiting_input_releases_ownership(
    repository_environment: tuple[PostgreSQLRunRepository, AsyncEngine],
) -> None:
    # Given: an operation already created and its Run claimed for execution.
    repository, _ = repository_environment
    await _seed(repository, 1)
    original = await repository.get(RunId("run_00000000"))
    assert isinstance(original, Run)
    duplicate = await repository.create(
        CreateRunRequest(
            original,
            OperationId("t10:00000000"),
            NOW,
            RunInput("测试输入", "article-1"),
        )
    )
    lease = (
        await repository.claim(
            ClaimRequest(
                1, ExecutionOwnerId("worker_waiting00"), RuntimeKind.SIMPLE, NOW, LEASE_DURATION
            )
        )
    )[0]

    # When: the worker completes into WAITING_INPUT with the exact revision fence.
    interaction_id = InteractionId("int_12345678")
    completed = await repository.complete(
        CompletionRequest(
            LeaseGuard(lease.run_id, lease.owner, lease.token, lease.revision, NOW),
            RunStatus.WAITING_INPUT,
            pending_interaction_id=interaction_id,
        )
    )

    # Then: create returned the original and waiting state no longer owns execution.
    assert isinstance(duplicate, DuplicateOperation)
    assert duplicate.run == original
    assert isinstance(completed, Completed)
    assert completed.run.status is RunStatus.WAITING_INPUT
    assert completed.run.execution_owner is None
    assert completed.run.pending_interaction_id == interaction_id
    assert completed.run.revision == lease.revision + 1


@pytest.mark.anyio
async def test_capability_idempotency_is_scoped_by_user_and_capability(
    repository_environment: tuple[PostgreSQLRunRepository, AsyncEngine],
) -> None:
    """A capability identity replays only within its user/capability scope."""
    repository, _ = repository_environment
    first_request = _capability_request("run_cap_first", "user-cap-a", "search", "cap-key")

    first = await repository.create(first_request)
    replay = await repository.create(first_request)
    conflict = await repository.create(
        _capability_request("run_cap_conflict", "user-cap-a", "search", "cap-key", "changed")
    )
    other_user = await repository.create(
        _capability_request("run_cap_other_user", "user-cap-b", "search", "cap-key")
    )
    other_capability = await repository.create(
        _capability_request("run_cap_other_cap", "user-cap-a", "write", "cap-key")
    )

    assert isinstance(first, Created)
    assert isinstance(replay, DuplicateOperation)
    assert isinstance(conflict, CreateConflict)
    assert isinstance(other_user, Created)
    assert isinstance(other_capability, Created)


def _capability_request(
    run_id: str,
    owner: str,
    capability: str,
    operation: str,
    message: str = "测试输入",
) -> CreateRunRequest:
    """Build one capability-backed create request with deterministic snapshot data."""
    run = Run(
        id=RunId(run_id),
        owner_user_id=UserId(owner),
        task_type=TaskType.SUMMARY,
        runtime=RuntimeSelection(RuntimeKind.SIMPLE, "v1"),
        revision=1,
        status=RunStatus.PENDING,
        created_at=NOW,
        updated_at=NOW,
        attempt=0,
        execution_owner=None,
        pending_interaction_id=None,
    )
    return CreateRunRequest(
        run,
        OperationId(operation),
        NOW,
        RunInput(
            message,
            "article-capability",
            capability=capability,
            recipe_id=f"{capability}-v1",
            recipe_version="v1",
            input_schema_version="v1",
            input_payload={"message": message, "capability": capability},
            input_digest=("a" if message == "测试输入" else "b") * 64,
            locale="zh-CN",
        ),
    )


async def _claim_into(
    repository: PostgreSQLRunRepository,
    worker: str,
    claims: list[tuple[RunLease, ...]],
) -> None:
    """Collect one concurrent claim result after its transaction commits."""
    leases = await repository.claim(
        ClaimRequest(4, ExecutionOwnerId(worker), RuntimeKind.SIMPLE, NOW, LEASE_DURATION)
    )
    claims.append(leases)


async def _claim_kind_into(
    repository: PostgreSQLRunRepository,
    runtime_kind: RuntimeKind,
    worker: str,
    claims: list[tuple[RunLease, ...]],
) -> None:
    """按运行时族收集一次并发认领结果."""
    leases = await repository.claim(
        ClaimRequest(4, ExecutionOwnerId(worker), runtime_kind, NOW, LEASE_DURATION)
    )
    claims.append(leases)


async def _seed(
    repository: PostgreSQLRunRepository,
    count: int,
    *,
    runtime_kind: RuntimeKind = RuntimeKind.SIMPLE,
    start: int = 0,
) -> None:
    """Persist deterministic pending runs through the public repository API."""
    for index in range(start, start + count):
        identity = f"{index:08d}"
        run = Run(
            id=RunId(f"run_{identity}"),
            owner_user_id=UserId("user-t10"),
            task_type=(
                TaskType.SUMMARY if runtime_kind is RuntimeKind.SIMPLE else TaskType.RESEARCH
            ),
            runtime=RuntimeSelection(runtime_kind, "v1"),
            revision=1,
            status=RunStatus.PENDING,
            created_at=NOW + timedelta(seconds=index),
            updated_at=NOW + timedelta(seconds=index),
            attempt=0,
            execution_owner=None,
            pending_interaction_id=None,
        )
        _ = await repository.create(
            CreateRunRequest(
                run,
                OperationId(f"t10:{identity}"),
                NOW,
                RunInput("测试输入", f"article-{identity}"),
            )
        )
