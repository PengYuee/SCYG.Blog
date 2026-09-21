"""Worker 测试统一使用生产 asyncio 后端."""

import pytest


@pytest.fixture
def anyio_backend() -> str:
    """固定 AnyIO 后端."""
    return "asyncio"
