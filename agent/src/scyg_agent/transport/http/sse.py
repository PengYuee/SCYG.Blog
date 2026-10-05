"""把持久事件或 Redis 瞬时事件编码为稳定 SSE 帧并管理订阅。"""  # noqa: D415

import json
from collections.abc import AsyncGenerator, Awaitable, Callable
from dataclasses import dataclass
from typing import Final

import anyio
from anyio import EndOfStream

from scyg_agent.adapters.database.event_codec import serialize_event
from scyg_agent.adapters.redis import (
    RedisStreamError,
    RedisStreamId,
    StreamEnvelope,
    StreamExpiredError,
)
from scyg_agent.domain.ports.event_store import StoredEvent

HEARTBEAT_FRAME: Final = ": heartbeat\n\n"


@dataclass(frozen=True, slots=True)
class StreamPolicy:
    """限制单个客户端的内存、心跳和阻塞时间。."""

    buffer_size: int = 16
    heartbeat_seconds: float = 15.0
    slow_client_seconds: float = 5.0

    def __post_init__(self) -> None:
        """拒绝会关闭背压保护的策略。."""
        if self.buffer_size < 1 or self.heartbeat_seconds <= 0 or self.slow_client_seconds <= 0:
            message = "SSE 策略必须使用正数边界"
            raise ValueError(message)


def encode_sse(stored: StoredEvent) -> str:
    """使用持久化序列与严格 JSON 编码一个事件。."""
    kind, payload = serialize_event(stored.event)
    data = {
        "event_id": str(stored.event.event_id),
        "run_id": str(stored.event.run_id),
        "revision": stored.event.revision,
        "occurred_at": stored.event.occurred_at.isoformat(),
        "kind": kind.value,
        "payload": payload,
    }
    encoded = json.dumps(data, ensure_ascii=False, allow_nan=False, separators=(",", ":"))
    return f"id: {stored.cursor.sequence}\nevent: {kind.value}\ndata: {encoded}\n\n"


def encode_redis_sse(cursor: RedisStreamId, envelope: StreamEnvelope) -> str:
    """Encode one transient Redis envelope while preserving its native cursor."""
    data = {
        "run_id": str(envelope.run_id),
        "attempt": envelope.attempt,
        "sequence": envelope.sequence,
        "occurred_at": envelope.occurred_at.isoformat(),
        "kind": envelope.kind.value,
        "payload": dict(envelope.payload),
    }
    encoded = json.dumps(data, ensure_ascii=False, allow_nan=False, separators=(",", ":"))
    return f"id: {cursor}\nevent: {envelope.kind.value}\ndata: {encoded}\n\n"


def encode_stream_expired(run_id: str, status: str, revision: int) -> str:
    """Encode a safe recovery hint when the transient stream no longer exists."""
    data = {
        "run_id": run_id,
        "status": status,
        "revision": revision,
        "snapshot_url": f"/api/runs/{run_id}",
    }
    encoded = json.dumps(data, ensure_ascii=False, allow_nan=False, separators=(",", ":"))
    return f"event: stream_expired\ndata: {encoded}\n\n"


def encode_stream_error(code: str) -> str:
    """Encode a stable transient-stream failure without infrastructure details."""
    encoded = json.dumps({"code": code}, ensure_ascii=False, separators=(",", ":"))
    return f"event: error\ndata: {encoded}\n\n"


async def buffered_sse(
    events: AsyncGenerator[StoredEvent, None], policy: StreamPolicy
) -> AsyncGenerator[str, None]:
    """以有界通道隔离慢客户端,并在退出时只关闭当前订阅。."""
    send, receive = anyio.create_memory_object_stream[str](policy.buffer_size)

    async def produce() -> None:
        """消费 T11 真值流;超时表示客户端过慢并终止本订阅。."""
        try:
            async with send:
                async for stored in events:
                    with anyio.fail_after(policy.slow_client_seconds):
                        await send.send(encode_sse(stored))
        except TimeoutError:
            return
        finally:
            await events.aclose()

    async with anyio.create_task_group() as task_group, receive:
        _ = task_group.start_soon(produce)
        try:
            while True:
                frame: str | None = None
                with anyio.move_on_after(policy.heartbeat_seconds):
                    frame = await receive.receive()
                if frame is None:
                    yield HEARTBEAT_FRAME
                    continue
                yield frame
        except (EndOfStream, GeneratorExit):
            return
        finally:
            task_group.cancel_scope.cancel()


async def buffered_redis_sse(
    events: AsyncGenerator[tuple[RedisStreamId, StreamEnvelope], None],
    policy: StreamPolicy,
    on_expired: Callable[[], Awaitable[str]],
) -> AsyncGenerator[str, None]:
    """Bound one Redis subscription and turn expiry/failure into one final frame."""
    send, receive = anyio.create_memory_object_stream[str](policy.buffer_size)

    async def produce() -> None:
        """Consume Redis until disconnect, expiry, or a sanitized infrastructure error."""
        try:
            async for cursor, envelope in events:
                with anyio.fail_after(policy.slow_client_seconds):
                    await send.send(encode_redis_sse(cursor, envelope))
        except TimeoutError:
            return
        except StreamExpiredError:
            with anyio.move_on_after(policy.slow_client_seconds):
                await send.send(await on_expired())
        except RedisStreamError:
            with anyio.move_on_after(policy.slow_client_seconds):
                await send.send(encode_stream_error("redis_unavailable"))
        finally:
            await events.aclose()
            await send.aclose()

    async with anyio.create_task_group() as task_group, receive:
        _ = task_group.start_soon(produce)
        try:
            while True:
                frame: str | None = None
                with anyio.move_on_after(policy.heartbeat_seconds):
                    frame = await receive.receive()
                if frame is None:
                    yield HEARTBEAT_FRAME
                    continue
                yield frame
        except (EndOfStream, GeneratorExit):
            return
        finally:
            task_group.cancel_scope.cancel()
