"""Endpoint-guarded PostgreSQL acceptance for revision 04 schema histories."""

import subprocess
import sys
from collections.abc import Awaitable, Callable, Mapping
from pathlib import Path

import anyio
from sqlalchemy import Connection, Inspector, func, inspect, select, text
from sqlalchemy.ext.asyncio import AsyncEngine, create_async_engine

from scyg_agent.adapters.database.run_records import RunRecord
from tests.acceptance_settings import require_test_settings

AGENT_ROOT = Path(__file__).parents[3]
COLUMN_NAME = "cancellation_requested_at"
OWNERSHIP_COMMENT = "owned by alembic revision 20260712_04"


def _environment() -> dict[str, str]:
    return require_test_settings().child_environment("migration")


def _alembic(arguments: list[str], environment: Mapping[str, str]) -> None:
    _ = subprocess.run(  # noqa: S603
        [sys.executable, "-m", "alembic", *arguments],
        cwd=AGENT_ROOT,
        env=environment,
        check=True,
        capture_output=True,
        text=True,
    )


def _alembic_failure(
    arguments: list[str], environment: Mapping[str, str]
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(  # noqa: S603
        [sys.executable, "-m", "alembic", *arguments],
        cwd=AGENT_ROOT,
        env=environment,
        check=False,
        capture_output=True,
        text=True,
    )


async def _phase[T](
    environment: Mapping[str, str], operation: Callable[[AsyncEngine], Awaitable[T]]
) -> T:
    """在一个事件循环内创建、使用并销毁独占 engine."""
    engine = create_async_engine(
        environment["SCYG_AGENT_DATABASE_URL"], pool_size=1, max_overflow=0
    )
    try:
        return await operation(engine)
    finally:
        await engine.dispose()


async def _column_state(engine: AsyncEngine) -> tuple[int, str | None]:
    async with engine.connect() as connection:
        return await connection.run_sync(_sync_column_state)


def _sync_column_state(connection: Connection) -> tuple[int, str | None]:
    inspector = inspect(connection)
    assert isinstance(inspector, Inspector)
    columns = inspector.get_columns("agent_runs")
    matches = [column for column in columns if column["name"] == COLUMN_NAME]
    return len(matches), matches[0].get("comment") if matches else None


def test_fresh_current_metadata_upgrade_head_is_stable() -> None:
    environment = _environment()
    _alembic(["downgrade", "base"], environment)
    try:
        _alembic(["upgrade", "head"], environment)
        _alembic(["upgrade", "head"], environment)
        count, comment = anyio.run(_phase, environment, _column_state)
        assert count == 1
        assert comment != OWNERSHIP_COMMENT
    finally:
        _alembic(["downgrade", "base"], environment)


def test_historical_revision03_cycle_preserves_rows_and_null_semantics() -> None:
    environment = _environment()
    _alembic(["downgrade", "base"], environment)
    seeded = False
    try:
        _alembic(["upgrade", "20260712_03"], environment)
        anyio.run(_phase, environment, _prepare_historical_row)
        seeded = True
        _alembic(["upgrade", "head"], environment)
        count, comment = anyio.run(_phase, environment, _column_state)
        is_null = anyio.run(_phase, environment, _cancellation_is_null)
        assert (count, comment, is_null) == (1, OWNERSHIP_COMMENT, True)
        _alembic(["downgrade", "20260712_03"], environment)
        count, _ = anyio.run(_phase, environment, _column_state)
        row_count = anyio.run(_phase, environment, _run_count)
        assert count == 0
        assert row_count == 1
        _alembic(["upgrade", "head"], environment)
        is_null = anyio.run(_phase, environment, _cancellation_is_null)
        assert is_null
    finally:
        if seeded:
            anyio.run(_phase, environment, _remove_historical_row)
        _alembic(["downgrade", "base"], environment)


def test_failed_online_migration_closes_resources_without_warnings() -> None:
    environment = _environment()
    _alembic(["downgrade", "base"], environment)
    renamed = False
    try:
        _alembic(["upgrade", "20260712_03"], environment)
        anyio.run(_phase, environment, _hide_run_table)
        renamed = True
        result = _alembic_failure(["upgrade", "head"], environment)
        output = result.stdout + result.stderr
        assert result.returncode != 0
        assert "unclosed" not in output.lower()
        assert "resourcewarning" not in output.lower()
    finally:
        if renamed:
            anyio.run(_phase, environment, _restore_run_table)
        _alembic(["downgrade", "base"], environment)


async def _prepare_historical_row(engine: AsyncEngine) -> None:
    async with engine.begin() as connection:
        _ = await connection.execute(
            text("ALTER TABLE agent_runs DROP COLUMN IF EXISTS cancellation_requested_at")
        )
        _ = await connection.execute(
            text(
                """INSERT INTO agent_runs (
                run_id, owner_user_id, operation_id, task_type, runtime_kind, runtime_version,
                revision, status, created_at, updated_at, attempt, next_attempt_at
) VALUES (
                'run_migration01', 'user-migration', 'migration:01', 'summary', 'simple',
                'v1', 1, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP
)"""
            )
        )


async def _remove_historical_row(engine: AsyncEngine) -> None:
    """Delete only this fixture's historical row before returning to base."""
    async with engine.begin() as connection:
        _ = await connection.execute(
            text("DELETE FROM agent_runs WHERE run_id = :run_id"),
            {"run_id": "run_migration01"},
        )


async def _cancellation_is_null(engine: AsyncEngine) -> bool:
    async with engine.connect() as connection:
        value = (
            await connection.execute(
                select(RunRecord.cancellation_requested_at).where(
                    RunRecord.run_id == "run_migration01"
                )
            )
        ).scalar_one()
        return value is None


async def _run_count(engine: AsyncEngine) -> int:
    async with engine.connect() as connection:
        return (
            await connection.execute(
                select(func.count())
                .select_from(RunRecord)
                .where(RunRecord.run_id == "run_migration01")
            )
        ).scalar_one()


async def _hide_run_table(engine: AsyncEngine) -> None:
    async with engine.begin() as connection:
        _ = await connection.execute(text("ALTER TABLE agent_runs RENAME TO agent_runs_hidden"))


async def _restore_run_table(engine: AsyncEngine) -> None:
    async with engine.begin() as connection:
        _ = await connection.execute(text("ALTER TABLE agent_runs_hidden RENAME TO agent_runs"))
