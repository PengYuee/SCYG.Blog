"""Revision 05 fresh 与历史 schema 所有权测试。"""

from importlib import import_module
from typing import Protocol, runtime_checkable

import pytest
from sqlalchemy import Column, String
from sqlalchemy.engine.interfaces import ReflectedColumn

REVISION_MODULE = "migrations.versions.20260712_05_t20_run_input"


@runtime_checkable
class Revision05(Protocol):
    """限定 revision 05 可测试表面。"""

    ARTICLE_COLUMN: str
    MESSAGE_COLUMN: str
    OWNERSHIP_COMMENT: str
    OFFLINE_UPGRADE_SQL: str
    OFFLINE_DOWNGRADE_SQL: str

    def upgrade_columns(self, columns: dict[str, ReflectedColumn]) -> None: ...

    def downgrade_columns(self, columns: dict[str, ReflectedColumn]) -> None: ...


def _revision() -> Revision05:
    module = import_module(REVISION_MODULE)
    assert isinstance(module, Revision05)
    return module


def _column(name: str, comment: str | None = None) -> ReflectedColumn:
    return ReflectedColumn(
        name=name,
        type=String(),
        nullable=True,
        default=None,
        comment=comment,
    )


def test_historical_schema_adds_two_owned_nullable_columns(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given
    revision = _revision()
    added: list[Column[str]] = []

    def add_column(table: str, column: Column[str]) -> None:
        del table
        added.append(column)

    monkeypatch.setattr(f"{REVISION_MODULE}.op.add_column", add_column)

    # When
    revision.upgrade_columns({})

    # Then
    assert {column.name for column in added} == {
        revision.MESSAGE_COLUMN,
        revision.ARTICLE_COLUMN,
    }
    assert all(column.nullable for column in added)
    assert all(column.comment == revision.OWNERSHIP_COMMENT for column in added)


def test_fresh_columns_are_not_added_or_removed(monkeypatch: pytest.MonkeyPatch) -> None:
    # Given
    revision = _revision()
    columns = {
        revision.MESSAGE_COLUMN: _column(revision.MESSAGE_COLUMN),
        revision.ARTICLE_COLUMN: _column(revision.ARTICLE_COLUMN),
    }
    changed: list[str] = []

    def add_column(table: str, column: Column[str]) -> None:
        del table, column
        changed.append("add")

    def drop_column(table: str, name: str) -> None:
        del table, name
        changed.append("drop")

    monkeypatch.setattr(f"{REVISION_MODULE}.op.add_column", add_column)
    monkeypatch.setattr(f"{REVISION_MODULE}.op.drop_column", drop_column)

    # When
    revision.upgrade_columns(columns)
    revision.downgrade_columns(columns)

    # Then
    assert changed == []


def test_downgrade_removes_only_revision_owned_columns(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given
    revision = _revision()
    dropped: list[str] = []

    def drop_column(table: str, name: str) -> None:
        del table
        dropped.append(name)

    monkeypatch.setattr(f"{REVISION_MODULE}.op.drop_column", drop_column)
    columns = {
        revision.MESSAGE_COLUMN: _column(revision.MESSAGE_COLUMN, revision.OWNERSHIP_COMMENT),
        revision.ARTICLE_COLUMN: _column(revision.ARTICLE_COLUMN, revision.OWNERSHIP_COMMENT),
    }

    # When
    revision.downgrade_columns(columns)

    # Then
    assert dropped == [revision.ARTICLE_COLUMN, revision.MESSAGE_COLUMN]


def test_offline_sql_is_conditional_and_comment_owned() -> None:
    revision = _revision()
    assert revision.OFFLINE_UPGRADE_SQL.count("IF NOT EXISTS") == 2
    assert revision.OFFLINE_DOWNGRADE_SQL.count("col_description") == 2
    assert revision.OWNERSHIP_COMMENT in revision.OFFLINE_UPGRADE_SQL
    assert revision.OWNERSHIP_COMMENT in revision.OFFLINE_DOWNGRADE_SQL
