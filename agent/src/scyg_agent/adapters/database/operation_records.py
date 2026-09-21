"""Tool operation and immutable audit SQLAlchemy records."""

from datetime import datetime
from typing import Final, final
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.dialects.postgresql import UUID as PGUUID
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.schema import SchemaItem

from .metadata import Base

TOOL_OPEN_CHECK: Final = "(status IN ('pending','in_flight') AND completed_at IS NULL)"
TOOL_COMPLETED_CHECK: Final = "(status NOT IN ('pending','in_flight') AND completed_at IS NOT NULL)"
TOOL_CLAIM_CHECK: Final = (
    "(status = 'in_flight') = (claim_token IS NOT NULL AND claim_expires_at IS NOT NULL)"
)
TOOL_RPC_STATUSES: Final = "('in_flight','succeeded','failed','external_outcome_unknown')"
TOOL_RPC_CHECK: Final = f"rpc_started_at IS NULL OR status IN {TOOL_RPC_STATUSES}"
TOOL_INTENT_CHECK: Final = (
    "(intent_semantic_digest IS NULL AND approval_interaction_id IS NULL "
    "AND approval_resolution_digest IS NULL AND status IN ('succeeded','failed')) OR "
    "(intent_semantic_digest IS NOT NULL AND approval_interaction_id IS NOT NULL "
    "AND approval_resolution_digest IS NOT NULL)"
)


@final
class ToolCallRecord(Base):
    """Persist tool idempotency and sanitized original outcome metadata."""

    __tablename__ = "agent_tool_calls"
    __table_args__: tuple[SchemaItem, ...] = (
        UniqueConstraint("operation_id"),
        CheckConstraint(
            "status IN ('pending','in_flight','succeeded','failed','external_outcome_unknown')",
            name="status_valid",
        ),
        CheckConstraint(
            f"{TOOL_OPEN_CHECK} OR {TOOL_COMPLETED_CHECK}",
            name="completion_invariants",
        ),
        CheckConstraint(TOOL_CLAIM_CHECK, name="claim_invariants"),
        CheckConstraint("claim_version >= 0", name="claim_version_nonnegative"),
        CheckConstraint(TOOL_RPC_CHECK, name="rpc_start_invariants"),
        CheckConstraint(TOOL_INTENT_CHECK, name="intent_invariants"),
        Index("ix_agent_tool_calls_run_id_status", "run_id", "status"),
    )

    tool_call_id: Mapped[str] = mapped_column(String(68), primary_key=True)
    operation_id: Mapped[str] = mapped_column(String(128))
    run_id: Mapped[str] = mapped_column(ForeignKey("agent_runs.run_id", ondelete="CASCADE"))
    tool_name: Mapped[str] = mapped_column(String(128))
    status: Mapped[str] = mapped_column(String(32))
    request_digest: Mapped[str] = mapped_column(
        String(128), comment="sanitized request digest; never raw request data"
    )
    semantic_digest: Mapped[str] = mapped_column(
        String(64), comment="canonical immutable tool operation semantics"
    )
    intent_semantic_digest: Mapped[str | None] = mapped_column(String(64))
    approval_interaction_id: Mapped[str | None] = mapped_column(
        String(68),
        ForeignKey("agent_interactions.interaction_id", ondelete="RESTRICT"),
    )
    approval_resolution_digest: Mapped[str | None] = mapped_column(String(64))
    request_audit_metadata: Mapped[dict[str, str]] = mapped_column(
        JSONB, comment="sanitized immutable audit correlation metadata"
    )
    result_reference: Mapped[str | None] = mapped_column(String(256))
    result_metadata: Mapped[dict[str, str] | None] = mapped_column(
        JSONB, comment="sanitized typed serializer output only; no secrets"
    )
    error_code: Mapped[str | None] = mapped_column(String(64))
    error_metadata: Mapped[dict[str, str] | None] = mapped_column(
        JSONB, comment="sanitized typed serializer output only; no secrets"
    )
    started_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    completed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    claim_token: Mapped[UUID | None] = mapped_column(PGUUID(as_uuid=True))
    claim_version: Mapped[int] = mapped_column(Integer, default=0)
    claim_expires_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    rpc_started_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))


@final
class AuditEventRecord(Base):
    """Persist append-only audit facts; application adapters must never update rows."""

    __tablename__ = "agent_audit_events"
    __table_args__: tuple[SchemaItem, ...] = (
        UniqueConstraint("run_id", "seq"),
        CheckConstraint("seq >= 0", name="seq_nonnegative"),
        Index("ix_agent_audit_events_run_id_occurred_at", "run_id", "occurred_at"),
        Index("ix_agent_audit_events_user_id_occurred_at", "user_id", "occurred_at"),
    )

    audit_id: Mapped[str] = mapped_column(String(68), primary_key=True)
    seq: Mapped[int] = mapped_column(Integer)
    occurred_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    run_id: Mapped[str] = mapped_column(ForeignKey("agent_runs.run_id", ondelete="RESTRICT"))
    user_id: Mapped[str] = mapped_column(String(128))
    command_id: Mapped[str | None] = mapped_column(
        ForeignKey("agent_commands.command_id", ondelete="SET NULL")
    )
    tool_call_id: Mapped[str | None] = mapped_column(
        ForeignKey("agent_tool_calls.tool_call_id", ondelete="SET NULL")
    )
    action: Mapped[str] = mapped_column(String(128))
    outcome: Mapped[str] = mapped_column(String(64))
    audit_metadata: Mapped[dict[str, str]] = mapped_column(
        "metadata",
        JSONB,
        comment="sanitized typed serializer output only; no secrets",
    )
