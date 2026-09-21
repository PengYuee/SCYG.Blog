"""覆盖 HTTP 错误映射与 SSE 生命周期边界。"""

from collections.abc import AsyncGenerator
from typing import override

import anyio
import pytest
from fastapi import HTTPException, Response
from httpx import ASGITransport, AsyncClient

from scyg_agent.adapters.auth import BlogServicePrincipal, EncodedJwt, Principal, WebRunPrincipal
from scyg_agent.application import (
    FacadeConflict,
    FacadeInternal,
    FacadeNotFound,
    FacadePrecondition,
    FacadeSuccess,
)
from scyg_agent.domain.ports.event_store import EventCursor, StoredEvent
from scyg_agent.domain.runs import CommandId, EventId, RunCancelled
from scyg_agent.domain.runs.cancellation import CancellationQueued
from scyg_agent.transport.http import HTTPDependencies, StreamPolicy, create_http_app
from scyg_agent.transport.http.router import map_mutation_outcome
from scyg_agent.transport.http.sse import HEARTBEAT_FRAME, buffered_sse
from tests.application.test_facade import NOW, RUN_ID, FakeRuns, facade, make_run
from tests.transport.http.test_asgi_routes import AUTH, FixedVerifier


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class ScopeVerifier(FixedVerifier):
    """补充一个没有 Web Run scope 的服务主体。"""

    @override
    def verify(self, token: EncodedJwt) -> Principal:
        if token.value == "service":
            return BlogServicePrincipal("blog-api")
        return super().verify(token)

    @override
    def require_exact_principal(self, principal: Principal) -> None:
        if type(principal) not in {BlogServicePrincipal, WebRunPrincipal}:
            raise AssertionError(principal)


def application() -> tuple[ASGITransport, FakeRuns]:
    """构造可观察取消状态的真实 HTTP 应用。"""
    app_facade, runs, _ = facade(make_run(), 2)
    app = create_http_app(HTTPDependencies(app_facade, ScopeVerifier(), lambda: NOW))
    return ASGITransport(app=app), runs


@pytest.mark.anyio
async def test_auth_rejects_duplicate_malformed_scope_and_invalid_run() -> None:
    # Given: 一个使用精确 Web 主体验证器的 ASGI 应用。
    transport, _ = application()
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        # When: 请求携带重复头、非规范 Bearer、服务 scope 或非法 Run 标识。
        duplicate = await client.get(
            f"/api/runs/{RUN_ID}",
            headers=[("Authorization", "Bearer valid"), ("Authorization", "Bearer valid")],
        )
        malformed = await client.get(
            f"/api/runs/{RUN_ID}", headers={"Authorization": "bearer valid"}
        )
        denied = await client.get(
            f"/api/runs/{RUN_ID}", headers={"Authorization": "Bearer service"}
        )
        invalid_run = await client.get("/api/runs/not-a-run", headers=AUTH)
    # Then: 鉴权失败不进入门面,非法路径只返回安全中文错误。
    assert duplicate.status_code == 401
    assert malformed.status_code == 401
    assert denied.status_code == 403
    assert invalid_run.status_code == 422
    assert invalid_run.json() == {"detail": "Run 标识无效"}


@pytest.mark.anyio
async def test_cursor_header_duplicates_future_and_validation_are_rejected() -> None:
    # Given: journal head 位于序列 2。
    transport, runs = application()
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        # When: 分别提交重复头、未来游标及额外请求字段。
        duplicate = await client.get(
            f"/api/runs/{RUN_ID}/events",
            headers=[
                ("Authorization", "Bearer valid"),
                ("Last-Event-ID", "1"),
                ("Last-Event-ID", "1"),
            ],
        )
        streamed = await client.get(f"/api/runs/{RUN_ID}/events?cursor=2", headers=AUTH)
        future = await client.get(f"/api/runs/{RUN_ID}/events?cursor=3", headers=AUTH)
        invalid_body = await client.post(
            f"/api/runs/{RUN_ID}/commands", headers=AUTH, json={"unexpected": True}
        )
    # Then: 三种边界在建立订阅或调用命令前失败。
    assert duplicate.status_code == 400
    assert streamed.status_code == 200
    assert not runs.cancelled
    assert future.status_code == 409
    assert invalid_body.status_code == 422
    assert invalid_body.json() == {"detail": "请求参数无效"}


