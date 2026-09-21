"""Run repository port value tests."""

from datetime import UTC, datetime, timedelta
from uuid import UUID

import pytest

from scyg_agent.domain.runs import (
    ExecutionOwnerId,
    InteractionId,
    RunId,
    RunStatus,
    RuntimeKind,
)
from scyg_agent.domain.runs.repository import (
    CancellationRequest,
    ClaimRequest,
    CompletionRequest,
    InvalidRepositoryInputError,
    LeaseGuard,
    LeaseToken,
    ReleaseRequest,
    RenewRequest,
)

NOW = datetime(2026, 7, 12, 9, tzinfo=UTC)
WORKER = ExecutionOwnerId("worker_12345678")
TOKEN = LeaseToken(UUID("12345678-1234-5678-9234-567812345678"))
GUARD = LeaseGuard(RunId("run_12345678"), WORKER, TOKEN, 1, NOW)


@pytest.mark.parametrize("limit", [0, -1])
def test_claim_rejects_nonpositive_limit(limit: int) -> None:
    with pytest.raises(InvalidRepositoryInputError, match="limit"):
        _ = ClaimRequest(limit, WORKER, RuntimeKind.SIMPLE, NOW, timedelta(minutes=1))


def test_claim_rejects_naive_time() -> None:
    naive = datetime(2026, 7, 12, tzinfo=None)  # noqa: DTZ001
    with pytest.raises(InvalidRepositoryInputError, match="now"):
        _ = ClaimRequest(1, WORKER, RuntimeKind.SIMPLE, naive, timedelta(minutes=1))


def test_claim_rejects_nonpositive_duration() -> None:
    with pytest.raises(InvalidRepositoryInputError, match="lease duration"):
        _ = ClaimRequest(1, WORKER, RuntimeKind.SIMPLE, NOW, timedelta(0))


def test_claim_accepts_exact_runtime_kind() -> None:
    request = ClaimRequest(2, WORKER, RuntimeKind.DEEP, NOW, timedelta(minutes=1))
    assert request.runtime_kind is RuntimeKind.DEEP


def test_renew_rejects_nonpositive_duration() -> None:
    with pytest.raises(InvalidRepositoryInputError, match="lease duration"):
        _ = RenewRequest(GUARD, timedelta(0))


def test_release_rejects_regressing_schedule() -> None:
    with pytest.raises(InvalidRepositoryInputError, match="next_attempt_at"):
        _ = ReleaseRequest(GUARD, NOW - timedelta(seconds=1))


def test_completion_requires_interaction_for_waiting() -> None:
    with pytest.raises(InvalidRepositoryInputError, match="pending interaction"):
        _ = CompletionRequest(GUARD, RunStatus.WAITING_INPUT)


def test_completion_accepts_waiting_interaction() -> None:
    request = CompletionRequest(
        GUARD, RunStatus.WAITING_INPUT, pending_interaction_id=InteractionId("int_12345678")
    )
    assert request.pending_interaction_id == InteractionId("int_12345678")


def test_cancellation_request_requires_utc() -> None:
    naive = datetime(2026, 7, 12, tzinfo=None)  # noqa: DTZ001
    with pytest.raises(InvalidRepositoryInputError, match="requested_at"):
        _ = CancellationRequest(RunId("run_12345678"), naive)


def test_cancellation_request_preserves_identity() -> None:
    request = CancellationRequest(RunId("run_12345678"), NOW)
    assert request.run_id == RunId("run_12345678")
