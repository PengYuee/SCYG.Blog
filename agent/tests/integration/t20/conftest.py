"""T20 JWT 夹具导出。"""

from tests.adapters.auth.test_jwt_verifier import private_key, verifier

__all__ = ["private_key", "verifier"]


import pytest


@pytest.fixture
def anyio_backend() -> str:
    """grpc.aio 与 asyncpg 使用 asyncio 后端。"""
    return "asyncio"
