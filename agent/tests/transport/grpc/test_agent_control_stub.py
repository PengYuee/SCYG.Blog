"""AgentControl 真实 grpc.aio 服务与生成 stub 测试."""

from collections.abc import AsyncIterator, Awaitable
from datetime import UTC, datetime
from typing import TYPE_CHECKING, Protocol

import grpc
import pytest
from cryptography.hazmat.primitives.asymmetric import rsa
from grpc import aio

from scyg_agent.adapters.auth import JwtVerifier
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc
from scyg_agent.generated.scyg.agent.v1 import common_pb2
from scyg_agent.transport.grpc import AgentControlServicer
from tests.adapters.auth.test_jwt_verifier import claims, encode
from tests.application.test_facade import facade

NOW = datetime(2026, 7, 12, 13, tzinfo=UTC)
Metadata = tuple[tuple[str, str], ...]


class ControlStub(Protocol):
    """声明测试使用的生成 stub 能力."""

    def CreateRun(  # noqa: N802
        self, request: service_pb2.CreateRunRequest, *, metadata: Metadata
    ) -> Awaitable[service_pb2.CreateRunResponse]: ...

    def GetRun(  # noqa: N802
        self, request: service_pb2.GetRunRequest, *, metadata: Metadata
    ) -> Awaitable[service_pb2.GetRunResponse]: ...


if TYPE_CHECKING:

    def add_control_servicer(_servicer: AgentControlServicer, _server: aio.Server) -> None: ...
else:
    add_control_servicer = service_grpc.add_AgentControlServiceServicer_to_server


@pytest.fixture
def anyio_backend() -> str:
    """grpc.aio 仅运行 asyncio 后端."""
    return "asyncio"


class FixedClock:
    """提供稳定创建时间."""

    def now(self) -> datetime:
        """返回固定 UTC 时间."""
        return NOW


@pytest.fixture
async def control_server(
    verifier: JwtVerifier,
) -> AsyncIterator[tuple[str, ControlStub]]:
    """启动真实本地 AgentControl 服务并确定关闭资源."""
    application, _, _ = facade()
    server = aio.server()
    add_control_servicer(AgentControlServicer(application, verifier, FixedClock()), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = aio.insecure_channel(f"127.0.0.1:{port}")
    try:
        stub: ControlStub = service_grpc.AgentControlServiceStub(channel)
        yield f"127.0.0.1:{port}", stub
    finally:
        await channel.close()
        await server.stop(None)


def create_request(operation_id: str = "operation-1") -> service_pb2.CreateRunRequest:
    """构造语义完整的创建请求."""
    return service_pb2.CreateRunRequest(
        metadata=common_pb2.RequestMetadata(request_id="request-1", correlation_id="trace-1"),
        operation_id=operation_id,
        owner_user_id=common_pb2.UserId(value="user-t20"),
        task_type=common_pb2.TASK_TYPE_SUMMARY,
        runtime=common_pb2.RuntimeSelection(kind=common_pb2.RUNTIME_KIND_SIMPLE, version="v1"),
        initial_message="初始消息",
        article_id=common_pb2.ArticleId(value="article-20"),
    )


@pytest.mark.anyio
async def test_create_duplicate_and_get_use_generated_stub(
    control_server: tuple[str, ControlStub],
    private_key: rsa.RSAPrivateKey,
) -> None:
    # Given: 合法 Blog service JWT 与真实生成 stub。
    _, stub = control_server
    token = encode(private_key, claims("blog_service")).value
    metadata = (("authorization", f"Bearer {token}"),)
    # When: 重放创建并按响应 Run 标识读取快照。
    first = await stub.CreateRun(create_request(), metadata=metadata)
    duplicate = await stub.CreateRun(create_request(), metadata=metadata)
    current = await stub.GetRun(
        service_pb2.GetRunRequest(
            metadata=common_pb2.RequestMetadata(request_id="request-2", correlation_id="trace-1"),
            run_id=first.run.id,
            owner_user_id=common_pb2.UserId(value="user-t20"),
        ),
        metadata=metadata,
    )
    # Then: 真实序列化保持幂等并只公开稳定字段。
    assert duplicate.run == first.run
    assert current.run.id == first.run.id
    assert current.run.revision == 1
    assert not current.run.HasField("failure")


@pytest.mark.anyio
@pytest.mark.parametrize("metadata", [(), (("authorization", "Bearer bad"),)])
async def test_authentication_failures_are_stable(
    control_server: tuple[str, ControlStub], metadata: Metadata
) -> None:
    # Given/When: 缺失或无效认证调用真实服务。
    _, stub = control_server
    with pytest.raises(aio.AioRpcError) as caught:
        _ = await stub.CreateRun(create_request(), metadata=metadata)
    # Then: 不泄露令牌、protobuf 或验证异常。
    assert caught.value.code() is grpc.StatusCode.UNAUTHENTICATED
    assert caught.value.details() == "身份认证失败"
