"""Revision 04 fresh and historical schema compatibility tests."""

from datetime import datetime
from importlib import import_module
from typing import Protocol, runtime_checkable

import pytest
from sqlalchemy import Column, DateTime
from sqlalchemy.engine.interfaces import ReflectedColumn

REVISION_MODULE = "migrations.versions.20260712_04_t19_prerequisite_contracts"


@runtime_checkable
class Revision04(Protocol):
    """Narrow callable surface of the imported Alembic revision."""

    COLUMN_NAME: str
    OWNERSHIP_COMMENT: str
    OFFLINE_UPGRADE_SQL: str
    OFFLINE_DOWNGRADE_SQL: str

    def upgrade_column(self, column: ReflectedColumn | None) -> None: ...

    def downgrade_column(self, column: ReflectedColumn | None) -> None: ...


def _revision() -> Revision04:
    module = import_module(REVISION_MODULE)
    assert isinstance(module, Revision04)
    return module


def _column(name: str, comment: str | None = None) -> ReflectedColumn:
    return ReflectedColumn(
        name=name,
        type=DateTime(timezone=True),
        nullable=True,
        default=None,
        comment=comment,
    )


def test_historical_schema_adds_owned_nullable_column_once(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    revision = _revision()
    added: list[tuple[str, Column[datetime]]] = []

    def add_column(table: str, column: Column[datetime]) -> None:
        added.append((table, column))

    monkeypatch.setattr(f"{REVISION_MODULE}.op.add_column", add_column)
    revision.upgrade_column(None)
    revision.upgrade_column(_column(revision.COLUMN_NAME, revision.OWNERSHIP_COMMENT))

    assert len(added) == 1
    assert added[0][0] == "agent_runs"
    assert added[0][1].name == revision.COLUMN_NAME
    assert added[0][1].nullable
    assert added[0][1].comment == revision.OWNERSHIP_COMMENT


def test_fresh_metadata_column_is_never_claimed_or_added_by_revision(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    revision = _revision()
    added: list[tuple[str, Column[datetime]]] = []

    def add_column(table: str, column: Column[datetime]) -> None:
        added.append((table, column))

    monkeypatch.setattr(f"{REVISION_MODULE}.op.add_column", add_column)
    existing = _column(revision.COLUMN_NAME)
    revision.upgrade_column(existing)
    revision.upgrade_column(existing)

    assert added == []


@pytest.mark.parametrize(
    ("comment", "expected_count"),
    [(None, 0), ("owned by alembic revision 20260712_04", 1)],
)
def test_downgrade_drops_only_revision_owned_column(
    monkeypatch: pytest.MonkeyPatch,
    comment: str | None,
    expected_count: int,
) -> None:
    revision = _revision()
    dropped: list[tuple[str, str]] = []

    def drop_column(table: str, column: str) -> None:
        dropped.append((table, column))

    monkeypatch.setattr(f"{REVISION_MODULE}.op.drop_column", drop_column)
    revision.downgrade_column(_column(revision.COLUMN_NAME, comment))

    assert len(dropped) == expected_count


def test_offline_blocks_are_state_conditional_and_ownership_aware() -> None:
    revision = _revision()

    assert "IF NOT EXISTS" in revision.OFFLINE_UPGRADE_SQL
    assert revision.OWNERSHIP_COMMENT in revision.OFFLINE_UPGRADE_SQL
    assert "col_description" in revision.OFFLINE_DOWNGRADE_SQL
    assert revision.OWNERSHIP_COMMENT in revision.OFFLINE_DOWNGRADE_SQL
