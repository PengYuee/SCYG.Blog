"""Add immutable Recipe, thread, quality, and checkpoint binding identity."""

from collections.abc import Sequence
from typing import Final

from alembic import context, op
from sqlalchemy import Column, DateTime, ForeignKey, MetaData, String, Table, text
from sqlalchemy.engine.reflection import Inspector

revision: str = "20260922_08"
down_revision: str | None = "20260921_07"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

RUN_COLUMNS: Final = {
    "thread_id": String(128),
    "quality": String(16),
    "state_schema_version": String(32),
    "result_reference": String(256),
}


def _inspector() -> Inspector:
    return Inspector.from_engine(op.get_bind())


def _create_binding_table() -> None:
    metadata = MetaData()
    table = Table(
        "agent_checkpoint_bindings",
        metadata,
        Column("thread_id", String(128), primary_key=True),
        Column(
            "run_id",
            String(68),
            ForeignKey("agent_runs.run_id", ondelete="CASCADE"),
            nullable=False,
        ),
        Column("recipe_id", String(64), nullable=False),
        Column("recipe_version", String(32), nullable=False),
        Column("state_schema_version", String(32), nullable=False),
        Column(
            "created_at",
            DateTime(timezone=True),
            server_default=text("CURRENT_TIMESTAMP"),
            nullable=False,
        ),
    )
    table.create(op.get_bind(), checkfirst=True)
    if "uq_agent_checkpoint_bindings_run_id" not in {
        index["name"] for index in _inspector().get_indexes("agent_checkpoint_bindings")
    }:
        op.create_index(
            "uq_agent_checkpoint_bindings_run_id",
            "agent_checkpoint_bindings",
            ["run_id"],
            unique=True,
        )


def upgrade() -> None:
    """Add new identity columns and a retry-safe checkpoint binding table."""
    if context.is_offline_mode():
        for name, column_type in RUN_COLUMNS.items():
            op.execute(
                f"ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS {name} "
                f"VARCHAR({column_type.length})"
            )
        op.execute(
            """CREATE TABLE IF NOT EXISTS agent_checkpoint_bindings (
                thread_id VARCHAR(128) PRIMARY KEY,
                run_id VARCHAR(68) NOT NULL REFERENCES agent_runs(run_id) ON DELETE CASCADE,
                recipe_id VARCHAR(64) NOT NULL,
                recipe_version VARCHAR(32) NOT NULL,
                state_schema_version VARCHAR(32) NOT NULL,
                created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
            )"""
        )
        op.execute(
            "CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_checkpoint_bindings_run_id "
            "ON agent_checkpoint_bindings (run_id)"
        )
        return
    existing = {column["name"] for column in _inspector().get_columns("agent_runs")}
    for name, column_type in RUN_COLUMNS.items():
        if name not in existing:
            op.add_column("agent_runs", Column(name, column_type))
    _create_binding_table()


def downgrade() -> None:
    """Remove only objects introduced by this revision."""
    op.drop_index("uq_agent_checkpoint_bindings_run_id", table_name="agent_checkpoint_bindings")
    op.drop_table("agent_checkpoint_bindings")
    for name in reversed(tuple(RUN_COLUMNS)):
        op.drop_column("agent_runs", name)
