"""Alembic migration 的日常路径测试。"""

import os
import subprocess
import sys
from pathlib import Path

AGENT_ROOT = Path(__file__).parents[3]


def test_initial_revision_uses_the_single_application_role() -> None:
    environment = os.environ | {
        "SCYG_AGENT_DATABASE_URL": "postgresql+asyncpg://offline.invalid/offline"
    }
    result = subprocess.run(  # noqa: S603 - 仅执行当前锁定的 Python 解释器。
        [sys.executable, "-m", "alembic", "upgrade", "head", "--sql"],
        cwd=AGENT_ROOT,
        env=environment,
        check=True,
        capture_output=True,
        text=True,
    )

    assert 'GRANT USAGE, CREATE ON SCHEMA langgraph TO "scyg_agent"' in result.stdout
    assert "scyg_checkpoint" not in result.stdout
    assert "CREATE TABLE agent_runs" in result.stdout
