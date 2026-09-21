"""使用真实本地 HTTP 连接验证 OpenAI 兼容 SSE 完整性."""

from collections.abc import Iterator
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread
from typing import Final

import httpx
import pytest
from pydantic import SecretStr

from scyg_agent.runtimes.simple.models import CompletionRequest, Message, MessageRole
from scyg_agent.runtimes.simple.provider import OpenAICompatibleProvider, ProviderConfig
from scyg_agent.runtimes.simple.results import (
    CompletionFinished,
    FailureKind,
    ProviderDelta,
    ProviderFailure,
)

API_KEY: Final = "local-http-secret"
BASE_STREAM: Final = (
    'data: {"choices":[{"delta":{"content":"ok"},"finish_reason":null}]}\n\n'
    'data: {"choices":[{"delta":{},"finish_reason":"stop"}]}\n\n'
    "data: [DONE]\n\n"
)
USAGE_FRAME: Final = (
    'data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}\n\n'
)
REQUEST: Final = CompletionRequest(
    model="test-model",
    messages=(Message(role=MessageRole.USER, content="本地 HTTP 回归"),),
)


class SseHandler(BaseHTTPRequestHandler):
    """返回测试注入的 SSE 正文并记录真实请求次数."""

    # body 由每个隔离服务器子类设置。
    body: bytes = b""
    # request_count 记录服务端实际收到的 POST 次数。
    request_count: int = 0

    def do_POST(self) -> None:
        """返回完整 SSE 响应."""
        type(self).request_count += 1
        content_length = int(self.headers.get("Content-Length", "0"))
        _ = self.rfile.read(content_length)
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(self.body)))
        self.end_headers()
        _ = self.wfile.write(self.body)


@contextmanager
def local_sse_server(body: str) -> Iterator[tuple[str, type[SseHandler]]]:
    """启动并可靠关闭仅监听回环地址的有界 HTTP 服务."""
    handler = type("BoundSseHandler", (SseHandler,), {"body": body.encode(), "request_count": 0})
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        address = server.server_address
        host, port = str(address[0]), int(address[1])
        yield f"http://{host}:{port}", handler
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def provider_for(base_url: str) -> OpenAICompatibleProvider:
    """构造连接真实本地服务器的提供方."""
    client = httpx.AsyncClient(base_url=base_url)
    return OpenAICompatibleProvider(
        client,
        ProviderConfig(SecretStr(API_KEY), retries=1, timeout_seconds=2),
    )


@pytest.fixture
def anyio_backend() -> str:
    """固定异步测试后端."""
    return "asyncio"


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("trailing", "expected_kind"),
    [
        ("data: [DONE]\n\n", FailureKind.DUPLICATE_TERMINAL),
        ('data: {"choices":[{"delta":{"content":"late"}}]}\n\n', FailureKind.MALFORMED),
        ("data: not-json\n\n", FailureKind.MALFORMED),
        ("TRAILING-BYTES", FailureKind.MALFORMED),
    ],
)
async def test_real_http_rejects_every_non_ignorable_frame_after_done(
    trailing: str,
    expected_kind: FailureKind,
) -> None:
    # Given: 真实 HTTP 服务在首个 DONE 后继续发送不可忽略数据。
    with local_sse_server(f"{BASE_STREAM}{trailing}") as (base_url, handler):
        provider = provider_for(base_url)

        # When: 完整消费网络响应。
        results = [result async for result in provider.stream(REQUEST)]
        await provider.aclose()

    # Then: 保留已提交增量但终态为失败, 且请求绝不重放。
    assert results == [ProviderDelta("ok"), ProviderFailure(expected_kind)]
    assert handler.request_count == 1


@pytest.mark.anyio
async def test_real_http_accepts_usage_only_frame_and_preserves_exact_usage() -> None:
    # Given: 标准 finish 后 usage-only 帧和 DONE。
    stream = BASE_STREAM.replace("data: [DONE]\n\n", f"{USAGE_FRAME}data: [DONE]\n\n")
    with local_sse_server(stream) as (base_url, handler):
        provider = provider_for(base_url)

        # When: 完整消费真实 HTTP 响应。
        results = [result async for result in provider.stream(REQUEST)]
        await provider.aclose()

    # Then: 精确 usage 进入唯一成功终态且只有一次请求。
    assert results == [
        ProviderDelta("ok"),
        CompletionFinished("stop", prompt_tokens=3, completion_tokens=2, total_tokens=5),
    ]
    assert handler.request_count == 1
