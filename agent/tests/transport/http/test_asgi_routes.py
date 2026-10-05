"""通过真实 ASGI 表面验证 HTTP 快照、SSE 与 mutation。"""

from collections.abc import AsyncGenerator
from datetime import UTC, datetime
from typing import cast

import pytest
from fastapi import FastAPI
from httpx import ASGITransport, AsyncClient

from scyg_agent.adapters.auth import (
    AuthenticationError,
    AuthenticationErrorCode,
    EncodedJwt,
    Principal,
    WebRunPrincipal,
)
from scyg_agent.adapters.redis import (
    RedisStreamId,
    RedisStreamKind,
    RedisStreamStore,
    StreamEnvelope,
    StreamExpiredError,
)
from scyg_agent.domain.runs import RunId, RunStatus
from scyg_agent.transport.http import HTTPDependencies, create_http_app
from tests.application.test_facade import NOW, RUN_ID, facade, make_run

AUTH = {"Authorization": "Bearer valid"}


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class FixedVerifier:
    """把测试 Bearer 值转换为精确 Web 主体。"""

    def verify(self, token: EncodedJwt) -> Principal:
        if token.value == "valid":
            return WebRunPrincipal.from_strings("user-t20", str(RUN_ID))
        if token.value == "other":
            return WebRunPrincipal.from_strings("other-t20", str(RUN_ID))
        raise AuthenticationError(AuthenticationErrorCode.REJECTED)

    def require_exact_principal(self, principal: Principal) -> None:
        if type(principal) is not WebRunPrincipal:
            raise AuthenticationError(AuthenticationErrorCode.INVALID_PRINCIPAL)


def make_app(*, status: RunStatus = RunStatus.PENDING, latest: int = 1) -> FastAPI:
    """构造只含 T20 路由的内存 ASGI 应用。"""
    application, _, _ = facade(make_run(status), latest)
    return create_http_app(HTTPDependencies(application, FixedVerifier(), lambda: NOW))


class FakeRedisStore:
    """Provide one finite transient stream for the ASGI main path."""

    def read(
        self, run_id: RunId, cursor: RedisStreamId | None = None
    ) -> AsyncGenerator[tuple[RedisStreamId, StreamEnvelope], None]:
        _ = cursor

        async def events() -> AsyncGenerator[tuple[RedisStreamId, StreamEnvelope], None]:
            yield (
                RedisStreamId("123-0"),
                StreamEnvelope(run_id, 1, RedisStreamKind.TEXT_DELTA, 0, NOW, {"text": "hello"}),
            )

        return events()


class ExpiredRedisStore:
    """Raise the typed expiry outcome when the transient stream is gone."""

    def read(
        self, run_id: RunId, cursor: RedisStreamId | None = None
    ) -> AsyncGenerator[tuple[RedisStreamId, StreamEnvelope], None]:
        _ = run_id, cursor

        async def events() -> AsyncGenerator[tuple[RedisStreamId, StreamEnvelope], None]:
            if cursor is None:
                yield (
                    RedisStreamId("0-0"),
                    StreamEnvelope(
                        run_id,
                        1,
                        RedisStreamKind.PROGRESS,
                        0,
                        NOW,
                        {"step": "idle"},
                    ),
                )
            else:
                raise StreamExpiredError

        return events()


def make_redis_app() -> FastAPI:
    """Construct the HTTP surface with transient Redis stream injection."""
    application, _, _ = facade(make_run(RunStatus.PENDING), 1)
    return create_http_app(
        HTTPDependencies(
            application,
            FixedVerifier(),
            lambda: NOW,
            stream_store=cast("RedisStreamStore", cast("object", FakeRedisStore())),
        )
    )


def make_expired_redis_app() -> FastAPI:
    """Construct the HTTP surface with an expired transient stream."""
    application, _, _ = facade(make_run(RunStatus.PENDING), 1)
    return create_http_app(
        HTTPDependencies(
            application,
            FixedVerifier(),
            lambda: NOW,
            stream_store=cast("RedisStreamStore", cast("object", ExpiredRedisStore())),
        )
    )


@pytest.mark.anyio
async def test_redis_sse_uses_native_cursor_and_transient_payload() -> None:
    transport = ASGITransport(app=make_redis_app())
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get(f"/api/runs/{RUN_ID}/events", headers=AUTH)

    assert response.status_code == 200
    assert "id: 123-0\nevent: text_delta\ndata: {" in response.text
    assert '"text":"hello"' in response.text


