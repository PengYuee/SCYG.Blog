"""AgentControl gRPC 测试夹具注册."""

import pytest

from tests.adapters.auth.test_jwt_verifier import private_key, verifier

from .scenario_support import rpc_harness

__all__ = ["private_key", "rpc_harness", "verifier"]


@pytest.fixture
def anyio_backend() -> str:
    """grpc.aio 仅运行 asyncio 后端."""
    return "asyncio"
