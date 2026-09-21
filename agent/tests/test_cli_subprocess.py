"""T21 CLI 子进程生命周期验收."""

import os
import signal
import socket
import subprocess
import sys
import textwrap
import time
from pathlib import Path
from typing import Protocol

import httpx
import pytest
from pydantic import TypeAdapter

from scyg_agent.composition import ReadinessResponse
from tests.acceptance_settings import require_test_settings

_AGENT_ROOT = Path(__file__).parents[1]
_HOST = "127.0.0.1"


def _port() -> int:
    with socket.create_server((_HOST, 0)) as listener:
        return TypeAdapter(int).validate_python(listener.getsockname()[1])


def _stop(process: subprocess.Popen[str]) -> None:
    if process.poll() is not None:
        return
    process.terminate()
    try:
        _ = process.wait(timeout=5)
    except subprocess.TimeoutExpired:
        process.kill()
        _ = process.wait(timeout=5)


class ProcessStatus(Protocol):
    """提供 readiness 轮询所需的最小子进程状态."""

    def poll(self) -> int | None:
        """返回退出码或仍运行的 None."""
        ...


def _poll_readiness(port: int, process: ProcessStatus, deadline: float) -> bool:
    """在既有截止期内仅重试启动期连接拒绝和连接超时."""
    while time.monotonic() < deadline and process.poll() is None:
        try:
            response = httpx.get(f"http://{_HOST}:{port}/health/ready", timeout=0.5)
            _ = response.raise_for_status()
            if ReadinessResponse.model_validate_json(response.content).ready:
                return True
        except (httpx.ConnectTimeout, httpx.ConnectError, OSError, TimeoutError):
            time.sleep(0.05)
    return False


def test_startup_failure_subprocess_is_bounded_and_secret_free() -> None:
    environment = os.environ | {
        "SCYG_AGENT_CONFIG_FILE": str(_AGENT_ROOT / "missing-subprocess-agent.toml"),
        "SCYG_AGENT_DATABASE_URL": "invalid://sentinel-user:sentinel-password@db/service",
        "SCYG_AGENT_JWT_PUBLIC_KEY_PATH": "sentinel-key.pem",
        "SCYG_AGENT_PROVIDER_BASE_URL": "https://provider.invalid/v1",
        "SCYG_AGENT_PROVIDER_API_KEY": "sentinel-provider-key",
        "SCYG_AGENT_PROVIDER_MODEL": "sentinel-model",
    }
    result = subprocess.run(  # noqa: S603
        [sys.executable, "-m", "scyg_agent", "run"],
        cwd=_AGENT_ROOT,
        env=environment,
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
    )
    output = result.stdout + result.stderr
    assert result.returncode == 2
    assert "SCYG Agent 配置无效" in output
    assert "sentinel-password" not in output
    assert "sentinel-provider-key" not in output


def test_real_subprocess_ready_grpc_and_sigterm_exit() -> None:
    settings = require_test_settings()
    database_url = settings.normal_url
    public_key = str(settings.require_jwt_public_key_path())
    http_port, grpc_port = _port(), _port()
    environment = os.environ | {
        "SCYG_AGENT_CONFIG_FILE": str(_AGENT_ROOT / "missing-subprocess-agent.toml"),
        "SCYG_AGENT_DATABASE_URL": database_url,
        "SCYG_AGENT_JWT_PUBLIC_KEY_PATH": public_key,
        "SCYG_AGENT_PROVIDER_BASE_URL": "http://127.0.0.1:9/v1",
        "SCYG_AGENT_PROVIDER_API_KEY": "local-provider-key",
        "SCYG_AGENT_PROVIDER_MODEL": "local-model",
        "SCYG_AGENT_BLOG_GRPC_TARGET": "127.0.0.1:9",
        "SCYG_AGENT_HTTP_PORT": str(http_port),
        "SCYG_AGENT_GRPC_PORT": str(grpc_port),
        "SCYG_AGENT_SHUTDOWN_SECONDS": "5",
        "SCYG_AGENT_HEARTBEAT_SECONDS": "5",
    }
    creation_flags = subprocess.CREATE_NEW_PROCESS_GROUP if sys.platform == "win32" else 0
    process = subprocess.Popen(  # noqa: S603
        [sys.executable, "-m", "scyg_agent", "run"],
        cwd=_AGENT_ROOT,
        env=environment,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        creationflags=creation_flags,
    )
    try:
        deadline = time.monotonic() + 15
        assert _poll_readiness(http_port, process, deadline)
        with socket.create_connection((_HOST, grpc_port), timeout=1):
            pass
        if sys.platform == "win32":
            process.send_signal(signal.CTRL_BREAK_EVENT)
        else:
            os.kill(process.pid, signal.SIGTERM)
        assert process.wait(timeout=8) == 0
    finally:
        _stop(process)
        stdout, stderr = process.communicate(timeout=1)
    output = stdout + stderr
    assert database_url not in output
    assert "local-provider-key" not in output


