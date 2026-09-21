"""程序化拥有 Uvicorn 监听器的 HTTP 生命周期组件."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - Uvicorn 任务必须由其原生 asyncio 循环拥有。
import socket
from collections.abc import Iterator
from contextlib import contextmanager, suppress
from dataclasses import dataclass
from typing import Final, final

import anyio
from fastapi import FastAPI
from pydantic import TypeAdapter
from uvicorn import Config, Server

from scyg_agent.lifecycle import ComponentDiagnostic

from .models import BoundEndpoint, ServerStartError, ServerState

_HTTP_NAME: Final = "http"


def _raise_start_error() -> None:
    """报告 HTTP 启动失败."""
    raise ServerStartError(_HTTP_NAME)


@dataclass(frozen=True, slots=True)
class HttpServerConfig:
    """定义 HTTP 绑定地址与有界启动、关闭时间."""

    host: str
    port: int
    startup_seconds: float
    shutdown_seconds: float


@final
class _ApplicationOwnedSignalServer(Server):
    """禁止 Uvicorn 注册进程信号,信号只由 T21 应用拥有."""

    @contextmanager
    def capture_signals(self) -> Iterator[None]:
        """跳过 Uvicorn 的进程级信号安装."""
        yield


@final
class HttpServerComponent:
    """拥有一个预绑定 socket 和一个 Uvicorn 服务任务."""

    def __init__(self, app: FastAPI, config: HttpServerConfig) -> None:
        """建立尚未绑定的 HTTP 服务组件."""
        self._app = app
        self._config = config
        self._state = ServerState.NEW
        self._endpoint: BoundEndpoint | None = None
        self._server: _ApplicationOwnedSignalServer | None = None
        self._listener: socket.socket | None = None
        self._serve_task: asyncio.Task[None] | None = None
        self._close_lock = asyncio.Lock()
        self._close_task: asyncio.Task[None] | None = None

    @property
    def name(self) -> str:
        """返回稳定的组件名称."""
        return _HTTP_NAME

    @property
    def state(self) -> ServerState:
        """返回当前 HTTP 生命周期状态."""
        return self._state

    @property
    def endpoint(self) -> BoundEndpoint | None:
        """返回实际绑定端点,不暴露 Uvicorn 配置."""
        return self._endpoint

    async def start(self) -> None:
        """预绑定 socket,启动 Uvicorn 并等待实际监听就绪."""
        if self._state is ServerState.RUNNING:
            return
        if self._state is not ServerState.NEW:
            _raise_start_error()
        self._state = ServerState.STARTING
        listener: socket.socket | None = None
        try:
            listener = socket.create_server((self._config.host, self._config.port))
            listener.settimeout(0.0)
            bound_port = TypeAdapter(int).validate_python(listener.getsockname()[1])
            self._endpoint = BoundEndpoint(self._config.host, bound_port)
            self._listener = listener
            server = _ApplicationOwnedSignalServer(
                Config(self._app, log_config=None, lifespan="on", ws="none")
            )
            self._server = server
            self._serve_task = asyncio.create_task(server.serve(sockets=[listener]))
            listener = None
            with anyio.fail_after(self._config.startup_seconds):
                while not server.started:
                    if self._serve_task.done():
                        _ = self._serve_task.result()
                        _raise_start_error()
                    await anyio.sleep(0)  # noqa: ASYNC115 - 轮询 Uvicorn 原生就绪标志.
            self._state = ServerState.RUNNING
        except (OSError, RuntimeError, TimeoutError, ServerStartError):
            if listener is not None:
                listener.close()
            self._state = ServerState.FAILED
            await self._request_close()
            self._state = ServerState.FAILED
            _raise_start_error()
        except anyio.get_cancelled_exc_class():
            if listener is not None:
                listener.close()
            self._state = ServerState.FAILED
            with anyio.CancelScope(shield=True):
                await self._request_close()
            self._state = ServerState.FAILED
            raise

    async def stop(self) -> None:
        """幂等且屏蔽等待者取消地关闭 HTTP 服务."""
        await self._request_close()

    async def close(self) -> None:
        """提供资源风格的幂等关闭别名."""
        await self.stop()

    async def probe(self) -> ComponentDiagnostic:
        """仅根据真实 listener 与服务任务状态报告就绪."""
        ready = (
            self._state is ServerState.RUNNING
            and self._endpoint is not None
            and self._serve_task is not None
            and not self._serve_task.done()
        )
        return ComponentDiagnostic(_HTTP_NAME, ready, "已就绪" if ready else "未就绪")

    async def _request_close(self) -> None:
        """让所有调用方共享同一个不可遗弃清理任务."""
        async with self._close_lock:
            if self._close_task is None:
                self._close_task = asyncio.create_task(self._cleanup())
            task = self._close_task
        await asyncio.shield(task)

    async def _cleanup(self) -> None:
        """先请求优雅退出,超时后强制退出并排空任务."""
        if self._state is ServerState.STOPPED:
            return
        self._state = ServerState.STOPPING
        server, task = self._server, self._serve_task
        if server is not None:
            server.should_exit = True
        if task is not None:
            with anyio.move_on_after(self._config.shutdown_seconds, shield=True) as scope:
                await task
            if scope.cancelled_caught and not task.done():
                if server is not None:
                    server.force_exit = True
                _ = task.cancel()
                with anyio.CancelScope(shield=True), suppress(asyncio.CancelledError):
                    await task
        if self._listener is not None:
            self._listener.close()
            self._listener = None
        self._endpoint = None
        self._state = ServerState.STOPPED
