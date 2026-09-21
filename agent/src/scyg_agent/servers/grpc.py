"""程序化拥有生成 AgentControl 注册的 grpc.aio 生命周期组件."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - grpc.aio 服务任务属于 asyncio 后端。
from dataclasses import dataclass
from typing import TYPE_CHECKING, Final, final

import anyio
from grpc import aio

from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc
from scyg_agent.lifecycle import ComponentDiagnostic
from scyg_agent.transport.grpc import AgentControlServicer

from .models import BoundEndpoint, ServerStartError, ServerState

_GRPC_NAME: Final = "grpc"


def _raise_start_error() -> None:
    """报告 gRPC 启动失败."""
    raise ServerStartError(_GRPC_NAME)


if TYPE_CHECKING:

    def _register(_servicer: AgentControlServicer, _server: aio.Server) -> None: ...
else:
    _register = service_grpc.add_AgentControlServiceServicer_to_server


@dataclass(frozen=True, slots=True)
class GrpcServerConfig:
    """定义 gRPC 绑定地址、启动期限和优雅宽限."""

    host: str
    port: int
    startup_seconds: float
    grace_seconds: float


@final
class GrpcServerComponent:
    """拥有一个注册 AgentControl 的 grpc.aio 服务."""

    def __init__(self, servicer: AgentControlServicer, config: GrpcServerConfig) -> None:
        """建立尚未绑定的 gRPC 服务组件."""
        self._servicer = servicer
        self._config = config
        self._state = ServerState.NEW
        self._endpoint: BoundEndpoint | None = None
        self._server: aio.Server | None = None
        self._close_lock = asyncio.Lock()
        self._close_task: asyncio.Task[None] | None = None

    @property
    def name(self) -> str:
        """返回稳定的组件名称."""
        return _GRPC_NAME

    @property
    def state(self) -> ServerState:
        """返回当前 gRPC 生命周期状态."""
        return self._state

    @property
    def endpoint(self) -> BoundEndpoint | None:
        """返回 gRPC runtime 报告的实际绑定端点."""
        return self._endpoint

    async def start(self) -> None:
        """注册生成服务、绑定精确地址并等待启动完成."""
        if self._state is ServerState.RUNNING:
            return
        if self._state is not ServerState.NEW:
            _raise_start_error()
        self._state = ServerState.STARTING
        server = aio.server()
        self._server = server
        _register(self._servicer, server)
        try:
            port = server.add_insecure_port(f"{self._config.host}:{self._config.port}")
            if port == 0:
                _raise_start_error()
            self._endpoint = BoundEndpoint(self._config.host, port)
            with anyio.fail_after(self._config.startup_seconds):
                await server.start()
            self._state = ServerState.RUNNING
        except (RuntimeError, OSError, TimeoutError, ServerStartError):
            self._state = ServerState.FAILED
            await self._request_close()
            self._state = ServerState.FAILED
            _raise_start_error()
        except anyio.get_cancelled_exc_class():
            self._state = ServerState.FAILED
            with anyio.CancelScope(shield=True):
                await self._request_close()
            self._state = ServerState.FAILED
            raise

    async def stop(self) -> None:
        """停止接收新 RPC 并共享一次有宽限的关闭."""
        await self._request_close()

    async def close(self) -> None:
        """提供资源风格的幂等关闭别名."""
        await self.stop()

    async def probe(self) -> ComponentDiagnostic:
        """仅在实际端口已绑定且服务运行时报告就绪."""
        ready = self._state is ServerState.RUNNING and self._endpoint is not None
        return ComponentDiagnostic(_GRPC_NAME, ready, "已就绪" if ready else "未就绪")

    async def _request_close(self) -> None:
        """让并发调用方共享一个屏蔽取消的清理任务."""
        async with self._close_lock:
            if self._close_task is None:
                self._close_task = asyncio.create_task(self._cleanup())
            task = self._close_task
        await asyncio.shield(task)

    async def _cleanup(self) -> None:
        """先撤销接受,再等待 gRPC 完成有界优雅停止."""
        if self._state is ServerState.STOPPED:
            return
        self._state = ServerState.STOPPING
        if self._server is not None:
            with anyio.CancelScope(shield=True):
                await self._server.stop(self._config.grace_seconds)
                _ = await self._server.wait_for_termination(timeout=self._config.grace_seconds)
        self._endpoint = None
        self._state = ServerState.STOPPED
