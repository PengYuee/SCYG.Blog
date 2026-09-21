"""AgentControl 公共 RPC 拒绝、结果和生命周期矩阵."""

from collections.abc import Callable

import anyio
import grpc
import pytest
from cryptography.hazmat.primitives.asymmetric import rsa
from grpc import aio

from scyg_agent.application import (
    FacadeCancelled,
    FacadeConflict,
    FacadeInternal,
    FacadeNotFound,
    FacadePrecondition,
    FacadeValidation,
)
from scyg_agent.domain.runs import RunId
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import common_pb2
from tests.adapters.auth.test_jwt_verifier import claims, encode

from .scenario_support import CreateOutcome, GetOutcome, Metadata, RpcHarness
from .test_agent_control_stub import create_request

RequestMutation = Callable[[service_pb2.CreateRunRequest], None]


def blog_metadata(
    private_key: rsa.RSAPrivateKey,
    payload: dict[str, str | int | list[str]] | None = None,
) -> Metadata:
    """签发测试 Blog token 并返回规范 metadata."""
    token = encode(private_key, payload or claims("blog_service")).value
    return (("authorization", f"Bearer {token}"),)


def get_request(run_id: str = "run_12345678") -> service_pb2.GetRunRequest:
    """构造语义完整的读取请求."""
    return service_pb2.GetRunRequest(
        metadata=common_pb2.RequestMetadata(request_id="get-1", correlation_id="trace-1"),
        run_id=common_pb2.RunId(value=run_id),
        owner_user_id=common_pb2.UserId(value="user-t20"),
    )


@pytest.mark.anyio
async def test_duplicate_metadata_and_web_principal_are_rejected(
    rpc_harness: RpcHarness, private_key: rsa.RSAPrivateKey
) -> None:
    # Given: 重复授权值和仅绑定单 Run 的 Web token。
    blog = blog_metadata(private_key)
    web = blog_metadata(private_key, claims())
    cases = (
        (blog + blog, grpc.StatusCode.UNAUTHENTICATED),
        (web, grpc.StatusCode.PERMISSION_DENIED),
    )
    # When/Then: 两种调用均经真实 server 返回稳定且无敏感信息的状态。
    for metadata, expected in cases:
        with pytest.raises(aio.AioRpcError) as caught:
            _ = await rpc_harness.stub.CreateRun(create_request(), metadata=metadata)
        assert caught.value.code() is expected
        assert "Bearer" not in (caught.value.details() or "")
        assert "run_12345678" not in (caught.value.details() or "")


@pytest.mark.anyio
async def test_missing_create_or_read_scope_is_rejected(
    rpc_harness: RpcHarness, private_key: rsa.RSAPrivateKey
) -> None:
    # Given: 分别缺失 create 或 read scope 的 Blog service JWT。
    for scope in (["agent:runs:read"], ["agent:runs:create"]):
        payload = claims("blog_service")
        payload["scope"] = scope
        with pytest.raises(aio.AioRpcError) as caught:
            _ = await rpc_harness.stub.CreateRun(
                create_request(), metadata=blog_metadata(private_key, payload)
            )
        assert caught.value.code() is grpc.StatusCode.UNAUTHENTICATED
        assert caught.value.details() == "身份认证失败"


def invalid_create_requests() -> tuple[service_pb2.CreateRunRequest, ...]:
    """构造覆盖全部创建语义边界的畸形 protobuf."""
    missing_metadata = create_request()
    missing_metadata.ClearField("metadata")
    bad_owner = create_request()
    bad_owner.owner_user_id.value = ""
    bad_task = create_request()
    bad_task.task_type = common_pb2.TASK_TYPE_UNSPECIFIED
    bad_runtime = create_request()
    bad_runtime.runtime.kind = common_pb2.RUNTIME_KIND_UNSPECIFIED
    bad_version = create_request()
    bad_version.runtime.version = "latest"
    empty_message = create_request()
    empty_message.initial_message = ""
    large_message = create_request()
    large_message.initial_message = "x" * 16_001
    empty_article = create_request()
    empty_article.article_id.value = ""
    bad_operation = create_request()
    bad_operation.operation_id = " bad"
    return (
        missing_metadata,
        bad_owner,
        bad_task,
        bad_runtime,
        bad_version,
        empty_message,
        large_message,
        empty_article,
        bad_operation,
    )


