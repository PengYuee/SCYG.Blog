"""Event and command journal SQLAlchemy records."""

from datetime import datetime
from typing import Final, final

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
from sqlalchemy.orm import Mapped, mapped_column
from sqlalchemy.schema import SchemaItem

from .metadata import Base

COMMAND_PENDING_CHECK: Final = (
    "(result_status = 'pending' AND completed_at IS NULL AND result_reference IS NULL)"
)
COMMAND_COMPLETED_CHECK: Final = (
    "(result_status <> 'pending' AND completed_at IS NOT NULL AND result_reference IS NOT NULL)"
)


@final
class EventRecord(Base):
    """Persist immutable replay facts in run sequence order."""

    __tablename__ = "agent_events"
    __table_args__: tuple[SchemaItem, ...] = (
        UniqueConstraint("run_id", "seq"),
        CheckConstraint("seq >= 0", name="seq_nonnegative"),
        CheckConstraint("revision >= 1", name="revision_positive"),
        Index("ix_agent_events_run_id_seq", "run_id", "seq"),
    )

    event_id: Mapped[str] = mapped_column(String(68), primary_key=True)
    run_id: Mapped[str] = mapped_column(ForeignKey("agent_runs.run_id", ondelete="CASCADE"))
    seq: Mapped[int] = mapped_column(Integer)
    revision: Mapped[int] = mapped_column(Integer)
    kind: Mapped[str] = mapped_column(String(64))
    occurred_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    payload: Mapped[dict[str, str]] = mapped_column(
        JSONB, comment="sanitized typed serializer output only; no secrets"
    )


@final
class CommandRecord(Base):
    """Persist idempotent command requests and outcomes."""

    __tablename__ = "agent_commands"
    __table_args__: tuple[SchemaItem, ...] = (
        CheckConstraint("expected_revision >= 0", name="expected_revision_nonnegative"),
        CheckConstraint("sequence >= 0", name="sequence_nonnegative"),
        CheckConstraint(
            "result_status IN ('pending','succeeded','rejected','failed')",
            name="result_status_valid",
        ),
        CheckConstraint(
            f"{COMMAND_PENDING_CHECK} OR {COMMAND_COMPLETED_CHECK}",
            name="result_completion_invariants",
        ),
        Index("ix_agent_commands_run_id_sequence", "run_id", "sequence"),
    )

    command_id: Mapped[str] = mapped_column(String(68), primary_key=True)
    run_id: Mapped[str] = mapped_column(ForeignKey("agent_runs.run_id", ondelete="CASCADE"))
    expected_revision: Mapped[int] = mapped_column(Integer)
    sequence: Mapped[int] = mapped_column(Integer)
    kind: Mapped[str] = mapped_column(String(64))
    request_digest: Mapped[str] = mapped_column(
        String(128), comment="sanitized request digest; never raw request data"
    )
    semantic_digest: Mapped[str] = mapped_column(
        String(64), comment="canonical immutable command semantics"
    )
    result_status: Mapped[str] = mapped_column(String(16))
    result_reference: Mapped[str | None] = mapped_column(String(256))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    completed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
