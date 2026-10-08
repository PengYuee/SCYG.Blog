"""本机 PostgreSQL 的一次性 Agent 数据库初始化."""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, override
from urllib.parse import unquote, urlsplit, urlunsplit

import anyio
from alembic import command
from alembic.config import Config
from psycopg import AsyncConnection, sql
from psycopg import Error as PsycopgError
from pydantic import SecretStr

from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.adapters.langgraph.values import CheckpointerConfig

if TYPE_CHECKING:
    from scyg_agent.config import ApplicationSettings

AGENT_ROLE = "scyg_agent"
CHECKPOINT_SCHEMA = "langgraph"


@dataclass(frozen=True, slots=True)
class DatabaseIdentity:
    """保存从运行时 DSN 提取的非敏感连接身份和角色密码."""

    host: str
    port: int
    database: str
    role: str
    password: str


class LocalSetupError(RuntimeError):
    """表示本机数据库初始化输入或执行失败."""


@dataclass(frozen=True, slots=True)
class LocalSetupStageError(RuntimeError):
    """保存失败阶段和非敏感驱动诊断."""

    stage: str
    error_type: str
    sqlstate: str | None = None

    @override
    def __str__(self) -> str:
        """返回不含凭据的阶段诊断."""
        suffix = f" (SQLSTATE {self.sqlstate})" if self.sqlstate else ""
        return f"stage={self.stage}; error={self.error_type}{suffix}"


def _sqlstate(error: BaseException) -> str | None:
    """从驱动错误或 SQLAlchemy 包装错误提取 SQLSTATE."""
    candidates: tuple[BaseException | None, ...] = (
        error,
        getattr(error, "orig", None),
        error.__cause__,
    )
    for candidate in candidates:
        if isinstance(candidate, PsycopgError) and candidate.sqlstate:
            return candidate.sqlstate
        state = getattr(candidate, "sqlstate", None)
        if isinstance(state, str) and state:
            return state
    return None


def _stage_error(stage: str, error: BaseException) -> LocalSetupStageError:
    return LocalSetupStageError(stage, type(error).__name__, _sqlstate(error))


def _identity(raw_url: str, expected_role: str) -> DatabaseIdentity:
    parsed = urlsplit(raw_url.replace("postgresql+asyncpg://", "postgresql://", 1))
    database = parsed.path.removeprefix("/")
    if (
        parsed.scheme != "postgresql"
        or parsed.username != expected_role
        or parsed.password is None
        or parsed.hostname is None
        or not database
        or parsed.query
        or parsed.fragment
    ):
        raise LocalSetupError
    return DatabaseIdentity(
        parsed.hostname,
        parsed.port or 5432,
        database,
        expected_role,
        unquote(parsed.password),
    )


def default_admin_url(settings: ApplicationSettings) -> str:
    """从 Agent truth 地址推导不含密码的本机管理员连接."""
    identity = _identity(settings.database_url.get_secret_value(), AGENT_ROLE)
    return f"postgresql://postgres@{identity.host}:{identity.port}/postgres"


def _admin_target_url(admin_url: str, database: str) -> str:
    parsed = urlsplit(admin_url)
    if parsed.scheme != "postgresql" or parsed.hostname is None or parsed.username is None:
        raise LocalSetupError
    return urlunsplit(parsed._replace(path=f"/{database}"))


async def _ensure_role(connection: AsyncConnection[object], role: str, password: str) -> None:
    result = await connection.execute("SELECT 1 FROM pg_roles WHERE rolname = %s", (role,))
    if await result.fetchone() is None:
        _ = await connection.execute(sql.SQL("CREATE ROLE {} LOGIN").format(sql.Identifier(role)))
    _ = await connection.execute(
        sql.SQL(
            """ALTER ROLE {} LOGIN PASSWORD {} NOSUPERUSER NOCREATEDB NOCREATEROLE
            NOINHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 20"""
        ).format(sql.Identifier(role), sql.Literal(password))
    )


