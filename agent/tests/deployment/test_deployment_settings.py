"""本地部署配置的日常路径测试。"""

import pytest
from pydantic import ValidationError

from scyg_agent.deployment_contracts import DeploymentSettings


def _settings(**overrides: str) -> DeploymentSettings:
    """构造普通本地部署配置。"""
    values = {
        "admin_database_url": "postgresql://postgres:admin%40sentinel@postgres:5432/scyg_agent",
        "admin_database_password": "admin@sentinel",
        "agent_database_url": (
            "postgresql+asyncpg://scyg_agent:agent%3Asentinel@postgres:5432/scyg_agent"
        ),
        "agent_database_password": "agent:sentinel",
    }
    values.update(overrides)
    return DeploymentSettings.model_validate(values)


def test_settings_accepts_one_application_dsn() -> None:
    settings = _settings()

    assert settings.agent_database_url.get_secret_value().startswith(
        "postgresql+asyncpg://scyg_agent:"
    )


def test_settings_rejects_agent_password_mismatch() -> None:
    wrong_password = "wrong-password"  # noqa: S105 - 测试哨兵。
    with pytest.raises(ValidationError):
        _ = _settings(agent_database_password=wrong_password)
