"""OpenAI 兼容流式提供方的 HTTP 边界测试。"""

from collections.abc import AsyncIterator
from typing import Final, override

import anyio
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

API_KEY: Final = "t15-super-secret"
SUCCESS_BODY: Final = 'data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}\n\n'
DONE_BODY: Final = "data: [DONE]\n\n"
DUPLICATE_BODY: Final = 'data: {"choices":[{"delta":{"content":"x"},"finish_reason":"stop"}]}\n\n'
DUPLICATE_FINISH_BODY: Final = 'data: {"choices":[{"delta":{},"finish_reason":"stop"}]}\n\n'
REQUEST: Final = CompletionRequest(
    model="test-model",
    messages=(Message(role=MessageRole.USER, content="待处理正文"),),
)


def _provider(handler: httpx.AsyncBaseTransport, *, retries: int = 1) -> OpenAICompatibleProvider:
    """构造使用 HTTP 传输假服务的真实提供方。"""
    client = httpx.AsyncClient(transport=handler, base_url="https://provider.test/v1")
    config = ProviderConfig(SecretStr(API_KEY), retries=retries, timeout_seconds=1)
    return OpenAICompatibleProvider(client, config)


@pytest.fixture
def anyio_backend() -> str:
    """固定项目支持的异步测试后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_stream_emits_typed_delta_usage_and_single_finish() -> None:
    # Given: 服务返回内容、usage 和唯一终止帧。
    body = (
        'data: {"choices":[{"delta":{"content":"你"},"finish_reason":null}]}\n\n'
        'data: {"choices":[{"delta":{"content":"好"},"finish_reason":"stop"}],'
        '"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}\n\n'
        "data: [DONE]\n\n"
    )
    transport = httpx.MockTransport(lambda _request: httpx.Response(200, text=body))
    provider = _provider(transport)

    # When: 消费完整流。
    results = [result async for result in provider.stream(REQUEST)]
    await provider.aclose()

    # Then: 增量和终态均为严格类型且终态只出现一次。
    assert results == [
        ProviderDelta("你"),
        ProviderDelta("好"),
        CompletionFinished("stop", prompt_tokens=3, completion_tokens=2, total_tokens=5),
    ]


@pytest.mark.anyio
async def test_retry_is_allowed_only_before_first_delta() -> None:
    # Given: 第一次连接失败,第二次成功。
    calls = 0

    def handler(_request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if calls == 1:
            msg = "secret body"
            raise httpx.ConnectError(msg, request=_request)
        return httpx.Response(
            200,
            text=f"{SUCCESS_BODY}{DONE_BODY}",
        )

    provider = _provider(httpx.MockTransport(handler))

    # When: 消费流。
    results = [result async for result in provider.stream(REQUEST)]
    await provider.aclose()

    # Then: 提交前仅重试一次并成功。
    assert calls == 2
    assert results == [ProviderDelta("ok"), CompletionFinished("stop")]


@pytest.mark.anyio
async def test_failure_after_delta_is_not_retried() -> None:
    # Given: 流先提交可见增量,随后截断。
    calls = 0

    def handler(_request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        return httpx.Response(
            200,
            text='data: {"choices":[{"delta":{"content":"committed"},"finish_reason":null}]}\n\n',
        )

    provider = _provider(httpx.MockTransport(handler), retries=2)

    # When: 消费截断流。
    results = [result async for result in provider.stream(REQUEST)]
    await provider.aclose()

    # Then: 不重放已提交增量,返回协议失败。
    assert calls == 1
    assert results == [ProviderDelta("committed"), ProviderFailure(FailureKind.TRUNCATED)]


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("status", "expected"),
    [
        (400, FailureKind.HTTP_CLIENT),
        (429, FailureKind.HTTP_RETRY_EXHAUSTED),
        (503, FailureKind.HTTP_RETRY_EXHAUSTED),
    ],
)
async def test_http_failures_are_typed_and_secret_free(status: int, expected: FailureKind) -> None:
    # Given: 服务返回包含秘密的失败正文。
    transport = httpx.MockTransport(
        lambda _request: httpx.Response(status, text=f"Authorization: Bearer {API_KEY}"),
    )
    provider = _provider(transport)

    # When: 消费失败流。
    results = [result async for result in provider.stream(REQUEST)]
    await provider.aclose()

    # Then: 仅暴露稳定类别。
    assert results == [ProviderFailure(expected, status_code=status)]
    assert API_KEY not in repr(results)
    assert API_KEY not in str(results[0])


@pytest.mark.anyio
@pytest.mark.parametrize(
    "body",
    [
        "event: message\ndata: not-json\n\n",
        'data: {"choices":[]}\n\n',
        f"{DUPLICATE_BODY}{DUPLICATE_FINISH_BODY}",
        "data: [DONE]\n\n",
    ],
)
async def test_malformed_duplicate_or_empty_completion_is_typed(body: str) -> None:
    # Given: 服务返回无效 SSE 或完成序列。
    provider = _provider(httpx.MockTransport(lambda _request: httpx.Response(200, text=body)))

    # When: 消费流。
    results = [result async for result in provider.stream(REQUEST)]
    await provider.aclose()

    # Then: 产生稳定且无原始正文的协议失败。
    assert type(results[-1]) is ProviderFailure
    assert results[-1].kind in {
        FailureKind.MALFORMED,
        FailureKind.DUPLICATE_TERMINAL,
        FailureKind.EMPTY,
    }
    assert body not in str(results[-1])


@pytest.mark.anyio
async def test_cancellation_propagates_and_closes_response() -> None:
    # Given: 一个永不结束但会记录关闭的响应流。
    closed = anyio.Event()

    class BlockingStream(httpx.AsyncByteStream):
        @override
        async def __aiter__(self) -> AsyncIterator[bytes]:
            yield b'data: {"choices":[{"delta":{"content":"x"},"finish_reason":null}]}\n\n'
            await anyio.sleep_forever()

        @override
        async def aclose(self) -> None:
            closed.set()

    provider = _provider(
        httpx.MockTransport(lambda _request: httpx.Response(200, stream=BlockingStream())),
    )

    # When: 首个增量后取消消费任务。
    async with anyio.create_task_group() as group:

        async def consume() -> None:
            async for _result in provider.stream(REQUEST):
                group.cancel_scope.cancel()

        _ = group.start_soon(consume)

    # Then: 取消没有转换为普通失败,响应资源已关闭。
    assert closed.is_set()
    await provider.aclose()


@pytest.mark.anyio
async def test_timeout_becomes_typed_failure() -> None:
    # Given: HTTP 传输在读取前达到超时。
    def handler(request: httpx.Request) -> httpx.Response:
        msg = "包含秘密的超时"
        raise httpx.ReadTimeout(msg, request=request)

    provider = _provider(httpx.MockTransport(handler))

    # When: 消费流。
    results = [result async for result in provider.stream(REQUEST)]
    await provider.aclose()

    # Then: 超时转换为无秘密类型化失败。
    assert results == [ProviderFailure(FailureKind.TIMEOUT)]
    assert API_KEY not in repr(results)
