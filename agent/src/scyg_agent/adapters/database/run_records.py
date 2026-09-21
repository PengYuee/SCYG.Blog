"""Run and interaction SQLAlchemy records."""

from datetime import datetime
from typing import Final, final
from uuid import UUID

from sqlalchemy import CheckConstraint, DateTime, ForeignKey, Index, Integer, String, Text, text
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.dialects.postgresql import UUID as PGUUID
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.schema import SchemaItem

from .metadata import Base

RUN_STATUS_CHECK: Final = (
    "status IN ('pending','running','waiting_input','pending_resume',"
    "'succeeded','failed','cancelled')"
)
RUN_LEASE_CHECK: Final = (
    "(status = 'running') = (lease_owner IS NOT NULL "
    "AND lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)"
)
INTERACTION_RESOLUTION_CHECK: Final = (
    "(status = 'pending' AND resolved_at IS NULL AND response_digest IS NULL) OR "
    "(status = 'resolved' AND resolved_at IS NOT NULL AND response_digest IS NOT NULL)"
)


@final
class RunRecord(Base):
    """Persist mutable Run state for transactional fencing."""

    __tablename__ = "agent_runs"
    __table_args__: tuple[SchemaItem, ...] = (
        CheckConstraint("revision >= 1", name="revision_positive"),
        CheckConstraint("attempt >= 0", name="attempt_nonnegative"),
        CheckConstraint(
            RUN_STATUS_CHECK,
            name="status_valid",
        ),
        CheckConstraint(
            RUN_LEASE_CHECK,
            name="lease_invariants",
        ),
        CheckConstraint(
            "(status = 'waiting_input') = (pending_interaction_id IS NOT NULL)",
            name="state_invariants",
        ),
        Index("ix_agent_runs_claim_queue", "status", "next_attempt_at", "lease_expires_at"),
        Index(
            "uq_agent_runs_capability_identity",
            "owner_user_id",
            "capability",
            "operation_id",
            unique=True,
            postgresql_where=text("capability IS NOT NULL"),
        ),
        Index(
            "uq_agent_runs_legacy_operation_id",
            "operation_id",
            unique=True,
            postgresql_where=text("capability IS NULL"),
        ),
    )

    run_id: Mapped[str] = mapped_column(String(68), primary_key=True)
    owner_user_id: Mapped[str] = mapped_column(String(128), index=True)
    operation_id: Mapped[str] = mapped_column(String(128))
    capability: Mapped[str | None] = mapped_column(String(32), index=True)
    recipe_id: Mapped[str | None] = mapped_column(String(64))
    recipe_version: Mapped[str | None] = mapped_column(String(32))
    input_schema_version: Mapped[str | None] = mapped_column(String(32))
    input_payload: Mapped[dict[str, object] | None] = mapped_column(JSONB)
    input_digest: Mapped[str | None] = mapped_column(String(64))
    locale: Mapped[str | None] = mapped_column(String(32))
    initial_message: Mapped[str | None] = mapped_column(Text)
    article_id: Mapped[str | None] = mapped_column(String(128))
    task_type: Mapped[str] = mapped_column(String(32))
    runtime_kind: Mapped[str] = mapped_column(String(16))
    runtime_version: Mapped[str] = mapped_column(String(32))
    revision: Mapped[int] = mapped_column(Integer)
    status: Mapped[str] = mapped_column(String(24))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=text("CURRENT_TIMESTAMP")
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=text("CURRENT_TIMESTAMP")
    )
    attempt: Mapped[int] = mapped_column(Integer, server_default="0")
    next_attempt_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    lease_owner: Mapped[str | None] = mapped_column(String(68))
    lease_token: Mapped[UUID | None] = mapped_column(PGUUID(as_uuid=True))
    lease_expires_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    # 取消请求不改变执行所有权, 由当前围栏持有者观察并提交终态.
    cancellation_requested_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    pending_interaction_id: Mapped[str | None] = mapped_column(String(68))
    terminal_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    terminal_metadata: Mapped[dict[str, str] | None] = mapped_column(
        JSONB, comment="sanitized typed serializer output only; no secrets"
    )
    error_code: Mapped[str | None] = mapped_column(String(64))
    error_message: Mapped[str | None] = mapped_column(Text)
    error_metadata: Mapped[dict[str, str] | None] = mapped_column(
        JSONB, comment="sanitized typed serializer output only; no secrets"
    )


@final
class InteractionRecord(Base):
    """Persist one-time user interaction resolution state."""

    __tablename__ = "agent_interactions"
    __table_args__: tuple[SchemaItem, ...] = (
        CheckConstraint("status IN ('pending','resolved')", name="status_valid"),
        CheckConstraint(
            INTERACTION_RESOLUTION_CHECK,
            name="resolution_invariants",
        ),
        Index("ix_agent_interactions_run_id_status", "run_id", "status"),
        Index(
            "uq_agent_interactions_pending_run",
            "run_id",
            unique=True,
            postgresql_where=text("status = 'pending'"),
        ),
    )

    interaction_id: Mapped[str] = mapped_column(String(68), primary_key=True)
    run_id: Mapped[str] = mapped_column(ForeignKey("agent_runs.run_id", ondelete="CASCADE"))
    kind: Mapped[str] = mapped_column(String(48))
    status: Mapped[str] = mapped_column(String(16))
    requested_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    request_semantic_digest: Mapped[str] = mapped_column(String(64))
    resolved_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    response_digest: Mapped[str | None] = mapped_column(String(128))
    resolution_semantic_digest: Mapped[str | None] = mapped_column(String(64))
    result_reference: Mapped[str | None] = mapped_column(String(256))


@final
class AgentRunResultRecord(Base):
    """Persist one validated terminal result separately from mutable Run state."""

    __tablename__ = "agent_run_results"
    __table_args__: tuple[SchemaItem, ...] = (Index("ix_agent_run_results_run_id", "run_id"),)

    run_id: Mapped[str] = mapped_column(
        ForeignKey("agent_runs.run_id", ondelete="CASCADE"), primary_key=True
    )
    schema_version: Mapped[str] = mapped_column(String(32))
    capability: Mapped[str] = mapped_column(String(32))
    result_payload: Mapped[dict[str, object]] = mapped_column(JSONB)
    result_digest: Mapped[str] = mapped_column(String(64))
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), server_default=text("CURRENT_TIMESTAMP")
    )
