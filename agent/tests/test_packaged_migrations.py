"""Public setup entry points execute packaged Alembic assets outside cwd."""

import os
import shutil
from io import StringIO
from pathlib import Path

import pytest
from alembic import command
from alembic.config import Config

from scyg_agent import deployment, local_setup
from tests.test_local_setup import settings as local_settings


@pytest.mark.parametrize("entrypoint", ["local", "deployment"])
@pytest.mark.parametrize("fails", [False, True])
def test_setup_consumes_packaged_migrations_and_restores_environment(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, entrypoint: str, *, fails: bool
) -> None:
    source = Path(__file__).parents[1]
    package = tmp_path / "site-packages" / "scyg_agent"
    package.mkdir(parents=True)
    _ = shutil.copy2(source / "alembic.ini", package / "alembic.ini")
    _ = shutil.copytree(source / "migrations", package / "migrations")
    configuration_directory = tmp_path / "configuration"
    configuration_directory.mkdir()
    monkeypatch.chdir(configuration_directory)
    module = local_setup if entrypoint == "local" else deployment
    monkeypatch.setattr(module, "__file__", str(package / f"{module.__name__.split('.')[-1]}.py"))
    previous_url = "postgresql+asyncpg://scyg_agent:sentinel@postgres:5432/scyg_agent"
    monkeypatch.setenv("SCYG_AGENT_DATABASE_URL", previous_url)
    monkeypatch.setenv(
        "SCYG_DEPLOY_ADMIN_DATABASE_URL",
        "postgresql://postgres:admin%40sentinel@postgres:5432/scyg_agent",
    )
    monkeypatch.setenv("SCYG_DEPLOY_ADMIN_DATABASE_PASSWORD", "admin@sentinel")
    monkeypatch.setenv(
        "SCYG_DEPLOY_AGENT_DATABASE_URL",
        "postgresql+asyncpg://scyg_agent:agent%3Asentinel@postgres:5432/scyg_agent",
    )
    monkeypatch.setenv("SCYG_DEPLOY_AGENT_DATABASE_PASSWORD", "agent:sentinel")
    rendered = StringIO()
    actual_upgrade = command.upgrade

    async def skip_network(_settings: object, *_args: object) -> None:
        return None

    monkeypatch.setattr(module, "_prepare_database", skip_network)
    monkeypatch.setattr(module, "_setup_checkpoint", skip_network)

    def offline_upgrade(config: Config, revision: str) -> None:
        config.output_buffer = rendered
        if fails:
            raise RuntimeError
        actual_upgrade(config, revision, sql=True)

    monkeypatch.setattr(command, "upgrade", offline_upgrade)
    if entrypoint == "local":
        configured = local_settings("postgresql+asyncpg://scyg_agent:sentinel@localhost/scyg_agent")
        if fails:
            with pytest.raises(local_setup.LocalSetupStageError) as caught:
                local_setup.setup_local_database(configured, "sentinel")
            assert caught.value.stage == "migration"
        else:
            local_setup.setup_local_database(configured, "sentinel")
    else:
        assert deployment.run() == (1 if fails else 0)
    if not fails:
        sql = rendered.getvalue()
        assert "CREATE TABLE agent_runs" in sql
        assert "CREATE TABLE agent_successful_operations" in sql
    assert os.environ["SCYG_AGENT_DATABASE_URL"] == previous_url
