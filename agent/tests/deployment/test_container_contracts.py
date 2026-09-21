"""本地 Compose 部署的日常契约。"""

import re
import subprocess
import sys
from pathlib import Path
from typing import Final

AGENT_ROOT: Final = Path(__file__).parents[2]
DOCKERFILE: Final = AGENT_ROOT / "Dockerfile"
COMPOSE_FILE: Final = AGENT_ROOT / "compose.yaml"
INIT_SQL: Final = AGENT_ROOT / "deploy" / "postgres" / "init" / "001_roles.sql"
ENV_TEMPLATE: Final = AGENT_ROOT / ".env.example"
MIGRATION: Final = AGENT_ROOT / "migrations" / "versions" / "20260711_01_agent_truth.py"


def _text(path: Path) -> str:
    """以 UTF-8 读取部署契约文件。"""
    return path.read_text(encoding="utf-8")


def test_dockerfile_uses_locked_multistage_non_root_runtime() -> None:
    content = _text(DOCKERFILE)
    stages = re.findall(r"^FROM .+ AS (\w+)$", content, flags=re.MULTILINE)

    assert len(stages) >= 2
    assert "python:3.12.13-slim-bookworm@sha256:" in content
    assert "ghcr.io/astral-sh/uv:0.8.13@sha256:" in content
    assert "uv sync --frozen --no-dev --no-editable" in content
    assert "COPY --from=builder /app/.venv /opt/venv" in content
    assert "USER 10001:10001" in content
    assert 'ENTRYPOINT ["/opt/venv/bin/python", "-m", "scyg_agent", "run"]' in content
    assert "HEALTHCHECK" in content
    assert "/health/ready" in content


def test_dockerignore_excludes_private_and_unrelated_context() -> None:
    ignored = set(_text(AGENT_ROOT / ".dockerignore").splitlines())

    assert {
        ".git",
        ".env",
        ".env.*",
        ".venv",
        "tests",
        "*.pem",
        "*.key",
    } <= ignored
    assert "!.env.example" in ignored


def test_compose_uses_one_agent_account() -> None:
    content = _text(COMPOSE_FILE)
    parsed = subprocess.run(  # noqa: S603 - 仅执行当前锁定的 Python 解释器。
        [
            sys.executable,
            "-c",
            "import yaml; yaml.safe_load(open('compose.yaml', encoding='utf-8'))",
        ],
        cwd=AGENT_ROOT,
        capture_output=True,
        check=False,
        text=True,
    )

    assert parsed.returncode == 0, parsed.stderr
    assert content.count("\n  postgres:\n") == 1
    assert content.count("\n  agent-setup:\n") == 1
    assert content.count("\n  agent:\n") == 1
    assert "condition: service_healthy" in content
    assert "condition: service_completed_successfully" in content
    assert "SCYG_DEPLOY_AGENT_DATABASE_PASSWORD" in content
    assert "SCYG_DEPLOY_CHECKPOINT" not in content
    assert "SCYG_AGENT_CHECKPOINT_DATABASE_URL" not in content


def test_bootstrap_and_migration_prepare_one_owner() -> None:
    bootstrap = _text(INIT_SQL)
    migration = _text(MIGRATION)

    assert "CREATE ROLE scyg_agent" in bootstrap
    assert "scyg_checkpoint" not in bootstrap
    assert "CREATE SCHEMA IF NOT EXISTS langgraph AUTHORIZATION scyg_agent" in bootstrap
    assert "scyg_checkpoint" not in migration
    assert 'op.execute("CREATE SCHEMA IF NOT EXISTS langgraph")' in migration


def test_environment_template_has_single_database_configuration() -> None:
    content = _text(ENV_TEMPLATE)
    assignments = [line for line in content.splitlines() if line and not line.startswith("#")]

    assert assignments
    assert all(line.endswith("=") for line in assignments)
    assert "SCYG_AGENT_DATABASE_URL=" in content
    assert "SCYG_AGENT_DB_PASSWORD=" in content
    assert "SCYG_CHECKPOINT_DATABASE_URL" not in content
    assert "SCYG_CHECKPOINT_DB_PASSWORD" not in content