class RunningProcess:
    """始终报告运行中的确定性进程状态."""

    def poll(self) -> int | None:
        return None


def test_poll_retries_connect_timeout_and_error_before_ready(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    request = httpx.Request("GET", "http://local/health/ready")
    outcomes: list[httpx.Response | httpx.ConnectTimeout | httpx.ConnectError] = [
        httpx.ConnectTimeout("startup timeout", request=request),
        httpx.ConnectError("startup refusal", request=request),
        httpx.Response(
            200,
            request=request,
            content=ReadinessResponse(ready=True, components=()).model_dump_json(),
        ),
    ]
    pauses: list[float] = []

    def get(_url: str, *, timeout: float) -> httpx.Response:
        assert timeout == 0.5
        outcome = outcomes.pop(0)
        if isinstance(outcome, httpx.Response):
            return outcome
        raise outcome

    monkeypatch.setattr(httpx, "get", get)
    monkeypatch.setattr(time, "sleep", pauses.append)

    assert _poll_readiness(8080, RunningProcess(), time.monotonic() + 15)
    assert pauses == [0.05, 0.05]
    assert outcomes == []


@pytest.mark.parametrize("failure", ["status", "protocol"])
def test_poll_does_not_retry_non_transient_http_failures(
    monkeypatch: pytest.MonkeyPatch, failure: str
) -> None:
    request = httpx.Request("GET", "http://local/health/ready")
    calls = 0
    pauses: list[float] = []
    protocol_error = httpx.RemoteProtocolError("invalid response", request=request)

    def get(_url: str, *, timeout: float) -> httpx.Response:
        nonlocal calls
        assert timeout == 0.5
        calls += 1
        if failure == "protocol":
            raise protocol_error
        return httpx.Response(503, request=request)

    monkeypatch.setattr(httpx, "get", get)
    monkeypatch.setattr(time, "sleep", pauses.append)
    expected = httpx.RemoteProtocolError if failure == "protocol" else httpx.HTTPStatusError

    with pytest.raises(expected):
        _ = _poll_readiness(8080, RunningProcess(), time.monotonic() + 15)
    assert calls == 1
    assert pauses == []


def test_child_configures_platform_policy_before_async_loop() -> None:
    script = textwrap.dedent(
        """
        import asyncio
        import sys
        import signal
        import anyio
        from scyg_agent.__main__ import configure_event_loop_policy
        from scyg_agent.cli_runtime import ShutdownController, wait_for_process_signal

        original = asyncio.get_event_loop_policy()
        configure_event_loop_policy()
        if sys.platform != "win32":
            assert asyncio.get_event_loop_policy() is original

        previous = signal.getsignal(signal.SIGINT)

        async def verify() -> None:
            if sys.platform == "win32":
                loop_name = type(asyncio.get_running_loop()).__name__
                assert "Selector" in loop_name
                assert "Proactor" not in loop_name
            controller = ShutdownController()
            async with anyio.create_task_group() as tasks:
                tasks.start_soon(wait_for_process_signal, controller)
                await anyio.sleep(0.05)
                signal.raise_signal(signal.SIGINT)
            assert controller.requested

        anyio.run(verify)
        assert signal.getsignal(signal.SIGINT) == previous
        """
    )
    result = subprocess.run(  # noqa: S603
        [sys.executable, "-c", script],
        cwd=_AGENT_ROOT,
        capture_output=True,
        text=True,
        timeout=10,
        check=False,
    )
    assert result.returncode == 0, result.stderr
