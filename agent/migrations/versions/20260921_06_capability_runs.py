"""Add capability input snapshots and terminal result storage.

Revision ID: 20260921_06
Revises: 20260712_05
"""

from collections.abc import Sequence
from typing import Final

from alembic import context, op
from sqlalchemy import Column, DateTime, ForeignKey, MetaData, String, Table, text
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.engine import Connection
from sqlalchemy.engine.reflection import Inspector

revision: str = "20260921_06"
down_revision: str | None = "20260712_05"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

RUN_COLUMNS: Final = {
    "capability": String(32),
    "recipe_id": String(64),
    "recipe_version": String(32),
    "input_schema_version": String(32),
    "input_payload": JSONB,
    "input_digest": String(64),
    "locale": String(32),
}


def _inspector() -> Inspector:
    """Inspect the connected database without assuming a fresh schema."""
    return Inspector.from_engine(op.get_bind())


def _add_run_columns() -> None:
    """Add nullable snapshot columns so historical rows remain readable."""
    existing = {column["name"] for column in _inspector().get_columns("agent_runs")}
    for name, type_ in RUN_COLUMNS.items():
        if name not in existing:
            op.add_column(
                "agent_runs", Column(name, type_, nullable=True, comment="owned by 20260921_06")
            )


def _create_result_table(connection: Connection) -> None:
    """Create the result table only when it is absent."""
    if "agent_run_results" in _inspector().get_table_names():
        return
    metadata = MetaData()
    _ = Table("agent_runs", metadata, autoload_with=connection)
    result_table = Table(
        "agent_run_results",
        metadata,
        Column(
            "run_id",
            String(68),
            ForeignKey("agent_runs.run_id", ondelete="CASCADE"),
            primary_key=True,
        ),
        Column("schema_version", String(32), nullable=False),
        Column("capability", String(32), nullable=False),
        Column("result_payload", JSONB, nullable=False),
        Column("result_digest", String(64), nullable=False),
        Column(
            "created_at",
            DateTime(timezone=True),
            server_default=text("CURRENT_TIMESTAMP"),
            nullable=False,
        ),
    )
    metadata.create_all(connection, tables=[result_table], checkfirst=True)


def upgrade() -> None:
    """Additive, retryable migration that never deletes existing Run data."""
    if context.is_offline_mode():
        for name, type_ in RUN_COLUMNS.items():
            sql_type = "JSONB" if type_ is JSONB else f"VARCHAR({type_.length})"
            op.execute(f"ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS {name} {sql_type}")
        op.execute(
            """CREATE TABLE IF NOT EXISTS agent_run_results (
                run_id VARCHAR(68) PRIMARY KEY REFERENCES agent_runs(run_id) ON DELETE CASCADE,
                schema_version VARCHAR(32) NOT NULL,
                capability VARCHAR(32) NOT NULL,
                result_payload JSONB NOT NULL,
                result_digest VARCHAR(64) NOT NULL,
                created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
            )"""
        )
        return
    connection = op.get_bind()
    _add_run_columns()
    _create_result_table(connection)


def downgrade() -> None:
    """Drop only objects owned by this migration, preserving historical Run rows."""
    if context.is_offline_mode():
        op.execute("DROP TABLE IF EXISTS agent_run_results")
        for name in reversed(tuple(RUN_COLUMNS)):
            op.execute(f"ALTER TABLE agent_runs DROP COLUMN IF EXISTS {name}")
        return
    op.drop_table("agent_run_results", if_exists=True)
    existing = {column["name"] for column in _inspector().get_columns("agent_runs")}
    for name in reversed(tuple(RUN_COLUMNS)):
        if name in existing:
            op.drop_column("agent_runs", name)
