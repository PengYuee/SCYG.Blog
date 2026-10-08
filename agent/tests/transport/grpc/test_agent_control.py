"""Five-RPC transport contract, public errors, and explicit streaming readiness."""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import anyio
import grpc
import pytest
from grpc import aio
from grpc_status import rpc_status

from scyg_agent.application.control import ControlError
from scyg_agent.generated.proto.scyg.agent.v1 import agent_control_service_pb2 as pb
from scyg_agent.generated.proto.scyg.agent.v1 import agent_control_service_pb2_grpc as rpc
from scyg_agent.generated.proto.scyg.agent.v1 import common_pb2 as common
from scyg_agent.transport.grpc import AgentControlServicer
from scyg_agent.transport.grpc.conversion import InvalidGrpcRequestError, validate_json
from tests.transport.grpc.scenario_support import OpenedStream, ScenarioFacade

KEY = "8fe8c687-7553-4c81-8916-42bd285c6457"


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@asynccontextmanager
async def serve(facade: ScenarioFacade) -> AsyncIterator[rpc.AgentControlServiceStub]:
    server = aio.server()
    rpc.add_AgentControlServiceServicer_to_server(AgentControlServicer(facade), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = aio.insecure_channel(f"127.0.0.1:{port}")
    try:
        yield rpc.AgentControlServiceStub(channel)
    finally:
        await channel.close()
        await server.stop(0)


@pytest.mark.anyio
async def test_subscription_metadata_precedes_frames_and_closed_resource() -> None:
    facade = ScenarioFacade()
    async with serve(facade) as stub:
        call = stub.StreamRunEvents(pb.StreamRunEventsRequest(user_id="user", run_id="safe-run"))
        assert ("scyg-subscription-ready", "1") in tuple(await call.initial_metadata())
        frames = [frame.frame async for frame in call]
    assert frames == [b": heartbeat\n\n"]
    assert facade.opened.closed


@pytest.mark.anyio
async def test_stream_failure_has_no_ready_marker_and_public_detail() -> None:
    facade = ScenarioFacade(error=ControlError("NOT_FOUND", "Run not found"))
    async with serve(facade) as stub:
        call = stub.StreamRunEvents(pb.StreamRunEventsRequest(user_id="user", run_id="safe-run"))
        headers = tuple(await call.initial_metadata())
        assert ("scyg-subscription-ready", "1") not in headers
        with pytest.raises(aio.AioRpcError) as caught:
            _ = await call.read()
    assert caught.value.code() is grpc.StatusCode.NOT_FOUND
    status = rpc_status.from_call(caught.value)
    assert status is not None
    detail = common.PublicError()
    assert status.details[0].Unpack(detail)
    assert detail.code == "NOT_FOUND"


@pytest.mark.anyio
async def test_transport_invalid_json_never_calls_business_and_unknown_errors_sanitized() -> None:
    facade = ScenarioFacade()
    async with serve(facade) as stub:
        with pytest.raises(aio.AioRpcError) as invalid:
            _ = await stub.CreateRun(
                pb.CreateRunRequest(
                    user_id="user",
                    idempotency_key=KEY,
                    capability=common.AGENT_CAPABILITY_CHAT,
                    json_payload=b"NaN",
                )
            )
        assert invalid.value.code() is grpc.StatusCode.INVALID_ARGUMENT
        assert not facade.calls
        facade.error = RuntimeError("database password secret")
        with pytest.raises(aio.AioRpcError) as internal:
            _ = await stub.GetRun(pb.GetRunRequest(user_id="user", run_id="safe-run"))
        assert internal.value.code() is grpc.StatusCode.INTERNAL
        details = internal.value.details()
        assert details is not None
        assert "secret" not in details


@pytest.mark.parametrize(
    "raw",
    [b"", b"{} {}", b"Infinity", b"\xff", b"x" * 1_048_577],
    ids=["empty", "multiple-values", "non-finite", "invalid-utf8", "too-large"],
)
def test_transport_rejects_invalid_json(raw: bytes) -> None:
    with pytest.raises(InvalidGrpcRequestError):
        _ = validate_json(raw)


@pytest.mark.parametrize("raw", [b"null", b"[]", b"1", b'"text"', b"{}"])
def test_transport_accepts_any_json_shape_without_business_parsing(raw: bytes) -> None:
    assert validate_json(raw) is raw


@pytest.mark.anyio
async def test_ready_does_not_wait_for_first_frame_and_cancel_closes_subscription() -> None:
    facade = ScenarioFacade(opened=OpenedStream(ready_to_emit=anyio.Event()))
    async with serve(facade) as stub:
        call = stub.StreamRunEvents(pb.StreamRunEventsRequest(user_id="user", run_id="safe-run"))
        with anyio.fail_after(1):
            headers = tuple(await call.initial_metadata())
        assert ("scyg-subscription-ready", "1") in headers
        assert call.cancel()
    assert facade.opened.closed
