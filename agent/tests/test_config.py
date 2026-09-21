"""Configuration boundary tests."""

from pathlib import Path

import pytest
from pydantic import SecretStr, ValidationError

from scyg_agent.adapters.database.config import AsyncDatabaseConfig
from scyg_agent.config import ConfigurationFileError, load_settings


@pytest.fixture
def anyio_backend() -> str:
    """仅使用项目已安装的 asyncio 后端。"""
    return "asyncio"


@pytest.fixture(autouse=True)
def isolate_default_config(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """Prevent a developer's ignored agent.toml from changing test inputs."""
    monkeypatch.chdir(tmp_path)


def test_settings_apply_operational_defaults_when_required_values_exist(
    configured_environment: None,
) -> None:
    # Given: the minimum valid environment is installed by the fixture.
    # When: settings parse the process environment once.
    assert configured_environment is None
    settings = load_settings()

    # Then: frozen typed operational defaults are available.
    assert settings.http_port == 8080
    assert settings.grpc_port == 9090
    assert settings.simple_concurrency == 4
    assert settings.deep_concurrency == 1
    assert settings.lease_seconds == 60
    assert settings.heartbeat_seconds == 15
    assert settings.shutdown_seconds == 30


def test_toml_configuration_can_supply_required_values(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    for name in (
        "SCYG_AGENT_DATABASE_URL",
        "SCYG_AGENT_JWT_PUBLIC_KEY_PATH",
        "SCYG_AGENT_PROVIDER_BASE_URL",
        "SCYG_AGENT_PROVIDER_API_KEY",
        "SCYG_AGENT_PROVIDER_MODEL",
    ):
        monkeypatch.delenv(name, raising=False)
    config_file = tmp_path / "agent.toml"
    _ = config_file.write_text(
        """database_url = "postgresql+asyncpg://agent:file-secret@localhost/scyg_agent"
jwt_public_key_path = "tests/fixtures/public.pem"
provider_base_url = "https://file-provider.example/v1"
provider_api_key = "file-provider-secret"
provider_model = "file-model"
""",
        encoding="utf-8",
    )

    settings = load_settings(config_file)

    assert settings.provider_model == "file-model"
    assert settings.provider_base_url.unicode_string() == "https://file-provider.example/v1"


def test_toml_values_override_environment_and_environment_fills_missing_values(
    configured_environment: None, tmp_path: Path
) -> None:
    assert configured_environment is None
    config_file = tmp_path / "agent.toml"
    _ = config_file.write_text(
        'provider_model = "file-model"\nhttp_port = 8181\n', encoding="utf-8"
    )

    settings = load_settings(config_file)

    assert settings.provider_model == "file-model"
    assert settings.http_port == 8181
    assert settings.grpc_port == 9090
    assert settings.provider_api_key.get_secret_value() == "provider-secret"


def test_malformed_toml_is_reported_without_file_content(tmp_path: Path) -> None:
    config_file = tmp_path / "agent.toml"
    sentinel = "sentinel-secret-value"
    _ = config_file.write_text(f'provider_api_key = "{sentinel}\n', encoding="utf-8")

    with pytest.raises(ConfigurationFileError) as caught:
        _ = load_settings(config_file)

    assert str(caught.value) == f"{config_file}: contains invalid TOML"
    assert sentinel not in str(caught.value)


def test_settings_redact_secrets_from_repr_and_serialization(
    configured_environment: None,
) -> None:
    # Given: settings contain provider and database credentials.
    # When: standard diagnostic and serialization surfaces are rendered.
    assert configured_environment is None
    settings = load_settings()
    rendered = repr(settings)
    serialized = settings.model_dump_json()

    # Then: neither secret value is disclosed.
    assert "provider-secret" not in rendered
    assert "provider-secret" not in serialized
    assert "database-secret" not in rendered
    assert "database-secret" not in serialized


@pytest.mark.parametrize(
    ("name", "value"),
    [
        ("SCYG_AGENT_PROVIDER_BASE_URL", "not-a-url"),
        ("SCYG_AGENT_SIMPLE_CONCURRENCY", "0"),
        ("SCYG_AGENT_DEEP_CONCURRENCY", "2"),
    ],
)
def test_settings_reject_malformed_boundary_values(
    configured_environment: None,
    monkeypatch: pytest.MonkeyPatch,
    name: str,
    value: str,
) -> None:
    # Given: one external value violates its typed boundary.
    assert configured_environment is None
    monkeypatch.setenv(name, value)

    # When/Then: parsing rejects the invalid environment.
    with pytest.raises(ValidationError):
        _ = load_settings()


def test_settings_require_database_jwt_and_provider_credentials(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: no required service environment is present.
    for name in (
        "SCYG_AGENT_DATABASE_URL",
        "SCYG_AGENT_JWT_PUBLIC_KEY_PATH",
        "SCYG_AGENT_PROVIDER_BASE_URL",
        "SCYG_AGENT_PROVIDER_API_KEY",
        "SCYG_AGENT_PROVIDER_MODEL",
    ):
        monkeypatch.delenv(name, raising=False)

    # When: settings parse an incomplete environment.
    with pytest.raises(ValidationError) as caught:
        _ = load_settings()

    # Then: required field names are reported without credential values.
    message = str(caught.value)
    assert "database_url" in message
    assert "jwt_public_key_path" in message
    assert "provider_api_key" in message
    assert "provider-secret" not in message


@pytest.mark.anyio
async def test_database_config_builds_bounded_secret_engine() -> None:
    """验证数据库配置在 I/O 边界创建有界且不泄密的引擎。"""
    # Given: 一个无需连接即可解析的异步 PostgreSQL DSN。
    config = AsyncDatabaseConfig(
        SecretStr("postgresql+asyncpg://user:secret@localhost/database"),
        pool_size=2,
        max_overflow=3,
        pool_timeout_seconds=4,
    )

    # When: 配置创建异步引擎。
    engine = config.create_engine()

    # Then: 引擎保留方言和池边界, 且配置表示不泄露密钥。
    assert engine.dialect.name == "postgresql"
    assert "secret" not in repr(config)
    await engine.dispose()
