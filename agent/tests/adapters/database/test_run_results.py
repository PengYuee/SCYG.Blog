"""Pure result mapping tests for Run persistence."""

from datetime import UTC, datetime, timedelta
from uuid import UUID

from scyg_agent.adapters.database.run_fencing import lease_predicates
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.run_results import (
    CompletionPlan,
    claim_results,
    complete_result,
    completion_plan,
    duplicate_result,
    get_result,
    release_result,
    renew_result,
)
from scyg_agent.domain.runs import ExecutionOwnerId, Run, RunId, RunStatus, RuntimeKind
from scyg_agent.domain.runs.repository import (
    ClaimRequest,
    Completed,
    CompletionRequest,
    DataIntegrityError,
    DuplicateOperation,
    LeaseGuard,
    LeaseLost,
    LeaseToken,
    NotFound,
    ReleaseRequest,
    Renewed,
    RetryScheduled,
    RevisionConflict,
    TerminalFailure,
)

NOW = datetime(2026, 7, 12, 9, tzinfo=UTC)
TOKEN = LeaseToken(UUID("12345678-1234-5678-9234-567812345678"))
WORKER = ExecutionOwnerId("worker_12345678")
RUN_ID = RunId("run_12345678")


def test_duplicate_maps_valid_row() -> None:
    assert isinstance(duplicate_result(_record(RunStatus.PENDING)), DuplicateOperation)


def test_duplicate_contains_corruption() -> None:
    row = _record(RunStatus.PENDING)
    row.status = "future"
    assert isinstance(duplicate_result(row), DataIntegrityError)


def test_get_maps_absence() -> None:
    assert isinstance(get_result(None, RUN_ID), NotFound)


def test_get_maps_valid_row() -> None:
    assert isinstance(get_result(_record(RunStatus.PENDING), RUN_ID), Run)


def test_claim_mutates_fence_and_preserves_cancellation() -> None:
    row = _record(RunStatus.PENDING)
    row.cancellation_requested_at = NOW
    request = ClaimRequest(1, WORKER, RuntimeKind.SIMPLE, NOW, timedelta(minutes=5))
    lease = claim_results((row,), request, lambda: TOKEN.value)[0]
    assert lease.revision == 2
    assert lease.attempt == 1
    assert lease.cancellation_requested_at == NOW


def test_renew_maps_absence_to_lease_loss() -> None:
    assert isinstance(renew_result(None, _guard()), LeaseLost)


def test_renew_returns_persisted_expiry_and_cancellation() -> None:
    row = _record(RunStatus.RUNNING)
    row.cancellation_requested_at = NOW
    result = renew_result(row, _guard())
    assert isinstance(result, Renewed)
    assert result.lease.expires_at == row.lease_expires_at
    assert result.lease.cancellation_requested_at == NOW


def test_release_result_maps_success_loss_and_corruption() -> None:
    request = ReleaseRequest(_guard(), NOW + timedelta(minutes=1))
    pending = _record(RunStatus.PENDING)
    assert isinstance(release_result(pending, request), RetryScheduled)
    assert isinstance(release_result(None, request), LeaseLost)
    pending.status = "future"
    assert isinstance(release_result(pending, request), LeaseLost)


def test_completion_plan_maps_fence_and_retry_policy() -> None:
    guard = _guard()
    request = CompletionRequest(
        guard, RunStatus.FAILED, retry_at=NOW + timedelta(minutes=1), max_attempts=2
    )
    live = _record(RunStatus.RUNNING)
    assert isinstance(completion_plan(None, request), LeaseLost)
    live.revision = 3
    assert isinstance(completion_plan(live, request), RevisionConflict)
    live.revision = 2
    plan = completion_plan(live, request)
    assert isinstance(plan, CompletionPlan)
    assert plan.status is RunStatus.PENDING


def test_completion_plan_maps_terminal_failure() -> None:
    request = CompletionRequest(_guard(), RunStatus.FAILED)
    plan = completion_plan(_record(RunStatus.RUNNING), request)
    assert isinstance(plan, CompletionPlan)
    assert plan.status is RunStatus.FAILED
    assert plan.terminal_at == NOW


def test_complete_result_maps_all_public_shapes() -> None:
    guard = _guard()
    success = CompletionRequest(guard, RunStatus.SUCCEEDED)
    retry = CompletionRequest(
        guard, RunStatus.FAILED, retry_at=NOW + timedelta(minutes=1), max_attempts=2
    )
    failure = CompletionRequest(guard, RunStatus.FAILED)
    assert isinstance(
        complete_result(
            _record(RunStatus.SUCCEEDED), success, CompletionPlan(RunStatus.SUCCEEDED, None, NOW)
        ),
        Completed,
    )
    assert isinstance(
        complete_result(
            _record(RunStatus.PENDING),
            retry,
            CompletionPlan(RunStatus.PENDING, retry.retry_at, None),
        ),
        RetryScheduled,
    )
    assert isinstance(
        complete_result(
            _record(RunStatus.FAILED), failure, CompletionPlan(RunStatus.FAILED, None, NOW)
        ),
        TerminalFailure,
    )
    assert isinstance(
        complete_result(None, success, CompletionPlan(RunStatus.SUCCEEDED, None, NOW)), LeaseLost
    )


def test_lease_predicates_include_optional_revision() -> None:
    assert len(lease_predicates(_guard(), require_revision=False)) == 5
    assert len(lease_predicates(_guard(), require_revision=True)) == 6


def _guard() -> LeaseGuard:
    return LeaseGuard(RUN_ID, WORKER, TOKEN, 2, NOW)


def _record(status: RunStatus) -> RunRecord:
    running = status is RunStatus.RUNNING
    return RunRecord(
        run_id=str(RUN_ID),
        owner_user_id="user-1",
        operation_id="op:run",
        task_type="summary",
        runtime_kind="simple",
        runtime_version="v1",
        revision=2 if running else 1,
        status=status.value,
        created_at=NOW,
        updated_at=NOW,
        attempt=1 if running else 0,
        next_attempt_at=NOW if status is RunStatus.PENDING else None,
        lease_owner=str(WORKER) if running else None,
        lease_token=TOKEN.value if running else None,
        lease_expires_at=NOW + timedelta(minutes=10) if running else None,
        cancellation_requested_at=None,
        pending_interaction_id=None,
        terminal_at=(
            NOW if status in {RunStatus.SUCCEEDED, RunStatus.FAILED, RunStatus.CANCELLED} else None
        ),
        terminal_metadata=None,
        error_code=None,
        error_message=None,
        error_metadata=None,
    )
