"""真实本地监听器的 T21 生命周期验收."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - 被测组件固定运行于 asyncio 后端.
import socket
from contextlib import closing

import anyio
import pytest
from fastapi import FastAPI
from grpc import aio
from grpc_health.v1 import health_pb2, health_pb2_grpc
from pydantic import TypeAdapter
from uvicorn import Server

from scyg_agent.servers import (
    GrpcServerComponent,
    GrpcServerConfig,
    HttpServerComponent,
    HttpServerConfig,
    ServerStartError,
    ServerState,
)
from scyg_agent.transport.grpc import AgentControlServicer
from tests.transport.grpc.scenario_support import ScenarioFacade

_HOST = "127.0.0.1"


def _http_component(app: FastAPI, port: int = 0) -> HttpServerComponent:
    return HttpServerComponent(app, HttpServerConfig(_HOST, port, 2.0, 2.0))


def _grpc_component(port: int = 0) -> GrpcServerComponent:
    servicer = AgentControlServicer(ScenarioFacade())
    return GrpcServerComponent(servicer, GrpcServerConfig(_HOST, port, 2.0, 0.1))


async def _request_http(port: int) -> bytes:
    stream = await anyio.connect_tcp(_HOST, port)
    async with stream:
        await stream.send(b"GET /live HTTP/1.1\r\nHost: local\r\nConnection: close\r\n\r\n")
        chunks = [chunk async for chunk in stream]
    return b"".join(chunks)


def test_http_ephemeral_request_and_concurrent_close() -> None:
    async def scenario() -> None:
        app = FastAPI()

        @app.get("/live")
        async def live() -> dict[str, bool]:
            return {"alive": True}

        _ = live

        component = _http_component(app)
        await component.start()
        endpoint = component.endpoint
        assert endpoint is not None
        assert endpoint.port > 0
        assert component.state is ServerState.RUNNING
        assert component.name == "http"
        assert (await component.probe()).ready
        response = await _request_http(endpoint.port)
        assert b"200 OK" in response
        assert b'{"alive":true}' in response
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(component.close)
            _ = tasks.start_soon(component.stop)
        await component.close()
        assert component.state is ServerState.STOPPED
        assert component.endpoint is None
        with pytest.raises(OSError, match="."):
            _ = await anyio.connect_tcp(_HOST, endpoint.port)

    anyio.run(scenario)


def test_http_bind_conflict_leaves_no_owned_listener() -> None:
    async def scenario() -> None:
        with closing(socket.create_server((_HOST, 0))) as occupied:
            port = TypeAdapter(int).validate_python(occupied.getsockname()[1])
            component = _http_component(FastAPI(), port)
            with pytest.raises(ServerStartError, match="HTTP|http"):
                await component.start()
            assert component.state is ServerState.FAILED
            assert component.endpoint is None
            await component.close()

    anyio.run(scenario)


def test_http_close_waiter_cancellation_does_not_abandon_cleanup() -> None:
    async def scenario() -> None:
        component = _http_component(FastAPI())
        await component.start()
        waiter = asyncio.create_task(component.close())
        _ = waiter.cancel()
        with pytest.raises(asyncio.CancelledError):
            await waiter
        await component.close()
        assert component.state is ServerState.STOPPED

    anyio.run(scenario)


def test_grpc_ephemeral_ready_and_concurrent_close() -> None:
    async def scenario() -> None:
        component = _grpc_component()
        await component.start()
        endpoint = component.endpoint
        assert endpoint is not None
        assert endpoint.port > 0
        assert component.name == "grpc"
        channel = aio.insecure_channel(f"{endpoint.host}:{endpoint.port}")
        try:
            await channel.channel_ready()
            health = health_pb2_grpc.HealthStub(channel)
            response = await health.Check(
                health_pb2.HealthCheckRequest(service="scyg.agent.v1.AgentControlService"),
            )
            assert response.status == health_pb2.HealthCheckResponse.SERVING
            assert (await component.probe()).ready
        finally:
            await channel.close()
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(component.stop)
            _ = tasks.start_soon(component.close)
        await component.close()
        assert component.state is ServerState.STOPPED
        assert component.endpoint is None

    anyio.run(scenario)


def test_grpc_bind_conflict_is_typed() -> None:
    async def scenario() -> None:
        with closing(socket.create_server((_HOST, 0))) as occupied:
            port = TypeAdapter(int).validate_python(occupied.getsockname()[1])
            component = _grpc_component(port)
            with pytest.raises(ServerStartError, match="grpc"):
                await component.start()
            assert component.state is ServerState.FAILED
            assert component.endpoint is None
            await component.stop()

    anyio.run(scenario)


def test_grpc_close_waiter_cancellation_does_not_abandon_cleanup() -> None:
    async def scenario() -> None:
        component = _grpc_component()
        await component.start()
        waiter = asyncio.create_task(component.close())
        _ = waiter.cancel()
        with pytest.raises(asyncio.CancelledError):
            await waiter
        await component.close()
        assert component.state is ServerState.STOPPED

    anyio.run(scenario)


def test_http_start_is_idempotent_and_restart_after_stop_is_rejected() -> None:
    async def scenario() -> None:
        component = _http_component(FastAPI())
        await component.start()
        endpoint = component.endpoint
        await component.start()
        assert component.endpoint == endpoint
        await component.stop()
        with pytest.raises(ServerStartError):
            await component.start()

    anyio.run(scenario)


def test_http_stop_before_start_is_idempotent() -> None:
    async def scenario() -> None:
        component = _http_component(FastAPI())
        await component.stop()
        await component.stop()
        assert component.state is ServerState.STOPPED
        assert not (await component.probe()).ready

    anyio.run(scenario)


def test_grpc_start_is_idempotent_and_restart_after_stop_is_rejected() -> None:
    async def scenario() -> None:
        component = _grpc_component()
        await component.start()
        endpoint = component.endpoint
        await component.start()
        assert component.endpoint == endpoint
        await component.stop()
        with pytest.raises(ServerStartError):
            await component.start()

    anyio.run(scenario)


def test_grpc_stop_before_start_is_idempotent() -> None:
    async def scenario() -> None:
        component = _grpc_component()
        await component.stop()
        await component.stop()
        assert component.state is ServerState.STOPPED
        assert not (await component.probe()).ready

    anyio.run(scenario)


def test_http_start_cancellation_closes_listener_and_drains_task(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    serving = anyio.Event()

    async def stalled_serve(_server: Server, sockets: list[socket.socket] | None = None) -> None:
        del sockets
        serving.set()
        await anyio.sleep_forever()

    monkeypatch.setattr(
        "scyg_agent.servers.http._ApplicationOwnedSignalServer.serve", stalled_serve
    )

    async def scenario() -> None:
        component = HttpServerComponent(FastAPI(), HttpServerConfig(_HOST, 0, 2.0, 0.01))
        starter = asyncio.create_task(component.start())
        await serving.wait()
        endpoint = component.endpoint
        assert endpoint is not None
        _ = starter.cancel()
        with pytest.raises(asyncio.CancelledError):
            await starter
        assert component.state is ServerState.FAILED
        assert component.endpoint is None
        with pytest.raises(OSError, match="."):
            _ = await anyio.connect_tcp(_HOST, endpoint.port)
        await component.close()

    anyio.run(scenario)
