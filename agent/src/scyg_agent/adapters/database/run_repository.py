"""PostgreSQL async adapter for Run state and fenced short-lived leases."""

from collections.abc import Callable
from typing import final
from uuid import UUID, uuid4

from sqlalchemy import select, update
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.runs import ExecutionOwnerId, RunId, RunStatus
from scyg_agent.domain.runs.cancellation import (
    CancellationQueued,
    CancellationRequested,
    CancellationTerminal,
)
from scyg_agent.domain.runs.repository import (
    CancellationRequest,
    CancellationResult,
    ClaimRequest,
    CompleteResult,
    CompletionRequest,
    CreateConflict,
    Created,
    CreateResult,
    CreateRunRequest,
    DataIntegrityError,
    GetResult,
    LeaseLost,
    LeaseToken,
    NotFound,
    ReleaseRequest,
    ReleaseResult,
    Renewed,
    RenewRequest,
    RenewResult,
    RunLease,
)

from .run_fencing import TERMINAL_STATUSES, lease_predicates
from .run_mapper import map_run
from .run_queries import claim_candidates_statement, renew_lease_statement
from .run_records import CheckpointBindingRecord, RunRecord
from .run_results import (
    CompletionPlan,
    claim_results,
    complete_result,
    completion_plan,
    duplicate_result,
    get_result,
    release_result,
    renew_result,
)


