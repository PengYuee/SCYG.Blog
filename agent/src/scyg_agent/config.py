"""Typed process configuration boundary."""

import os
import tomllib
from dataclasses import dataclass
from enum import StrEnum
from importlib.metadata import version
from pathlib import Path
from typing import Annotated, ClassVar, Final, Self, override

from pydantic import (
    AnyHttpUrl,
    Field,
    PostgresDsn,
    SecretStr,
    TypeAdapter,
    ValidationError,
    model_validator,
)
from pydantic_core import ErrorDetails
from pydantic_settings import BaseSettings, SettingsConfigDict

PositiveSeconds = Annotated[int, Field(ge=1)]
PositiveBoundedIterations = Annotated[int, Field(ge=1, le=128)]
ContextTokens = Annotated[int, Field(ge=1_024, le=1_000_000)]
type ErrorLocation = str | int

AGENT_CONTRACT_VERSION: Final = "v1"
AGENT_STATE_SCHEMA_VERSION: Final = "v1"


@dataclass(frozen=True, slots=True)
class ConfigurationIssue:
    """A field-level configuration diagnostic that never carries input values."""

    field: str
    reason: str


@dataclass(frozen=True, slots=True)
class ConfigurationFileError(ValueError):
    """报告无法读取或解析的配置文件而不泄露文件内容."""

    path: Path
    reason: str

    @override
    def __str__(self) -> str:
        return f"{self.path}: {self.reason}"


class ModelProtocol(StrEnum):
    """Explicit provider API protocol shared by all model tiers."""

    CHAT_COMPLETIONS = "chat_completions"
    RESPONSES = "responses"


class ModelTierSettings(BaseSettings):
    """One server-owned model tier configuration."""

    model_config: ClassVar[SettingsConfigDict] = SettingsConfigDict(frozen=True, extra="forbid")

    model: Annotated[str, Field(min_length=1, repr=False)]
    base_url: AnyHttpUrl = Field(repr=False)
    api_key: SecretStr = Field(repr=False)
    timeout_seconds: PositiveSeconds


class ModelSettings(BaseSettings):
    """Explicit protocol and model tiers selected by server policy."""

    model_config: ClassVar[SettingsConfigDict] = SettingsConfigDict(frozen=True, extra="forbid")

    protocol: ModelProtocol = ModelProtocol.CHAT_COMPLETIONS
    fast: ModelTierSettings
    standard: ModelTierSettings
    strong: ModelTierSettings


class FeatureFlags(BaseSettings):
    """允许测试按类型关闭组件,生产默认全部启用."""

    model_config: ClassVar[SettingsConfigDict] = SettingsConfigDict(frozen=True)
    grpc: bool = True
    worker: bool = True
    http: bool = True


class ApplicationSettings(BaseSettings):
    """Parse and freeze Agent process configuration from TOML and environment."""

    model_config: ClassVar[SettingsConfigDict] = SettingsConfigDict(
        env_prefix="SCYG_AGENT_",
        env_nested_delimiter="__",
        frozen=True,
        extra="forbid",
    )

    application_contract_version: str = AGENT_CONTRACT_VERSION
    state_schema_version: str = AGENT_STATE_SCHEMA_VERSION
    agent_max_iterations: PositiveBoundedIterations = 32
    agent_max_context_tokens: ContextTokens = 128_000
    http_host: str = "127.0.0.1"
    http_port: Annotated[int, Field(ge=1, le=65535)] = 8080
    grpc_host: str = "127.0.0.1"
    grpc_port: Annotated[int, Field(ge=1, le=65535)] = 9090
    database_url: SecretStr = Field(repr=False)
    redis_url: SecretStr = Field(default=SecretStr("redis://localhost:6379/0"), repr=False)
    redis_ttl_seconds: PositiveSeconds = 86_400
    redis_stream_maxlen: Annotated[int, Field(ge=100, le=1_000_000)] = 10_000
    redis_connect_timeout_seconds: Annotated[float, Field(gt=0, le=300)] = 5.0
    redis_read_timeout_seconds: Annotated[float, Field(gt=0, le=300)] = 30.0
    stream_flush_chars: Annotated[int, Field(ge=1, le=32_000)] = 4_096
    stream_flush_interval_ms: Annotated[int, Field(ge=1, le=60_000)] = 100
    blog_grpc_target: SecretStr = Field(default=SecretStr("127.0.0.1:50051"), repr=False)
    blog_deadline_seconds: PositiveSeconds = 10
    worker_concurrency: Annotated[int, Field(ge=1, le=64)] = 4
    models: ModelSettings
    lease_seconds: PositiveSeconds = 60
    heartbeat_seconds: PositiveSeconds = 15
    shutdown_seconds: PositiveSeconds = 30
    worker_poll_milliseconds: PositiveSeconds = 100
    worker_error_backoff_seconds: PositiveSeconds = 1
    worker_drain_seconds: PositiveSeconds = 10
    feature_flags: FeatureFlags = FeatureFlags()

    @model_validator(mode="after")
    def validate_database_url(self) -> Self:
        """Require an async PostgreSQL DSN while retaining secret serialization."""
        _ = TypeAdapter(PostgresDsn).validate_python(
            self.database_url.get_secret_value(),
        )
        return self


Settings = ApplicationSettings


def load_settings(config_file: Path | None = None) -> ApplicationSettings:
    """Load TOML values over environment values, then validate once."""
    resolved_file = (
        Path(os.environ.get("SCYG_AGENT_CONFIG_FILE", "agent.toml"))
        if config_file is None
        else config_file
    )
    file_values = _read_config_file(resolved_file)
    return ApplicationSettings.model_validate(file_values)


def dependency_contract_fingerprint() -> str:
    """Return the pinned framework versions used by checkpoint compatibility."""
    packages = ("deepagents", "langchain", "langgraph", "langgraph-checkpoint-postgres")
    return "|".join(f"{package}={version(package)}" for package in packages)


def _read_config_file(config_file: Path) -> dict[str, object]:
    if not config_file.is_file():
        return {}
    try:
        content = config_file.read_text(encoding="utf-8")
        parsed = tomllib.loads(content)
    except OSError as error:
        raise ConfigurationFileError(config_file, "cannot be read") from error
    except tomllib.TOMLDecodeError as error:
        raise ConfigurationFileError(config_file, "contains invalid TOML") from error
    return TypeAdapter(dict[str, object]).validate_python(parsed)


def sanitize_validation_error(error: ValidationError) -> tuple[ConfigurationIssue, ...]:
    """Convert Pydantic diagnostics to stable value-free configuration issues."""
    return tuple(_sanitize_error(detail) for detail in error.errors(include_input=False))


def _sanitize_error(detail: ErrorDetails) -> ConfigurationIssue:
    location: tuple[ErrorLocation, ...] = detail["loc"]
    field = ".".join(str(part) for part in location) or "database_url"
    error_type = detail["type"]
    reason = _reason_for(field, error_type)
    return ConfigurationIssue(field=field, reason=reason)


def _reason_for(field: str, error_type: str) -> str:
    if field == "database_url":
        return "must be a valid PostgreSQL DSN"
    if error_type in {"url_parsing", "url_scheme"}:
        return "must be a valid URL"
    if error_type == "greater_than_equal":
        return "must be greater than or equal to 1"
    if error_type == "missing":
        return "is required"
    return "has an invalid value"
