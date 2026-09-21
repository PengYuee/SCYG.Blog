"""Deep Runtime 集成测试共享夹具。"""

from collections.abc import AsyncIterator

import pytest
from grpc import aio

from tests.adapters.blog_grpc.support import FakeBlog, add_blog_servicer


@pytest.fixture
async def local_blog_server() -> AsyncIterator[tuple[str, FakeBlog]]:
    """启动并确定停止真实本地 Blog gRPC 服务。"""
    service = FakeBlog()
    server = aio.server()
    add_blog_servicer(service, server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}", service
    finally:
        await server.stop(None)
