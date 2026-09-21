"""本地 PostgreSQL 一次性部署的最小配置契约."""

import re
from types import MappingProxyType
from typing import ClassVar, Final, override
from urllib.parse import unquote_to_bytes

from pydantic import PostgresDsn, SecretStr, TypeAdapter, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

ADMIN_ROLE: Final = "postgres"
AGENT_ROLE: Final = "scyg_agent"
AGENT_SCHEMA: Final = "public"
CHECKPOINT_SCHEMA: Final = "langgraph"
DATABASE_HOST: Final = "postgres"
DATABASE_PORT: Final = 5432
DATABASE_PATH: Final = "/scyg_agent"
ROLE_SCHEMES: Final = MappingProxyType(
    {
        ADMIN_ROLE: "postgresql",
        AGENT_ROLE: "postgresql+asyncpg",
    }
)
_RAW_SCHEME = r"^(?P<scheme>[a-z0-9+]+)://(?P<username>[a-z_]+):"
_RAW_PASSWORD = r"(?P<password>(?:[A-Za-z0-9._~-]|%[0-9A-Fa-f]{2})+)"  # noqa: S105
_RAW_SUFFIX = r"@(?P<host>[a-z]+):(?P<port>[0-9]+)/(?P<database>[a-z_]+)$"
RAW_DSN_PATTERN: Final = re.compile(_RAW_SCHEME + _RAW_PASSWORD + _RAW_SUFFIX)
ENCODED_OCTET_PATTERN: Final = re.compile(r"%[0-9A-Fa-f]{2}")


class DeploymentConfigurationError(ValueError):
    """表示本地 Compose 数据库身份或密码绑定不一致."""

    @override
    def __str__(self) -> str:
        """返回不含连接和密码的稳定诊断."""
        return "Agent 数据库部署配置无效"


class DeploymentSetupError(RuntimeError):
    """表示数据库部署初始化失败."""

    @override
    def __str__(self) -> str:
        """返回不含连接、角色口令或 SQL 细节的诊断."""
        return "Agent 数据库部署初始化失败"


def _ensure_database_identity(url: SecretStr, password: SecretStr, role: str) -> None:
    """在 URL decode 前后验证本地 Compose DSN 的身份与密码."""
    raw_url = url.get_secret_value()
    match = RAW_DSN_PATTERN.fullmatch(raw_url)
    if match is None:
        raise DeploymentConfigurationError
    encoded_password = match.group("password")
    try:
        decoded_password = unquote_to_bytes(encoded_password).decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise DeploymentConfigurationError from error
    try:
        parsed = TypeAdapter(PostgresDsn).validate_python(raw_url)
    except ValueError as error:
        raise DeploymentConfigurationError from error
    hosts = parsed.hosts()
    valid = (
        len(hosts) == 1
        and match.group("scheme") == ROLE_SCHEMES[role]
        and match.group("username") == role
        and match.group("host") == DATABASE_HOST
        and int(match.group("port")) == DATABASE_PORT
        and f"/{match.group('database')}" == DATABASE_PATH
        and parsed.query is None
        and parsed.fragment is None
        and decoded_password == password.get_secret_value()
        and ENCODED_OCTET_PATTERN.search(decoded_password) is None
    )
    if not valid:
        raise DeploymentConfigurationError


class DeploymentSettings(BaseSettings):
    """解析一次性 setup 所需的管理员和单一应用 DSN."""

    model_config: ClassVar[SettingsConfigDict] = SettingsConfigDict(
        env_prefix="SCYG_DEPLOY_", frozen=True, extra="forbid", hide_input_in_errors=True
    )
    admin_database_url: SecretStr
    admin_database_password: SecretStr
    agent_database_url: SecretStr
    agent_database_password: SecretStr

    @model_validator(mode="after")
    def validate_database_bindings(self) -> "DeploymentSettings":
        """校验管理员和应用 DSN 的身份、结构与编码契约."""
        for url, password, role in (
            (self.admin_database_url, self.admin_database_password, ADMIN_ROLE),
            (self.agent_database_url, self.agent_database_password, AGENT_ROLE),
        ):
            _ensure_database_identity(url, password, role)
        return self
