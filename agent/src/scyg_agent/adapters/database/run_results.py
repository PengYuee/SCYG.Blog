"""Pure result mapping and in-memory row mutation for the Run repository adapter."""

from collections.abc import Callable, Iterable
from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from scyg_agent.domain.runs import RunId, RunStatus
from scyg_agent.domain.runs.repository import (
    ClaimRequest,
    Completed,
    CompleteResult,
    CompletionRequest,
    DataIntegrityError,
    DuplicateOperation,
    GetResult,
    LeaseGuard,
    LeaseLost,
    LeaseToken,
    NotFound,
    ReleaseRequest,
    ReleaseResult,
    Renewed,
    RenewResult,
    RetryScheduled,
    RevisionConflict,
    RunLease,
    TerminalFailure,
)

from .run_fencing import completion_values
from .run_mapper import map_run
from .run_records import RunRecord


@dataclass(frozen=True, slots=True)
class CompletionPlan:
    """Carry a locked row and its deterministic completion values."""

    status: RunStatus
    next_attempt_at: datetime | None
    terminal_at: datetime | None


def duplicate_result(record: RunRecord) -> DuplicateOperation | DataIntegrityError:
    """Map the original idempotent operation row."""
    mapped = map_run(record)
    if isinstance(mapped, DataIntegrityError):
        return mapped
    return DuplicateOperation(mapped)


def get_result(record: RunRecord | None, run_id: RunId) -> GetResult:
    """Map an optional persisted row into the typed get result."""
    if record is None:
        return NotFound(run_id)
    return map_run(record)


def claim_results(
    records: Iterable[RunRecord],
    request: ClaimRequest,
    token_factory: Callable[[], UUID],
) -> tuple[RunLease, ...]:
    """Apply one claim fence to already locked rows and return exact leases."""
    expires_at = request.now + request.lease_duration
    leases: list[RunLease] = []
    for record in records:
        token = LeaseToken(token_factory())
        record.status = RunStatus.RUNNING.value
        record.lease_owner = str(request.worker)
        record.lease_token = token.value
        record.lease_expires_at = expires_at
        record.pending_interaction_id = None
        record.revision += 1
        record.attempt += 1
        record.updated_at = request.now
        leases.append(
            RunLease(
                RunId(record.run_id),
                request.worker,
                token,
                record.revision,
                record.attempt,
                expires_at,
                record.cancellation_requested_at,
            )
        )
    return tuple(leases)


def renew_result(record: RunRecord | None, guard: LeaseGuard) -> RenewResult:
    """Map the persisted monotonic expiry returned by a guarded renewal."""
    if record is None or record.lease_expires_at is None:
        return LeaseLost(guard.run_id)
    return Renewed(
        RunLease(
            guard.run_id,
            guard.owner,
            guard.token,
            guard.expected_revision,
            record.attempt,
            record.lease_expires_at,
            record.cancellation_requested_at,
        )
    )


def release_result(record: RunRecord | None, request: ReleaseRequest) -> ReleaseResult:
    """Map a guarded release row while containing corrupt persisted state."""
    if record is None:
        return LeaseLost(request.guard.run_id)
    mapped = map_run(record)
    if isinstance(mapped, DataIntegrityError):
        return LeaseLost(request.guard.run_id)
    return RetryScheduled(mapped, request.next_attempt_at)


def completion_plan(
    record: RunRecord | None, request: CompletionRequest
) -> CompletionPlan | LeaseLost | RevisionConflict:
    """Validate the locked live row before its fenced completion update."""
    guard = request.guard
    if record is None:
        return LeaseLost(guard.run_id)
    if record.revision != guard.expected_revision:
        return RevisionConflict(guard.run_id, guard.expected_revision, record.revision)
    status, next_attempt_at, terminal_at = completion_values(request, record.attempt)
    return CompletionPlan(status, next_attempt_at, terminal_at)


def complete_result(
    record: RunRecord | None, request: CompletionRequest, plan: CompletionPlan
) -> CompleteResult:
    """Map the completed row into its exhaustive public outcome."""
    if record is None:
        return LeaseLost(request.guard.run_id)
    mapped = map_run(record)
    if isinstance(mapped, DataIntegrityError):
        return LeaseLost(request.guard.run_id)
    if plan.status is RunStatus.PENDING:
        return RetryScheduled(mapped, request.retry_at or request.guard.now)
    if plan.status is RunStatus.FAILED:
        return TerminalFailure(mapped)
    return Completed(mapped)
