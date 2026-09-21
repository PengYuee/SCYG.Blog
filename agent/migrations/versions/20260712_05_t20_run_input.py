"""Persist nullable historical Run input columns.

Revision ID: 20260712_05
Revises: 20260712_04
"""

from collections.abc import Sequence
from typing import Final

from alembic import context, op
from sqlalchemy import Column, String, Text, inspect
from sqlalchemy.engine.interfaces import ReflectedColumn

revision: str = "20260712_05"
down_revision: str | None = "20260712_04"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

TABLE_NAME: Final = "agent_runs"
MESSAGE_COLUMN: Final = "initial_message"
ARTICLE_COLUMN: Final = "article_id"
OWNERSHIP_COMMENT: Final = "owned by alembic revision 20260712_05"
OFFLINE_UPGRADE_SQL: Final = """DO $$ BEGIN
IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'agent_runs'
      AND column_name = 'initial_message'
) THEN
    ALTER TABLE agent_runs ADD COLUMN initial_message TEXT;
    COMMENT ON COLUMN agent_runs.initial_message
    IS 'owned by alembic revision 20260712_05';
END IF;
IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'agent_runs'
      AND column_name = 'article_id'
) THEN
    ALTER TABLE agent_runs ADD COLUMN article_id VARCHAR(128);
    COMMENT ON COLUMN agent_runs.article_id
    IS 'owned by alembic revision 20260712_05';
END IF;
END $$"""
OFFLINE_DOWNGRADE_SQL: Final = """DO $$ BEGIN
IF EXISTS (
    SELECT 1 FROM pg_attribute attribute
    JOIN pg_class relation ON relation.oid = attribute.attrelid
    WHERE relation.relname = 'agent_runs'
      AND attribute.attname = 'article_id'
      AND col_description(relation.oid, attribute.attnum)
          = 'owned by alembic revision 20260712_05'
) THEN ALTER TABLE agent_runs DROP COLUMN article_id; END IF;
IF EXISTS (
    SELECT 1 FROM pg_attribute attribute
    JOIN pg_class relation ON relation.oid = attribute.attrelid
    WHERE relation.relname = 'agent_runs'
      AND attribute.attname = 'initial_message'
      AND col_description(relation.oid, attribute.attnum)
          = 'owned by alembic revision 20260712_05'
) THEN ALTER TABLE agent_runs DROP COLUMN initial_message; END IF;
END $$"""


def _columns() -> dict[str, ReflectedColumn]:
    """按列名读取当前 Agent Run 元数据。."""
    return {column["name"]: column for column in inspect(op.get_bind()).get_columns(TABLE_NAME)}


def upgrade_columns(columns: dict[str, ReflectedColumn]) -> None:
    """只添加历史 schema 缺失的可空列并标记所有权。."""
    if MESSAGE_COLUMN not in columns:
        op.add_column(
            TABLE_NAME,
            Column(MESSAGE_COLUMN, Text(), nullable=True, comment=OWNERSHIP_COMMENT),
        )
    if ARTICLE_COLUMN not in columns:
        op.add_column(
            TABLE_NAME,
            Column(ARTICLE_COLUMN, String(128), nullable=True, comment=OWNERSHIP_COMMENT),
        )


def downgrade_columns(columns: dict[str, ReflectedColumn]) -> None:
    """只删除由本 revision 实际添加并标记的列。."""
    for name in (ARTICLE_COLUMN, MESSAGE_COLUMN):
        column = columns.get(name)
        if column is not None and column.get("comment") == OWNERSHIP_COMMENT:
            op.drop_column(TABLE_NAME, name)


def upgrade() -> None:
    """兼容 fresh metadata 与历史 revision 04 schema。."""
    if context.is_offline_mode():
        op.execute(OFFLINE_UPGRADE_SQL)
        return
    upgrade_columns(_columns())


def downgrade() -> None:
    """保留 fresh-owned 列并逆序删除 revision-owned 列。."""
    if context.is_offline_mode():
        op.execute(OFFLINE_DOWNGRADE_SQL)
        return
    downgrade_columns(_columns())
