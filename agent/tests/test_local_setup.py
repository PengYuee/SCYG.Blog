"""本机数据库统一初始化的连接边界测试。"""

import pytest
from pydantic import SecretStr

from scyg_agent.config import ApplicationSettings
from scyg_agent.local_setup import (
    LocalSetupError,
    LocalSetupStageError,
    default_admin_url,
    setup_local_database,
)


def settings(database_url: str) -> ApplicationSettings:
    """构造只验证 setup 地址推导所需的完整设置。"""
    return ApplicationSettings.model_validate(
        {
            "database_url": SecretStr(database_url),
            "models": {
                tier: {
                    "base_url": "https://provider.example/v1",
                    "api_key": SecretStr("provider-secret"),
                    "model": "model",
                    "timeout_seconds": 60,
                }
                for tier in ("fast", "standard", "strong")
            },
        }
    )


def test_default_admin_url_uses_database_endpoint_without_role_secret() -> None:
    configured = settings(
        "postgresql+asyncpg://scyg_agent:agent%40secret@localhost:5544/scyg_agent"
    )

    admin_url = default_admin_url(configured)

    assert admin_url == "postgresql://postgres@localhost:5544/postgres"
    assert "agent" not in admin_url.split("@", maxsplit=1)[0]
    assert "secret" not in admin_url


def test_default_admin_url_rejects_non_agent_role() -> None:
    configured = settings("postgresql+asyncpg://postgres:admin-secret@localhost:5544/scyg_agent")

    with pytest.raises(LocalSetupError):
        _ = default_admin_url(configured)


def test_setup_runs_database_migrations_before_checkpoint(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    configured = settings("postgresql+asyncpg://scyg_agent:agent-secret@localhost:5544/scyg_agent")
    events: list[str] = []
    password_sentinel = "setup-password-sentinel"  # noqa: S105 - 非凭据测试哨兵。

    async def prepare(_settings: ApplicationSettings, admin_url: str, password: str) -> None:
        assert admin_url == "postgresql://admin@localhost:5544/postgres"
        assert password == password_sentinel
        events.append("prepare")

    def migrate(_settings: ApplicationSettings) -> None:
        events.append("migrate")

    async def checkpoint(_settings: ApplicationSettings) -> None:
        events.append("checkpoint")

    monkeypatch.setattr("scyg_agent.local_setup._prepare_database", prepare)
    monkeypatch.setattr("scyg_agent.local_setup._run_migrations", migrate)
    monkeypatch.setattr("scyg_agent.local_setup._setup_checkpoint", checkpoint)

    setup_local_database(
        configured,
        password_sentinel,
        "postgresql://admin@localhost:5544/postgres",
    )
    assert events == ["prepare", "migrate", "checkpoint"]


def test_setup_stops_at_failed_stage_without_running_later_stages(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    configured = settings("postgresql+asyncpg://scyg_agent:agent-secret@localhost:5544/scyg_agent")
    events: list[str] = []

    async def prepare(_settings: ApplicationSettings, _admin_url: str, _password: str) -> None:
        events.append("prepare")

    def migrate(_settings: ApplicationSettings) -> None:
        events.append("migrate")
        failure = "database-password-sentinel"
        raise RuntimeError(failure)

    async def checkpoint(_settings: ApplicationSettings) -> None:
        events.append("checkpoint")

    monkeypatch.setattr("scyg_agent.local_setup._prepare_database", prepare)
    monkeypatch.setattr("scyg_agent.local_setup._run_migrations", migrate)
    monkeypatch.setattr("scyg_agent.local_setup._setup_checkpoint", checkpoint)

    with pytest.raises(LocalSetupStageError) as raised:
        setup_local_database(
            configured, "admin-password-sentinel", "postgresql://admin@localhost/postgres"
        )

    assert str(raised.value) == "stage=migration; error=RuntimeError"
    assert events == ["prepare", "migrate"]