@pytest.mark.anyio
async def test_invalid_create_semantics_map_invalid_argument(
    rpc_harness: RpcHarness, private_key: rsa.RSAPrivateKey
) -> None:
    metadata = blog_metadata(private_key)
    for request in invalid_create_requests():
        with pytest.raises(aio.AioRpcError) as caught:
            _ = await rpc_harness.stub.CreateRun(request, metadata=metadata)
        assert caught.value.code() is grpc.StatusCode.INVALID_ARGUMENT
        assert caught.value.details() == "创建 Run 的请求参数无效"
    assert rpc_harness.facade.calls == 0


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("outcome", "code", "detail"),
    [
        (FacadeConflict(), grpc.StatusCode.ALREADY_EXISTS, "创建请求与已有事实冲突"),
        (FacadePrecondition(), grpc.StatusCode.FAILED_PRECONDITION, "Run 当前状态不允许此操作"),
        (FacadeValidation(), grpc.StatusCode.INVALID_ARGUMENT, "请求参数无效"),
        (FacadeCancelled(), grpc.StatusCode.CANCELLED, "请求已取消"),
        (FacadeInternal(), grpc.StatusCode.INTERNAL, "服务内部错误"),
    ],
)
async def test_create_outcomes_map_stable_status(
    rpc_harness: RpcHarness,
    private_key: rsa.RSAPrivateKey,
    outcome: CreateOutcome,
    code: grpc.StatusCode,
    detail: str,
) -> None:
    rpc_harness.facade.create_outcome = outcome
    with pytest.raises(aio.AioRpcError) as caught:
        _ = await rpc_harness.stub.CreateRun(create_request(), metadata=blog_metadata(private_key))
    assert caught.value.code() is code
    assert caught.value.details() == detail


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("outcome", "code"),
    [
        (FacadeNotFound(RunId("run_12345678")), grpc.StatusCode.NOT_FOUND),
        (FacadePrecondition(), grpc.StatusCode.FAILED_PRECONDITION),
        (FacadeValidation(), grpc.StatusCode.INVALID_ARGUMENT),
        (FacadeCancelled(), grpc.StatusCode.CANCELLED),
        (FacadeInternal(), grpc.StatusCode.INTERNAL),
    ],
)
async def test_get_outcomes_map_stable_status(
    rpc_harness: RpcHarness,
    private_key: rsa.RSAPrivateKey,
    outcome: GetOutcome,
    code: grpc.StatusCode,
) -> None:
    rpc_harness.facade.get_outcome = outcome
    with pytest.raises(aio.AioRpcError) as caught:
        _ = await rpc_harness.stub.GetRun(get_request(), metadata=blog_metadata(private_key))
    assert caught.value.code() is code
    assert caught.value.details()


@pytest.mark.anyio
async def test_deadline_and_cancellation_drain_facade(
    rpc_harness: RpcHarness, private_key: rsa.RSAPrivateKey
) -> None:
    rpc_harness.facade.block = True
    metadata = blog_metadata(private_key)
    with pytest.raises(aio.AioRpcError) as deadline:
        _ = await rpc_harness.stub.CreateRun(create_request(), metadata=metadata, timeout=0.01)
    assert deadline.value.code() is grpc.StatusCode.DEADLINE_EXCEEDED
    with anyio.fail_after(1):
        await rpc_harness.facade.drained.wait()
    rpc_harness.facade.started = anyio.Event()
    rpc_harness.facade.drained = anyio.Event()
    call = rpc_harness.stub.CreateRun(create_request("operation-2"), metadata=metadata)
    await rpc_harness.facade.started.wait()
    assert call.cancel()
    with pytest.raises(anyio.get_cancelled_exc_class()):
        _ = await call
    with anyio.fail_after(1):
        await rpc_harness.facade.drained.wait()


@pytest.mark.anyio
async def test_invalid_get_id_and_causation_never_call_facade(
    rpc_harness: RpcHarness, private_key: rsa.RSAPrivateKey
) -> None:
    """读取语义错误通过真实 stub 稳定映射且不进入门面."""
    invalid_id = get_request("")
    invalid_causation = get_request()
    invalid_causation.metadata.causation_id = " bad"
    for request in (invalid_id, invalid_causation):
        with pytest.raises(aio.AioRpcError) as caught:
            _ = await rpc_harness.stub.GetRun(request, metadata=blog_metadata(private_key))
        assert caught.value.code() is grpc.StatusCode.INVALID_ARGUMENT
        assert caught.value.details() == "读取 Run 的请求参数无效"
    assert rpc_harness.facade.calls == 0