async def _prepare_database(
    settings: ApplicationSettings, admin_url: str, admin_password: str
) -> None:
    agent = _identity(settings.database_url.get_secret_value(), AGENT_ROLE)

    maintenance = await AsyncConnection.connect(
        admin_url, password=admin_password or None, autocommit=True
    )
    try:
        await _ensure_role(maintenance, agent.role, agent.password)
        result = await maintenance.execute(
            "SELECT 1 FROM pg_database WHERE datname = %s", (agent.database,)
        )
        if await result.fetchone() is None:
            _ = await maintenance.execute(
                sql.SQL("CREATE DATABASE {}").format(sql.Identifier(agent.database))
            )
    finally:
        await maintenance.close()

    target = await AsyncConnection.connect(
        _admin_target_url(admin_url, agent.database),
        password=admin_password or None,
        autocommit=True,
    )
    try:
        _ = await target.execute(
            sql.SQL("GRANT CONNECT ON DATABASE {} TO {}").format(
                sql.Identifier(agent.database), sql.Identifier(agent.role)
            )
        )
        _ = await target.execute(
            sql.SQL("GRANT USAGE, CREATE ON SCHEMA public TO {}").format(sql.Identifier(agent.role))
        )
        _ = await target.execute(
            sql.SQL("CREATE SCHEMA IF NOT EXISTS {} AUTHORIZATION {}").format(
                sql.Identifier(CHECKPOINT_SCHEMA), sql.Identifier(agent.role)
            )
        )
        _ = await target.execute(
            sql.SQL("ALTER SCHEMA {} OWNER TO {}").format(
                sql.Identifier(CHECKPOINT_SCHEMA), sql.Identifier(agent.role)
            )
        )
        _ = await target.execute(
            sql.SQL("GRANT USAGE, CREATE ON SCHEMA {} TO {}").format(
                sql.Identifier(CHECKPOINT_SCHEMA), sql.Identifier(agent.role)
            )
        )
        _ = await target.execute(
            sql.SQL("ALTER ROLE {} IN DATABASE {} SET search_path = public, pg_catalog").format(
                sql.Identifier(agent.role), sql.Identifier(agent.database)
            )
        )
    finally:
        await target.close()


def _run_migrations(settings: ApplicationSettings) -> None:
    previous_url = os.environ.get("SCYG_AGENT_DATABASE_URL")
    os.environ["SCYG_AGENT_DATABASE_URL"] = settings.database_url.get_secret_value()
    try:
        command.upgrade(Config(Path(__file__).with_name("alembic.ini")), "head")
    finally:
        if previous_url is None:
            del os.environ["SCYG_AGENT_DATABASE_URL"]
        else:
            os.environ["SCYG_AGENT_DATABASE_URL"] = previous_url


async def _setup_checkpoint(settings: ApplicationSettings) -> None:
    checkpoint_dsn = SecretStr(
        settings.database_url.get_secret_value().replace(
            "postgresql+asyncpg://", "postgresql://", 1
        )
    )
    async with CheckpointStore(CheckpointerConfig(checkpoint_dsn)) as store:
        await store.setup()
        _ = await store.readiness()


def setup_local_database(
    settings: ApplicationSettings, admin_password: str, admin_url: str | None = None
) -> None:
    """创建本机数据库边界、执行迁移并初始化 checkpoint."""
    stage = "database"
    try:
        resolved_admin_url = admin_url or default_admin_url(settings)
        anyio.run(_prepare_database, settings, resolved_admin_url, admin_password)
    except Exception as error:  # noqa: BLE001, RUF100  # noqa: BROAD_EXCEPT_OK - 每个 setup 阶段都要转换为不含凭据的诊断。
        raise _stage_error(stage, error) from error
    stage = "migration"
    try:
        _run_migrations(settings)
    except Exception as error:  # noqa: BLE001, RUF100  # noqa: BROAD_EXCEPT_OK - 每个 setup 阶段都要转换为不含凭据的诊断。
        raise _stage_error(stage, error) from error
    stage = "checkpoint"
    try:
        anyio.run(_setup_checkpoint, settings)
    except Exception as error:  # noqa: BLE001, RUF100  # noqa: BROAD_EXCEPT_OK - 每个 setup 阶段都要转换为不含凭据的诊断。
        raise _stage_error(stage, error) from error
