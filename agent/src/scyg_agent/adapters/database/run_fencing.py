"""Shared SQL fencing predicates and deterministic completion policy."""

from datetime import datetime

from sqlalchemy import ColumnElement

from scyg_agent.domain.runs import RunStatus
from scyg_agent.domain.runs.repository import CompletionRequest, LeaseGuard

from .run_records import RunRecord

TERMINAL_STATUSES = frozenset({RunStatus.SUCCEEDED, RunStatus.FAILED, RunStatus.CANCELLED})


def lease_predicates(
    guard: LeaseGuard, *, require_revision: bool
) -> tuple[ColumnElement[bool], ...]:
    """Build shared guarded update predicates without interpolated SQL."""
    predicates = (
        RunRecord.run_id == str(guard.run_id),
        RunRecord.lease_owner == str(guard.owner),
        RunRecord.lease_token == guard.token.value,
        RunRecord.status == RunStatus.RUNNING.value,
        RunRecord.lease_expires_at > guard.now,
    )
    if require_revision:
        return (*predicates, RunRecord.revision == guard.expected_revision)
    return predicates


def completion_values(
    request: CompletionRequest, attempt: int
) -> tuple[RunStatus, datetime | None, datetime | None]:
    """Resolve deterministic retry exhaustion without external work."""
    if (
        request.status is RunStatus.FAILED
        and request.retry_at is not None
        and attempt < (request.max_attempts or 0)
    ):
        return RunStatus.PENDING, request.retry_at, None
    terminal_at = request.guard.now if request.status in TERMINAL_STATUSES else None
    return request.status, None, terminal_at
