"""Shared PostgreSQL acceptance configuration tests."""

from pathlib import Path

import pytest

from tests import acceptance_settings


def test_shared_settings_parse_urls_and_derive_listener(tmp_path: Path) -> None:
    path = tmp_path / "test-agent.toml"
    _ = path.write_text(
        (
            'normal_database_url = "postgresql+asyncpg://user:secret@localhost:5432/agent"\n'
            'migration_database_url = "postgresql+asyncpg://user:secret@localhost:5433/migration"\n'
        ),
        encoding="utf-8",
    )

    settings = acceptance_settings.TestSettings.from_file(path)

    assert settings.normal_url.endswith("localhost:5432/agent")
    assert settings.listener_dsn("normal").startswith("postgresql://")
    assert settings.migration_url.endswith("localhost:5433/migration")
    assert "secret" not in repr(settings)


def test_shared_settings_reject_invalid_database_url(tmp_path: Path) -> None:
    path = tmp_path / "test-agent.toml"
    _ = path.write_text(
        (
            'normal_database_url = "not-postgres://user:secret@localhost/agent"\n'
            'migration_database_url = "postgresql://user:secret@localhost/migration"\n'
        ),
        encoding="utf-8",
    )

    with pytest.raises(acceptance_settings.TestConfigurationError):
        _ = acceptance_settings.TestSettings.from_file(path)