@pytest.mark.anyio
async def test_redis_sse_emits_stream_expired_recovery_hint() -> None:
    """Expired Redis state yields a safe durable-snapshot recovery frame."""
    transport = ASGITransport(app=make_expired_redis_app())
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get(
            f"/api/runs/{RUN_ID}/events?cursor=122-0",
            headers=AUTH,
        )

    assert response.status_code == 200
    assert "event: stream_expired" in response.text
    assert '"snapshot_url":"/api/runs/run_t20facade0"' in response.text


@pytest.mark.anyio
async def test_snapshot_auth_binding_and_owner_hiding_through_asgi() -> None:
    # Given: 一个存在且属于令牌用户的 Run。
    transport = ASGITransport(app=make_app())
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        # When: 合法、缺失、无效、错误 Run 和错误所有者依次读取。
        valid = await client.get(f"/api/runs/{RUN_ID}", headers=AUTH)
        missing = await client.get(f"/api/runs/{RUN_ID}")
        invalid = await client.get(f"/api/runs/{RUN_ID}", headers={"Authorization": "Bearer bad"})
        wrong_run = await client.get("/api/runs/run_t20other00", headers=AUTH)
        wrong_owner = await client.get(
            f"/api/runs/{RUN_ID}", headers={"Authorization": "Bearer other"}
        )
    # Then: 仅精确绑定返回快照,所有者与 Run 绑定均不泄漏存在性。
    assert valid.status_code == 200
    assert valid.json()["cursor"] == 1
    assert missing.status_code == 401
    assert invalid.status_code == 401
    assert wrong_run.status_code == 404
    assert wrong_owner.status_code == 404


@pytest.mark.anyio
async def test_sse_cursor_replay_and_strict_frame_through_asgi() -> None:
    # Given: 一个从游标 2 可结束的确定性 T11 跟随流。
    transport = ASGITransport(app=make_app(latest=2))
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        # When: 使用相同查询与重连头读取事件。
        response = await client.get(
            f"/api/runs/{RUN_ID}/events?cursor=2",
            headers={**AUTH, "Last-Event-ID": "2"},
        )
        conflict = await client.get(
            f"/api/runs/{RUN_ID}/events?cursor=1",
            headers={**AUTH, "Last-Event-ID": "2"},
        )
        malformed = await client.get(f"/api/runs/{RUN_ID}/events?cursor=-1", headers=AUTH)
    # Then: SSE 使用持久化序列、事件名和紧凑 JSON,冲突/非法游标被拒绝。
    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/event-stream")
    assert "id: 3\nevent: run_cancelled\ndata: {" in response.text
    assert conflict.status_code == 400
    assert malformed.status_code == 400


@pytest.mark.anyio
async def test_command_cancel_and_input_are_independent_from_sse() -> None:
    # Given: 等待输入 Run 与完整幂等请求字段。
    transport = ASGITransport(app=make_app(status=RunStatus.WAITING_INPUT))
    timestamp = datetime(2026, 7, 12, 13, tzinfo=UTC).isoformat()
    input_body = {
        "interaction_id": "int_t20facade0",
        "command_id": "cmd_t20input00",
        "event_id": "evt_t20input00",
        "expected_revision": 1,
        "expected_sequence": 0,
        "response_digest": "b" * 64,
        "result_reference": "input:t20",
        "occurred_at": timestamp,
    }
    command_body = {
        "kind": "cancel",
        "command_id": "cmd_t20cancel0",
        "event_id": "evt_t20cancel0",
        "expected_revision": 1,
        "expected_sequence": 0,
        "request_digest": "a" * 64,
        "occurred_at": timestamp,
    }
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        # When: 不建立 SSE 即提交输入、命令与独立取消。
        submitted = await client.post(f"/api/runs/{RUN_ID}/input", headers=AUTH, json=input_body)
        commanded = await client.post(
            f"/api/runs/{RUN_ID}/commands", headers=AUTH, json=command_body
        )
        cancelled = await client.post(f"/api/runs/{RUN_ID}/cancel", headers=AUTH)
    # Then: 三个 mutation 都保留幂等重放响应头并成功返回。
    assert submitted.status_code == commanded.status_code == cancelled.status_code == 200
    assert submitted.headers["Idempotency-Replayed"] == "false"
    assert commanded.headers["Idempotency-Replayed"] == "false"
    assert cancelled.headers["Idempotency-Replayed"] == "false"