def test_mutation_outcomes_map_to_stable_http_contract() -> None:
    # Given: mutation 的每个闭合门面结果。
    queued = CancellationQueued(RUN_ID, NOW, replayed=True)
    response = Response()
    # When/Then: 重放成功保留响应头。
    success = map_mutation_outcome(FacadeSuccess(queued, replayed=True), response)
    assert success.replayed
    assert response.headers["Idempotency-Replayed"] == "true"
    # When/Then: 每种失败映射为稳定状态与中文消息。
    cases = (
        (FacadeNotFound(RUN_ID), 404, "未找到 Run"),
        (FacadeConflict(), 409, "请求与已持久化事实冲突"),
        (FacadePrecondition(), 409, "Run 当前状态不允许此操作"),
        (FacadeInternal(), 500, "应用内部状态异常"),
    )
    for outcome, code, message in cases:
        with pytest.raises(HTTPException) as captured:
            _ = map_mutation_outcome(outcome, Response())
        error = captured.value
        assert error.status_code == code
        assert error.detail == message


def stored_event(sequence: int = 3) -> StoredEvent:
    """创建一个可严格编码的持久化终态事件。"""
    return StoredEvent(
        EventCursor(sequence),
        RunCancelled(EventId("evt_t20stream0"), CommandId("cmd_t20stream0"), NOW, RUN_ID, 2),
    )


@pytest.mark.anyio
async def test_heartbeat_and_disconnect_close_only_subscription() -> None:
    # Given: 一个等待唤醒且记录关闭的订阅。
    closed = anyio.Event()

    async def waiting() -> AsyncGenerator[StoredEvent, None]:
        try:
            await anyio.sleep_forever()
            yield stored_event()
        finally:
            closed.set()

    stream = buffered_sse(
        waiting(), StreamPolicy(buffer_size=1, heartbeat_seconds=0.001, slow_client_seconds=1)
    )
    # When: 客户端收到心跳后断开。
    frame = await anext(stream)
    await stream.aclose()
    # Then: 心跳不携带序列,且只关闭订阅生成器。
    assert frame == HEARTBEAT_FRAME
    assert closed.is_set()


@pytest.mark.anyio
async def test_slow_client_policy_closes_bounded_producer() -> None:
    # Given: 一个持续提供日志事件并记录关闭的订阅。
    closed = anyio.Event()

    async def rapid() -> AsyncGenerator[StoredEvent, None]:
        try:
            sequence = 1
            while True:
                yield stored_event(sequence)
                sequence += 1
        finally:
            closed.set()

    stream = buffered_sse(
        rapid(), StreamPolicy(buffer_size=1, heartbeat_seconds=1, slow_client_seconds=0.001)
    )
    # When: 客户端只读取首帧后停止消费至生产者背压超时。
    first = await anext(stream)
    await anyio.sleep(0.01)
    await stream.aclose()
    # Then: 首帧来自日志,慢客户端策略关闭订阅且不触碰 Run。
    assert first.startswith("id: 1\n")
    assert closed.is_set()


def test_stream_policy_rejects_nonpositive_boundaries() -> None:
    # Given/When/Then: 任一非正边界都被中文类型错误拒绝。
    for policy in ((0, 1.0, 1.0), (1, 0.0, 1.0), (1, 1.0, 0.0)):
        with pytest.raises(ValueError, match="^SSE 策略必须使用正数边界$"):
            _ = StreamPolicy(*policy)
