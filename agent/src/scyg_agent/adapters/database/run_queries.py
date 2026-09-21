"""Parameterized SQLAlchemy statement builders for the Run lease queue."""

from sqlalchemy import DateTime, Integer, Select, String, and_, bindparam, func, or_, select, update
from sqlalchemy.sql.dml import Update

from scyg_agent.domain.runs.repository import RenewRequest

from .run_fencing import lease_predicates
from .run_records import RunRecord


def claim_candidates_statement() -> Select[tuple[RunRecord]]:
    """Build the deterministic PostgreSQL queue lock statement."""
    now = bindparam("now", type_=DateTime(timezone=True))
    runtime_kind = bindparam("runtime_kind", type_=String())
    return (
        select(RunRecord)
        .where(
            RunRecord.runtime_kind == runtime_kind,
            or_(
                and_(
                    RunRecord.status.in_(("pending", "pending_resume")),
                    or_(RunRecord.next_attempt_at.is_(None), RunRecord.next_attempt_at <= now),
                ),
                and_(RunRecord.status == "running", RunRecord.lease_expires_at <= now),
            ),
        )
        .order_by(
            RunRecord.next_attempt_at.asc().nullsfirst(),
            RunRecord.created_at.asc(),
            RunRecord.run_id.asc(),
        )
        .limit(bindparam("limit", type_=Integer()))
        .with_for_update(skip_locked=True)
    )


def renew_lease_statement(request: RenewRequest) -> Update:
    """Build a guarded monotonic renewal returning the persisted expiry."""
    candidate = request.guard.now + request.lease_duration
    return (
        update(RunRecord)
        .where(*lease_predicates(request.guard, require_revision=True))
        .values(lease_expires_at=func.greatest(RunRecord.lease_expires_at, candidate))
        .returning(RunRecord)
    )