@final
class PostgreSQLRunRepository:
    """Own one short SQLAlchemy transaction per repository operation."""

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        token_factory: Callable[[], UUID] = uuid4,
    ) -> None:
        """Bind a session factory and injectable cryptographic token source."""
        self._sessions = sessions
        self._token_factory = token_factory

    async def create(self, request: CreateRunRequest) -> CreateResult:
        """Insert a Run under the capability-aware idempotency identity."""
        run = request.run
        statement = insert(RunRecord).values(
            run_id=str(run.id),
            owner_user_id=str(run.owner_user_id),
            operation_id=str(request.operation_id),
            initial_message=request.input.initial_message,
            article_id=request.input.article_id,
            capability=request.input.capability,
            recipe_id=request.input.recipe_id,
            recipe_version=request.input.recipe_version,
            thread_id=request.input.thread_id or str(run.id).removeprefix("run_"),
            quality=request.input.quality,
            state_schema_version=request.input.state_schema_version,
            result_reference=None,
            input_schema_version=request.input.input_schema_version,
            input_payload=request.input.input_payload,
            input_digest=request.input.input_digest,
            locale=request.input.locale,
            task_type=run.task_type.value,
            runtime_kind=run.runtime.kind.value,
            runtime_version=run.runtime.version,
            revision=run.revision,
            status=run.status.value,
            created_at=run.created_at,
            updated_at=run.updated_at,
            attempt=run.attempt,
            next_attempt_at=request.next_attempt_at,
        )
        if request.input.capability is None:
            identity_filters = (
                RunRecord.operation_id == str(request.operation_id),
                RunRecord.capability.is_(None),
            )
            statement = statement.on_conflict_do_nothing(
                index_elements=[RunRecord.operation_id],
                index_where=RunRecord.capability.is_(None),
            )
        else:
            identity_filters = (
                RunRecord.owner_user_id == str(run.owner_user_id),
                RunRecord.capability == request.input.capability,
                RunRecord.operation_id == str(request.operation_id),
            )
            statement = statement.on_conflict_do_nothing(
                index_elements=[
                    RunRecord.owner_user_id,
                    RunRecord.capability,
                    RunRecord.operation_id,
                ],
                index_where=RunRecord.capability.is_not(None),
            )
        statement = statement.returning(RunRecord.run_id)
        async with self._sessions.begin() as session:
            inserted = (await session.execute(statement)).scalar_one_or_none()
            if inserted is not None:
                if request.input.recipe_id is not None and request.input.recipe_version is not None:
                    _ = await session.execute(
                        insert(CheckpointBindingRecord)
                        .values(
                            thread_id=request.input.thread_id or str(run.id).removeprefix("run_"),
                            run_id=str(run.id),
                            state_schema_version=request.input.state_schema_version or "v1",
                            recipe_id=request.input.recipe_id,
                            recipe_version=request.input.recipe_version,
                        )
                        .on_conflict_do_nothing()
                    )
                return Created(run)
            record = (
                await session.execute(select(RunRecord).where(*identity_filters).with_for_update())
            ).scalar_one_or_none()
            if record is None:
                return DataIntegrityError(str(run.id), "idempotency conflict row is missing")
            if (
                record.run_id != str(run.id)
                or record.owner_user_id != str(run.owner_user_id)
                or record.operation_id != str(request.operation_id)
                or record.task_type != run.task_type.value
                or record.runtime_kind != run.runtime.kind.value
                or record.runtime_version != run.runtime.version
                or record.initial_message != request.input.initial_message
                or record.article_id != request.input.article_id
                or record.capability != request.input.capability
                or record.recipe_id != request.input.recipe_id
                or record.recipe_version != request.input.recipe_version
                or record.thread_id != (request.input.thread_id or str(run.id).removeprefix("run_"))
                or record.quality != request.input.quality
                or record.state_schema_version != request.input.state_schema_version
                or record.input_schema_version != request.input.input_schema_version
                or record.input_payload != request.input.input_payload
                or record.input_digest != request.input.input_digest
                or record.locale != request.input.locale
            ):
                return CreateConflict(request.operation_id)
            return duplicate_result(record)

    async def create_in_session(self, session: AsyncSession, request: CreateRunRequest) -> Created:
        """Create an internally unique Run and checkpoint binding without committing."""
        run, value = request.run, request.input
        session.add(
            RunRecord(
                run_id=str(run.id),
                owner_user_id=str(run.owner_user_id),
                operation_id=str(request.operation_id),
                initial_message=value.initial_message,
                article_id=value.article_id,
                capability=value.capability,
                recipe_id=value.recipe_id,
                recipe_version=value.recipe_version,
                thread_id=value.thread_id or str(run.id),
                quality=value.quality,
                state_schema_version=value.state_schema_version,
                result_reference=None,
                input_schema_version=value.input_schema_version,
                input_payload=value.input_payload,
                input_digest=value.input_digest,
                locale=value.locale,
                task_type=run.task_type.value,
                runtime_kind=run.runtime.kind.value,
                runtime_version=run.runtime.version,
                revision=run.revision,
                status=run.status.value,
                created_at=run.created_at,
                updated_at=run.updated_at,
                attempt=run.attempt,
                next_attempt_at=request.next_attempt_at,
            )
        )
        await session.flush()
        if value.recipe_id is not None and value.recipe_version is not None:
            session.add(
                CheckpointBindingRecord(
                    thread_id=value.thread_id or str(run.id),
                    run_id=str(run.id),
                    state_schema_version=value.state_schema_version or "v1",
                    recipe_id=value.recipe_id,
                    recipe_version=value.recipe_version,
                )
            )
        await session.flush()
        return Created(run)

    async def get(self, run_id: RunId) -> GetResult:
        """Load and validate one persisted aggregate."""
        async with self._sessions() as session:
            record = (
                await session.execute(select(RunRecord).where(RunRecord.run_id == str(run_id)))
            ).scalar_one_or_none()
            return get_result(record, run_id)

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """Claim a bounded disjoint queue slice using SKIP LOCKED."""
        async with self._sessions.begin() as session:
            records = (
                await session.execute(
                    claim_candidates_statement(),
                    {
                        "now": request.now,
                        "limit": request.limit,
                        "runtime_kind": request.runtime_kind.value,
                    },
                )
            ).scalars()
            leases = claim_results(records, request, self._token_factory)
            await session.flush()
        return leases

    async def request_cancellation(self, request: CancellationRequest) -> CancellationResult:
        """锁定 Run 并幂等记录取消意图, 不改变活动租约."""
        async with self._sessions.begin() as session:
            return await self.request_cancellation_in_session(session, request)

    async def request_cancellation_in_session(
        self, session: AsyncSession, request: CancellationRequest
    ) -> CancellationResult:
        """在调用方事务中记录取消意图且不提交."""
        record = (
            await session.execute(
                select(RunRecord).where(RunRecord.run_id == str(request.run_id)).with_for_update()
            )
        ).scalar_one_or_none()
        if record is None:
            return NotFound(request.run_id)
        run = map_run(record)
        if isinstance(run, DataIntegrityError):
            return run
        if run.status in TERMINAL_STATUSES:
            return CancellationTerminal(run)
        replayed = record.cancellation_requested_at is not None
        if record.cancellation_requested_at is None:
            record.cancellation_requested_at = request.requested_at
        requested_at = record.cancellation_requested_at or request.requested_at
        if record.status != RunStatus.RUNNING.value:
            return CancellationQueued(request.run_id, requested_at, replayed)
        if (
            record.lease_token is None
            or record.lease_owner is None
            or record.lease_expires_at is None
        ):
            return DataIntegrityError(record.run_id, "running lease is incomplete")
        lease = RunLease(
            request.run_id,
            ExecutionOwnerId(record.lease_owner),
            LeaseToken(record.lease_token),
            record.revision,
            record.attempt,
            record.lease_expires_at,
            requested_at,
        )
        return CancellationRequested(request.run_id, requested_at, lease, replayed)

    async def renew(self, request: RenewRequest) -> RenewResult:
        """Extend one live fenced lease without incrementing revision."""
        async with self._sessions.begin() as session:
            return await self.renew_in_session(session, request)

    async def renew_in_session(self, session: AsyncSession, request: RenewRequest) -> RenewResult:
        """在调用方事务中续租或返回可观察取消意图."""
        guard = request.guard
        current = (
            await session.execute(
                select(RunRecord)
                .where(*lease_predicates(guard, require_revision=True))
                .with_for_update()
            )
        ).scalar_one_or_none()
        if current is None:
            return LeaseLost(guard.run_id)
        if current.cancellation_requested_at is not None:
            lease = renew_result(current, guard)
            if not isinstance(lease, Renewed):
                return lease
            return CancellationRequested(
                guard.run_id, current.cancellation_requested_at, lease.lease, replayed=True
            )
        record = (await session.execute(renew_lease_statement(request))).scalar_one_or_none()
        return renew_result(record, guard)

    async def release(self, request: ReleaseRequest) -> ReleaseResult:
        """Release one live lease and schedule deterministic retry eligibility."""
        guard = request.guard
        statement = (
            update(RunRecord)
            .where(*lease_predicates(guard, require_revision=True))
            .values(
                status=RunStatus.PENDING.value,
                lease_owner=None,
                lease_token=None,
                lease_expires_at=None,
                revision=RunRecord.revision + 1,
                updated_at=guard.now,
                next_attempt_at=request.next_attempt_at,
            )
            .returning(RunRecord)
        )
        async with self._sessions.begin() as session:
            record = (await session.execute(statement)).scalar_one_or_none()
            return release_result(record, request)

    async def complete(self, request: CompletionRequest) -> CompleteResult:
        """Apply one fenced terminal, retry, or waiting-input mutation."""
        guard = request.guard
        async with self._sessions.begin() as session:
            live = (
                await session.execute(
                    select(RunRecord)
                    .where(*lease_predicates(guard, require_revision=False))
                    .with_for_update()
                )
            ).scalar_one_or_none()
            planned = completion_plan(live, request)
            if not isinstance(planned, CompletionPlan):
                return planned
            statement = (
                update(RunRecord)
                .where(*lease_predicates(guard, require_revision=True))
                .values(
                    status=planned.status.value,
                    lease_owner=None,
                    lease_token=None,
                    lease_expires_at=None,
                    pending_interaction_id=(
                        str(request.pending_interaction_id)
                        if request.pending_interaction_id is not None
                        else None
                    ),
                    revision=RunRecord.revision + 1,
                    updated_at=guard.now,
                    next_attempt_at=planned.next_attempt_at,
                    terminal_at=planned.terminal_at,
                )
                .returning(RunRecord)
            )
            record = (await session.execute(statement)).scalar_one_or_none()
            return complete_result(record, request, planned)
