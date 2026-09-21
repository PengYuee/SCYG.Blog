"""Add observable cancellation requests for T19 prerequisites.

Revision ID: 20260712_04
Revises: 20260712_03
"""

from collections.abc import Sequence
from typing import Final

from alembic import context, op
from sqlalchemy import Column, DateTime, inspect
from sqlalchemy.engine.interfaces import ReflectedColumn

revision: str = "20260712_04"
down_revision: str | None = "20260712_03"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

COLUMN_NAME: Final = "cancellation_requested_at"
OWNERSHIP_COMMENT: Final = "owned by alembic revision 20260712_04"
TABLE_NAME: Final = "agent_runs"
OFFLINE_UPGRADE_SQL: Final = """DO $$ BEGIN
IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'agent_runs'
      AND column_name = 'cancellation_requested_at'
) THEN
    ALTER TABLE agent_runs ADD COLUMN cancellation_requested_at TIMESTAMPTZ;
    COMMENT ON COLUMN agent_runs.cancellation_requested_at
    IS 'owned by alembic revision 20260712_04';
END IF;
END $$"""
OFFLINE_DOWNGRADE_SQL: Final = """DO $$ BEGIN
IF EXISTS (
    SELECT 1
    FROM pg_attribute attribute
    JOIN pg_class relation ON relation.oid = attribute.attrelid
    JOIN pg_namespace namespace ON namespace.oid = relation.relnamespace
    WHERE namespace.nspname = current_schema()
      AND relation.relname = 'agent_runs'
      AND attribute.attname = 'cancellation_requested_at'
      AND col_description(relation.oid, attribute.attnum)
          = 'owned by alembic revision 20260712_04'
) THEN
    ALTER TABLE agent_runs DROP COLUMN cancellation_requested_at;
END IF;
END $$"""


def _column() -> ReflectedColumn | None:
    """通过 Alembic 当前连接读取精确 PostgreSQL 列元数据."""
    columns = inspect(op.get_bind()).get_columns(TABLE_NAME)
    return next((column for column in columns if column["name"] == COLUMN_NAME), None)


def upgrade_column(column: ReflectedColumn | None) -> None:
    """仅为缺列的历史 schema 新增并标记 revision 所有权."""
    if column is None:
        op.add_column(
            TABLE_NAME,
            Column(COLUMN_NAME, DateTime(timezone=True), nullable=True, comment=OWNERSHIP_COMMENT),
        )


def downgrade_column(column: ReflectedColumn | None) -> None:
    """仅删除由本 revision 新增并标记的列."""
    if column is not None and column.get("comment") == OWNERSHIP_COMMENT:
        op.drop_column(TABLE_NAME, COLUMN_NAME)


def upgrade() -> None:
    """兼容 fresh metadata 与缺列的 historical revision 03 schema."""
    if context.is_offline_mode():
        op.execute(OFFLINE_UPGRADE_SQL)
        return
    upgrade_column(_column())


def downgrade() -> None:
    """按列所有权执行可逆降级."""
    if context.is_offline_mode():
        op.execute(OFFLINE_DOWNGRADE_SQL)
        return
    downgrade_column(_column())
