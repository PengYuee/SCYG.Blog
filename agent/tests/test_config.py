"""Strict process configuration and explicit server-owned model tier tests."""

from pathlib import Path

import pytest
from pydantic import SecretStr, ValidationError

from scyg_agent.adapters.database.config import AsyncDatabaseConfig
from scyg_agent.config import ConfigurationFileError, ModelProtocol, load_settings


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.fixture(autouse=True)
def isolate_default_config(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """Ignore the developer's local configuration."""
    monkeypatch.chdir(tmp_path)


def _clear_required_environment(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("SCYG_AGENT_DATABASE_URL", raising=False)
    monkeypatch.delenv("SCYG_AGENT_MODELS", raising=False)
    monkeypatch.delenv("SCYG_AGENT_MODELS__PROTOCOL", raising=False)
    for tier in ("FAST", "STANDARD", "STRONG"):
        for field in ("MODEL", "BASE_URL", "API_KEY", "TIMEOUT_SECONDS"):
            monkeypatch.delenv(f"SCYG_AGENT_MODELS__{tier}__{field}", raising=False)


def test_toml_configuration_can_supply_required_values(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    _clear_required_environment(monkeypatch)
    config_file = tmp_path / "agent.toml"
    tiers = "\n".join(
        f"""[models.{tier}]
model = "file-{tier}"
base_url = "https://file-provider.example/v1"
api_key = "file-provider-secret"
timeout_seconds = 30
"""
        for tier in ("fast", "standard", "strong")
    )
    _ = config_file.write_text(
        """database_url = "postgresql+asyncpg://agent:file-secret@localhost/scyg_agent"
[models]
protocol = "responses"
"""
        + tiers,
        encoding="utf-8",
    )
    settings = load_settings(config_file)
    assert settings.models.protocol is ModelProtocol.RESPONSES
    assert settings.models.fast.model == "file-fast"
    assert settings.models.standard.model == "file-standard"
    assert settings.models.strong.model == "file-strong"
    assert settings.models.fast.base_url.unicode_string() == "https://file-provider.example/v1"


def test_toml_values_override_environment_and_environment_fills_missing_values(
    configured_environment: None,
    tmp_path: Path,
) -> None:
    assert configured_environment is None
    config_file = tmp_path / "agent.toml"
    _ = config_file.write_text(
        'http_port = 8181\n[models.fast]\nmodel = "file-model"\n',
        encoding="utf-8",
    )
    settings = load_settings(config_file)
    assert settings.models.fast.model == "file-model"
    assert settings.models.standard.model == "test-model"
    assert settings.http_port == 8181
    assert settings.grpc_port == 9090
    assert settings.models.fast.api_key.get_secret_value() == "provider-secret"


def test_malformed_toml_is_reported_without_file_content(tmp_path: Path) -> None:
    config_file = tmp_path / "agent.toml"
    sentinel = "sentinel-secret-value"
    _ = config_file.write_text(f'[models.fast]\napi_key = "{sentinel}\n', encoding="utf-8")
    with pytest.raises(ConfigurationFileError) as caught:
        _ = load_settings(config_file)
    assert str(caught.value) == f"{config_file}: contains invalid TOML"
    assert sentinel not in str(caught.value)


def test_settings_redact_secrets_from_repr_and_serialization(configured_environment: None) -> None:
    assert configured_environment is None
    settings = load_settings()
    for rendered in (repr(settings), settings.model_dump_json()):
        assert "provider-secret" not in rendered
        assert "database-secret" not in rendered


@pytest.mark.parametrize(
    ("name", "value"),
    [
        ("SCYG_AGENT_MODELS__FAST__BASE_URL", "not-a-url"),
        ("SCYG_AGENT_MODELS__PROTOCOL", "unknown"),
        ("SCYG_AGENT_MODELS__STRONG__TIMEOUT_SECONDS", "0"),
        ("SCYG_AGENT_WORKER_CONCURRENCY", "0"),
        ("SCYG_AGENT_WORKER_CONCURRENCY", "65"),
    ],
)
def test_settings_reject_malformed_boundary_values(
    configured_environment: None,
    monkeypatch: pytest.MonkeyPatch,
    name: str,
    value: str,
) -> None:
    assert configured_environment is None
    monkeypatch.setenv(name, value)
    with pytest.raises(ValidationError):
        _ = load_settings()


def test_settings_require_database_and_model_tiers(monkeypatch: pytest.MonkeyPatch) -> None:
    _clear_required_environment(monkeypatch)
    with pytest.raises(ValidationError) as caught:
        _ = load_settings()
    message = str(caught.value)
    assert "database_url" in message
    assert "models" in message
    assert "provider-secret" not in message


def test_legacy_provider_options_are_not_configuration_aliases(
    configured_environment: None,
    tmp_path: Path,
) -> None:
    assert configured_environment is None
    config_file = tmp_path / "agent.toml"
    _ = config_file.write_text('provider_model = "obsolete-model"\n', encoding="utf-8")
    with pytest.raises(ValidationError):
        _ = load_settings(config_file)


@pytest.mark.anyio
async def test_database_config_builds_bounded_secret_engine() -> None:
    config = AsyncDatabaseConfig(
        SecretStr("postgresql+asyncpg://user:secret@localhost/database"),
        pool_size=2,
        max_overflow=3,
        pool_timeout_seconds=4,
    )
    engine = config.create_engine()
    assert engine.dialect.name == "postgresql"
    assert "secret" not in repr(config)
    await engine.dispose()
