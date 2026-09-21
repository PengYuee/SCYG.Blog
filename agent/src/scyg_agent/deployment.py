"""本地 Compose PostgreSQL 的最小初始化入口."""

import os
import sys
from pathlib import Path
from typing import Final

import anyio
from alembic import command
from alembic.config import Config
from psycopg import AsyncConnection, sql
from pydantic import SecretStr

from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.adapters.langgraph.values import CheckpointerConfig
from scyg_agent.deployment_contracts import (
    ADMIN_ROLE,
    AGENT_ROLE,
    AGENT_SCHEMA,
    CHECKPOINT_SCHEMA,
    DATABASE_HOST,
    DATABASE_PATH,
    DATABASE_PORT,
    ROLE_SCHEMES,
    DeploymentConfigurationError,
    DeploymentSettings,
    DeploymentSetupError,
)

__all__ = [
    "ADMIN_ROLE",
    "AGENT_ROLE",
    "AGENT_SCHEMA",
    "CHECKPOINT_SCHEMA",
    "DATABASE_HOST",
    "DATABASE_PATH",
    "DATABASE_PORT",
    "ROLE_SCHEMES",
    "DeploymentConfigurationError",
    "DeploymentSettings",
    "DeploymentSetupError",
]

AGENT_ROLE_ATTRIBUTES: Final = (
    "LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT "
    "NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 20"
)


async def _prepare_database(settings: DeploymentSettings) -> None:
    """创建或同步单一应用角色,并准备两个普通 schema."""
    connection = await AsyncConnection.connect(
        settings.admin_database_url.get_secret_value(),
        password=settings.admin_database_password.get_secret_value(),
        autocommit=True,
    )
    try:
        result = await connection.execute(
            "SELECT 1 FROM pg_roles WHERE rolname = %s", (AGENT_ROLE,)
        )
        if await result.fetchone() is None:
            _ = await connection.execute(
                sql.SQL("CREATE ROLE {} LOGIN").format(sql.Identifier(AGENT_ROLE))
            )
        _ = await connection.execute(
            sql.SQL("ALTER ROLE {} {} PASSWORD %s").format(
                sql.Identifier(AGENT_ROLE), sql.SQL(AGENT_ROLE_ATTRIBUTES)
            ),
            (settings.agent_database_password.get_secret_value(),),
        )
        _ = await connection.execute(
            sql.SQL("GRANT CONNECT ON DATABASE {} TO {}").format(
                sql.Identifier("scyg_agent"), sql.Identifier(AGENT_ROLE)
            )
        )
        _ = await connection.execute(
            sql.SQL("GRANT USAGE, CREATE ON SCHEMA {} TO {}").format(
                sql.Identifier(AGENT_SCHEMA), sql.Identifier(AGENT_ROLE)
            )
        )
        _ = await connection.execute(
            sql.SQL("CREATE SCHEMA IF NOT EXISTS {} AUTHORIZATION {}").format(
                sql.Identifier(CHECKPOINT_SCHEMA), sql.Identifier(AGENT_ROLE)
            )
        )
        _ = await connection.execute(
            sql.SQL("ALTER SCHEMA {} OWNER TO {}").format(
                sql.Identifier(CHECKPOINT_SCHEMA), sql.Identifier(AGENT_ROLE)
            )
        )
        _ = await connection.execute(
            sql.SQL("GRANT USAGE, CREATE ON SCHEMA {} TO {}").format(
                sql.Identifier(CHECKPOINT_SCHEMA), sql.Identifier(AGENT_ROLE)
            )
        )
        _ = await connection.execute(
            sql.SQL("ALTER ROLE {} IN DATABASE {} SET search_path = public, pg_catalog").format(
                sql.Identifier(AGENT_ROLE), sql.Identifier("scyg_agent")
            )
        )
    finally:
        await connection.close()


def _run_agent_migrations(settings: DeploymentSettings) -> None:
    """使用单一应用账号执行 Agent truth migration."""
    previous_url = os.environ.get("SCYG_AGENT_DATABASE_URL")
    os.environ["SCYG_AGENT_DATABASE_URL"] = settings.agent_database_url.get_secret_value()
    try:
        root = Path(__file__).parents[2]
        command.upgrade(Config(root / "alembic.ini"), "head")
    finally:
        if previous_url is None:
            del os.environ["SCYG_AGENT_DATABASE_URL"]
        else:
            os.environ["SCYG_AGENT_DATABASE_URL"] = previous_url


async def _setup_checkpoint(settings: DeploymentSettings) -> None:
    """使用同一个应用 DSN 初始化 LangGraph checkpoint."""
    checkpoint_dsn = SecretStr(
        settings.agent_database_url.get_secret_value().replace(
            "postgresql+asyncpg://", "postgresql://", 1
        )
    )
    async with CheckpointStore(CheckpointerConfig(checkpoint_dsn)) as store:
        await store.setup()
        _ = await store.readiness()


def run() -> int:
    """执行一次性数据库初始化并返回稳定退出码."""
    try:
        settings = DeploymentSettings.model_validate({})
        anyio.run(_prepare_database, settings)
        _run_agent_migrations(settings)
        anyio.run(_setup_checkpoint, settings)
    except Exception:  # noqa: BLE001, RUF100  # noqa: BROAD_EXCEPT_OK - CLI 边界隐藏部署异常。
        _ = sys.stderr.write("Agent 数据库部署初始化失败\n")
        return 1
    _ = sys.stdout.write("Agent 数据库部署初始化完成\n")
    return 0


def main() -> None:
    """启动一次性数据库部署命令."""
    raise SystemExit(run())


if __name__ == "__main__":
    main()
