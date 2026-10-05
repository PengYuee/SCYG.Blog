"""Redis stream adapter tests using an in-memory scripted client."""

from collections.abc import Mapping
from datetime import UTC, datetime
from typing import TYPE_CHECKING, cast, final

import pytest
from pydantic import SecretStr

from scyg_agent.adapters.redis import (
    InvalidRedisStreamIdError,
    RedisKeyspace,
    RedisSettings,
    RedisStreamClient,
    RedisStreamId,
    RedisStreamKind,
    StaleAttemptError,
    StreamEnvelope,
    StreamExpiredError,
)
from scyg_agent.domain.runs import RunId

if TYPE_CHECKING:
    from scyg_agent.adapters.redis.streams import RedisClientProtocol

RUN_ID = RunId("run_redis001")
NOW = datetime(2026, 9, 21, 12, tzinfo=UTC)


@final
class Script:
    """Return scripted Lua results and record invocation arguments."""

    def __init__(self, result: object) -> None:
        self.result = result
        self.calls: list[dict[str, object]] = []

    async def __call__(self, **kwargs: object) -> object:
        self.calls.append(kwargs)
        return self.result


@final
class RedisCommands:
    """Provide the minimum async Redis command surface for adapter tests."""

    def __init__(self) -> None:
        self.pings = 0
        self.closed = False

    async def ping(self) -> bool:
        self.pings += 1
        return True

    async def aclose(self) -> None:
        self.closed = True


@final
class ReadRedisCommands:
    """Script stream existence and XREAD results for replay tests."""

    def __init__(
        self,
        exists_results: list[int],
        read_results: list[list[tuple[str, list[tuple[str, Mapping[str, str]]]]]],
    ) -> None:
        self.exists_results = exists_results
        self.read_results = read_results
        self.cursors: list[dict[str, str]] = []

    async def exists(self, name: str) -> int:
        _ = name
        return self.exists_results.pop(0)

    async def xread(
        self,
        streams: Mapping[str, str],
        *,
        count: int,
        block: int,
    ) -> list[tuple[str, list[tuple[str, Mapping[str, str]]]]]:
        _ = count, block
        self.cursors.append(dict(streams))
        return self.read_results.pop(0)


@pytest.mark.anyio
async def test_attempt_activation_and_append_pass_stable_fencing_keys() -> None:
    """Given the active attempt, When appending, Then Lua receives both stable keys."""
    commands = RedisCommands()
    activate = Script(1)
    append = Script([1, "123-0"])
    client = RedisStreamClient(
        cast("RedisClientProtocol", cast("object", commands)),
        RedisSettings(SecretStr("redis://localhost/0")),
        activate,
        append,
    )
    envelope = StreamEnvelope(
        RUN_ID,
        3,
        RedisStreamKind.TEXT_DELTA,
        7,
        NOW,
        {"text": "hello"},
    )

    await client.activate_attempt(RUN_ID, 3)
    assert await client.append(envelope) == RedisStreamId("123-0")

    activation_call = activate.calls[0]
    append_call = append.calls[0]
    assert cast("list[str]", activation_call["keys"]) == [RedisKeyspace.attempt(RUN_ID)]
    assert cast("list[str]", append_call["keys"]) == [
        RedisKeyspace.stream(RUN_ID),
        RedisKeyspace.attempt(RUN_ID),
    ]
    append_args = cast("list[str]", append_call["args"])
    assert append_args[0] == "3"
    assert append_args[3] == "text_delta"


@pytest.mark.anyio
async def test_stale_attempt_result_is_rejected_without_stream_fallback() -> None:
    """Given a fenced old attempt, When Lua rejects it, Then the typed stale error escapes."""
    commands = RedisCommands()
    client = RedisStreamClient(
        cast("RedisClientProtocol", cast("object", commands)),
        RedisSettings(SecretStr("redis://localhost/0")),
        Script(0),
        Script([0, ""]),
    )

    with pytest.raises(StaleAttemptError):
        _ = await client.append(
            StreamEnvelope(RUN_ID, 2, RedisStreamKind.PROGRESS, 1, NOW, {"stage": "work"})
        )


def test_redis_keyspace_and_cursor_are_stable_and_bounded() -> None:
    """Stable Run keys and Redis cursors remain deterministic and value-bounded."""
    assert RedisKeyspace.stream(RUN_ID) == "agent:run:run_redis001:events"
    assert RedisKeyspace.attempt(RUN_ID) == "agent:run:run_redis001:current-attempt"
    assert str(RedisStreamId("0-0")) == "0-0"
    with pytest.raises(InvalidRedisStreamIdError):
        _ = RedisStreamId("not-a-stream-id")


@pytest.mark.anyio
async def test_read_replays_after_last_event_id_and_decodes_envelope() -> None:
    """XREAD starts after Last-Event-ID and returns the typed transient envelope."""
    fields = {
        "run_id": str(RUN_ID),
        "attempt": "3",
        "kind": "text_delta",
        "sequence": "7",
        "occurred_at": NOW.isoformat(),
        "payload": '{"text":"hello"}',
    }
    commands = ReadRedisCommands([1], [[(RedisKeyspace.stream(RUN_ID), [("123-0", fields)])]])
    client = RedisStreamClient(
        cast("RedisClientProtocol", cast("object", commands)),
        RedisSettings(SecretStr("redis://localhost/0")),
        Script(1),
        Script([1, "123-0"]),
    )

    events = client.read(RUN_ID, RedisStreamId("122-0"))
    cursor, envelope = await anext(events)
    await events.aclose()

    assert cursor == RedisStreamId("123-0")
    assert envelope.kind is RedisStreamKind.TEXT_DELTA
    assert envelope.sequence == 7
    assert envelope.payload == {"text": "hello"}
    assert commands.cursors == [{RedisKeyspace.stream(RUN_ID): "122-0"}]


@pytest.mark.anyio
async def test_read_reports_expiry_after_stream_disappears() -> None:
    """A cursor-backed stream that vanishes yields the explicit expiry outcome."""
    commands = ReadRedisCommands([1, 0], [[]])
    client = RedisStreamClient(
        cast("RedisClientProtocol", cast("object", commands)),
        RedisSettings(SecretStr("redis://localhost/0")),
        Script(1),
        Script([1, "123-0"]),
    )

    events = client.read(RUN_ID, RedisStreamId("122-0"))
    with pytest.raises(StreamExpiredError):
        _ = await anext(events)
