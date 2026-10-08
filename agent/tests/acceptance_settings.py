"""Shared PostgreSQL acceptance-test configuration."""

from __future__ import annotations

import os
import tomllib
from dataclasses import dataclass
from pathlib import Path
from typing import Final, Literal

import pytest
from pydantic import PostgresDsn, SecretStr, TypeAdapter

DEFAULT_TEST_CONFIG_FILE: Final = Path(__file__).with_name("test-agent.toml")
DatabaseTarget = Literal["normal", "migration"]


class TestConfigurationError(ValueError):
    """报告测试配置结构错误且不回显连接凭据。"""


@dataclass(frozen=True, slots=True, repr=False)
class TestSettings:
    """Own one test configuration for all endpoint-backed acceptance tests."""

    normal_database_url: SecretStr
    migration_database_url: SecretStr
    admin_database_url: SecretStr | None = None

    @classmethod
    def from_file(cls, path: Path) -> TestSettings:
        """Read and validate one TOML test configuration."""
        try:
            values = tomllib.loads(path.read_text(encoding="utf-8"))
        except OSError as error:
            message = f"{path}: cannot be read"
            raise TestConfigurationError(message) from error
        except tomllib.TOMLDecodeError as error:
            message = f"{path}: invalid TOML"
            raise TestConfigurationError(message) from error
        try:
            normal = _required_url(values, "normal_database_url")
            migration = _required_url(values, "migration_database_url")
            admin = _optional_url(values, "admin_database_url")
        except (TypeError, ValueError) as error:
            message = f"{path}: invalid test endpoint configuration"
            raise TestConfigurationError(message) from error
        return cls(
            normal_database_url=SecretStr(normal),
            migration_database_url=SecretStr(migration),
            admin_database_url=SecretStr(admin) if admin is not None else None,
        )

    @property
    def normal_url(self) -> str:
        """Return the normal acceptance DSN at the I/O boundary."""
        return self.normal_database_url.get_secret_value()

    @property
    def migration_url(self) -> str:
        """Return the destructive-migration acceptance DSN at the I/O boundary."""
        return self.migration_database_url.get_secret_value()

    def url(self, target: DatabaseTarget) -> str:
        """Select one configured database without exposing values in diagnostics."""
        return self.normal_url if target == "normal" else self.migration_url

    def listener_dsn(self, target: DatabaseTarget) -> str:
        """Convert a SQLAlchemy PostgreSQL DSN to an asyncpg listener DSN."""
        value = self.url(target)
        for scheme in ("postgresql+asyncpg://", "postgresql+psycopg://"):
            if value.startswith(scheme):
                return "postgresql://" + value.removeprefix(scheme)
        return value

    def child_environment(self, target: DatabaseTarget) -> dict[str, str]:
        """Build a child-process environment with the selected Agent DSN."""
        environment = os.environ.copy()
        environment["SCYG_AGENT_DATABASE_URL"] = self.url(target)
        return environment

    def require_admin_url(self) -> str:
        """Return the optional administrator endpoint or skip that acceptance test."""
        if self.admin_database_url is None:
            pytest.skip("admin_database_url is required for this PostgreSQL acceptance")
        return self.admin_database_url.get_secret_value()


def load_test_settings() -> TestSettings | None:
    """Load the configured test file, returning None when acceptance is not enabled."""
    configured = os.environ.get("SCYG_TEST_CONFIG_FILE")
    path = Path(configured) if configured else DEFAULT_TEST_CONFIG_FILE
    if not path.is_file():
        return None
    return TestSettings.from_file(path)


def require_test_settings() -> TestSettings:
    """Load the shared test file or skip endpoint-backed acceptance cleanly."""
    settings = load_test_settings()
    if settings is None:
        message = "shared PostgreSQL test configuration is required; set SCYG_TEST_CONFIG_FILE"
        pytest.skip(message)
    return settings


def _required_url(values: dict[str, object], field: str) -> str:
    value = values.get(field)
    if not isinstance(value, str) or not value:
        raise TestConfigurationError(field)
    _ = TypeAdapter(PostgresDsn).validate_python(value)
    return value


def _optional_url(values: dict[str, object], field: str) -> str | None:
    value = values.get(field)
    if value is None:
        return None
    if not isinstance(value, str) or not value:
        raise TestConfigurationError(field)
    _ = TypeAdapter(PostgresDsn).validate_python(value)
    return value
