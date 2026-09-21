"""两个生产 Worker 的 endpoint-guarded PostgreSQL 调度测试."""

from collections.abc import AsyncIterator

import pytest
from tests.acceptance_settings import require_test_settings

from .fixture_support import WorkerDatabaseFixture
from .postgres_support import exercise_two_workers


@pytest.fixture
def anyio_backend() -> str:
    """真实 asyncpg 测试固定 asyncio 后端."""
    return "asyncio"


@pytest.fixture
async def worker_database() -> AsyncIterator[WorkerDatabaseFixture]:
    """建立清洁生产会话并在所有路径关闭引擎。"""
    database = WorkerDatabaseFixture.create(require_test_settings().normal_url)
    try:
        await database.prepare()
        yield database
    finally:
        await database.close()


@pytest.mark.anyio
async def test_two_production_workers_process_mixed_queue_with_fencing(
    worker_database: WorkerDatabaseFixture,
) -> None:
    """验证双 Worker 的混合调度、续租、取消、恢复与有界停止."""
    await exercise_two_workers(worker_database)
