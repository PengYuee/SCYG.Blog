"""Shared deterministic settings fixtures."""

from pathlib import Path

import pytest


@pytest.fixture
def configured_environment(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """Provide the minimum valid service configuration without real credentials."""
    monkeypatch.setenv("SCYG_AGENT_CONFIG_FILE", str(tmp_path / "missing-agent.toml"))
    monkeypatch.setenv(
        "SCYG_AGENT_DATABASE_URL",
        "postgresql+asyncpg://agent:database-secret@localhost:5432/scyg_agent",
    )
    monkeypatch.setenv("SCYG_AGENT_JWT_PUBLIC_KEY_PATH", "tests/fixtures/public.pem")
    monkeypatch.setenv("SCYG_AGENT_PROVIDER_BASE_URL", "https://provider.example/v1")
    monkeypatch.setenv("SCYG_AGENT_PROVIDER_API_KEY", "provider-secret")
    monkeypatch.setenv("SCYG_AGENT_PROVIDER_MODEL", "test-model")


def pytest_collection_modifyitems(items: list[pytest.Item]) -> None:
    """Classify endpoint-backed tests without duplicating module boilerplate."""
    for item in items:
        path = str(item.path).replace("\\", "/")
        if "postgresql" in path:
            item.add_marker("postgres")
        if "/integration/" in path or path.endswith("test_cli_subprocess.py"):
            item.add_marker("integration")
        if "migration" in path or "t19" in path or "t20" in path:
            item.add_marker("migration")
