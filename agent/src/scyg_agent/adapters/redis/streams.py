"""Typed Redis Streams adapter for transient Run output."""

from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from typing import TYPE_CHECKING, Final, Protocol, cast, override

from redis.asyncio import Redis
from redis.exceptions import RedisError

from scyg_agent.domain.runs import RunId

if TYPE_CHECKING:
    from collections.abc import AsyncGenerator, Awaitable, Mapping

    from pydantic import SecretStr

MIN_STREAM_MAXLEN: Final = 100
MAX_PAYLOAD_BYTES: Final = 256_000
STREAM_ID_PART_COUNT: Final = 2
STREAM_KEY_PREFIX: Final = "agent:run:"
STREAM_KEY_SUFFIX: Final = ":events"
ATTEMPT_KEY_SUFFIX: Final = ":current-attempt"
DEFAULT_CONNECT_TIMEOUT_SECONDS: Final = 5.0
DEFAULT_READ_TIMEOUT_SECONDS: Final = 30.0
DEFAULT_BLOCK_MILLISECONDS: Final = 1_000
DEFAULT_READ_COUNT: Final = 100


class InvalidRedisSettingsError(ValueError):
    """Reject invalid Redis endpoint and stream bounds."""

    @override
    def __str__(self) -> str:
        """Return a stable value-free configuration diagnostic."""
        return "Redis 配置边界无效"


class InvalidRedisStreamIdError(ValueError):
    """Reject a cursor that is not a Redis stream ID."""

    @override
    def __str__(self) -> str:
        """Return a stable value-free cursor diagnostic."""
        return "Redis 流游标无效"


class InvalidStreamEnvelopeError(ValueError):
    """Reject an empty or non-progressing transient envelope."""

    @override
    def __str__(self) -> str:
        """Return a stable value-free envelope diagnostic."""
        return "Redis 流事件边界无效"


class RedisStreamKind(StrEnum):
    """Closed set of transient stream envelope kinds."""

    TEXT_DELTA = "text_delta"
    PROGRESS = "progress"
    APPROVAL_REQUIRED = "approval_required"
    TERMINAL = "terminal"
    ERROR = "error"
    STREAM_EXPIRED = "stream_expired"


@dataclass(frozen=True, slots=True)
class RedisSettings:
    """Hold validated Redis endpoint and bounded transient-stream settings."""

    url: SecretStr
    stream_ttl_seconds: int = 86_400
    stream_maxlen: int = 10_000
    connect_timeout_seconds: float = DEFAULT_CONNECT_TIMEOUT_SECONDS
    read_timeout_seconds: float = DEFAULT_READ_TIMEOUT_SECONDS

    def __post_init__(self) -> None:
        """Reject unbounded or non-positive Redis stream settings."""
        if (
            not self.url.get_secret_value()
            or self.stream_ttl_seconds < 1
            or self.stream_maxlen < MIN_STREAM_MAXLEN
            or self.connect_timeout_seconds <= 0
            or self.read_timeout_seconds <= 0
        ):
            raise InvalidRedisSettingsError


@dataclass(frozen=True, slots=True)
class RedisStreamId:
    """Carry Redis's opaque stream cursor without accepting arbitrary values."""

    value: str

    def __post_init__(self) -> None:
        """Require a Redis stream ID shape suitable for XREAD."""
        parts = self.value.split("-", 1) if type(self.value) is str else ()
        if len(parts) != STREAM_ID_PART_COUNT or not all(part.isdigit() for part in parts):
            raise InvalidRedisStreamIdError

    @override
    def __str__(self) -> str:
        """Return the validated Redis stream ID."""
        return self.value


@dataclass(frozen=True, slots=True)
class StreamEnvelope:
    """Represent one bounded transient Run stream event."""

    run_id: RunId
    attempt: int
    kind: RedisStreamKind
    sequence: int
    occurred_at: datetime
    payload: Mapping[str, str]

    def __post_init__(self) -> None:
        """Enforce the stable envelope's scalar and payload boundaries."""
        if (
            type(self.run_id) is not RunId
            or type(self.attempt) is not int
            or self.attempt < 1
            or type(self.kind) is not RedisStreamKind
            or type(self.sequence) is not int
            or self.sequence < 0
            or type(self.occurred_at) is not datetime
            or not self.payload
            or any(
                type(key) is not str or type(value) is not str
                for key, value in self.payload.items()
            )
        ):
            raise InvalidStreamEnvelopeError
        encoded = json.dumps(
            dict(self.payload), ensure_ascii=False, allow_nan=False, separators=(",", ":")
        )
        if len(encoded.encode("utf-8")) > MAX_PAYLOAD_BYTES:
            raise InvalidStreamEnvelopeError


class RedisStreamError(RuntimeError):
    """Base class for sanitized Redis stream failures."""

    @override
    def __str__(self) -> str:
        """Return a stable failure without endpoint details."""
        return "Redis 流操作失败"


class RedisUnavailableError(RedisStreamError):
    """Report Redis connectivity or command failure."""

    @override
    def __str__(self) -> str:
        """Return a stable unavailable diagnostic."""
        return "Redis 流存储不可用"


class StaleAttemptError(RedisStreamError):
    """Report a Worker attempt fenced by a newer claim."""

    @override
    def __str__(self) -> str:
        """Return a stable fencing diagnostic."""
        return "Redis 流写入尝试已过期"


class StreamExpiredError(RedisStreamError):
    """Report a cursor whose transient stream has expired."""

    @override
    def __str__(self) -> str:
        """Return a stable cursor-expired diagnostic."""
        return "Redis 流已过期"


class RedisScriptProtocol(Protocol):
    """Type the small callable surface returned by Redis register_script."""

    def __call__(
        self,
        *,
        keys: list[str],
        args: list[str],
        client: RedisClientProtocol,
    ) -> Awaitable[object]:
        """Execute a registered Lua script against one client."""
        ...


class RedisClientProtocol(Protocol):
    """Type the decoded async Redis commands used by the adapter."""

    async def ping(self) -> bool:
        """Check server reachability."""
        ...

    async def aclose(self) -> None:
        """Close the connection pool."""
        ...

    async def exists(self, name: str) -> int:
        """Return whether a stream key exists."""
        ...

    async def xread(
        self,
        streams: Mapping[str, str],
        *,
        count: int,
        block: int,
    ) -> list[tuple[str, list[tuple[str, Mapping[str, str]]]]]:
        """Read entries after one or more stream cursors."""
        ...

    def register_script(self, script: str) -> RedisScriptProtocol:
        """Register one Lua script."""
        ...


class RedisStreamStore(Protocol):
    """Expose transient stream activation, writing, and replay operations."""

    async def ping(self) -> None:
        """Verify Redis availability."""
        ...

    async def activate_attempt(self, run_id: RunId, attempt: int) -> None:
        """Monotonically activate a PostgreSQL-owned Worker attempt."""
        ...

    async def append(self, envelope: StreamEnvelope) -> RedisStreamId:
        """Fence and append one transient event."""
        ...

    def read(
        self, run_id: RunId, cursor: RedisStreamId | None = None
    ) -> AsyncGenerator[tuple[RedisStreamId, StreamEnvelope], None]:
        """Replay and follow one stable Run stream."""
        ...

    async def close(self) -> None:
        """Close the owned Redis client."""
        ...


ACTIVATE_ATTEMPT_SCRIPT: Final = """
local current = redis.call('GET', KEYS[1])
local incoming = tonumber(ARGV[1])
if current ~= false and incoming < tonumber(current) then
  return 0
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return 1
"""

APPEND_ATTEMPT_SCRIPT: Final = """
local current = redis.call('GET', KEYS[2])
if current == false or tonumber(current) ~= tonumber(ARGV[1]) then
  return {0, ''}
end
local stream_id = redis.call(
  'XADD', KEYS[1], 'MAXLEN', '~', ARGV[2], '*',
  'run_id', ARGV[3],
  'attempt', ARGV[1],
  'kind', ARGV[4],
  'sequence', ARGV[5],
  'occurred_at', ARGV[6],
  'payload', ARGV[7]
)
redis.call('EXPIRE', KEYS[1], ARGV[8])
redis.call('EXPIRE', KEYS[2], ARGV[8])
return {1, stream_id}
"""


@dataclass(frozen=True, slots=True)
class RedisKeyspace:
    """Generate stable keys without exposing Redis endpoint configuration."""

    @staticmethod
    def stream(run_id: RunId) -> str:
        """Return the stable transient stream key for a Run."""
        return f"{STREAM_KEY_PREFIX}{run_id}{STREAM_KEY_SUFFIX}"

    @staticmethod
    def attempt(run_id: RunId) -> str:
        """Return the monotonic current-attempt key for a Run."""
        return f"{STREAM_KEY_PREFIX}{run_id}{ATTEMPT_KEY_SUFFIX}"


@dataclass(frozen=True, slots=True)
class RedisStreamClient:
    """Own one async Redis client and enforce Lua attempt fencing."""

    client: RedisClientProtocol
    settings: RedisSettings
    _activate_script: RedisScriptProtocol
    _append_script: RedisScriptProtocol

    @classmethod
    def create(cls, settings: RedisSettings) -> RedisStreamClient:
        """Create a decoded async client without opening a connection."""
        client = cast(
            "RedisClientProtocol",
            cast(
                "object",
                Redis.from_url(  # pyright: ignore[reportUnknownMemberType]  # TYPE_IGNORE_OK - redis-py 动态工厂返回值由本地协议收窄。
                    settings.url.get_secret_value(),
                    decode_responses=True,
                    socket_connect_timeout=settings.connect_timeout_seconds,
                    socket_timeout=settings.read_timeout_seconds,
                    health_check_interval=30,
                ),
            ),
        )
        return cls(
            client,
            settings,
            client.register_script(ACTIVATE_ATTEMPT_SCRIPT),
            client.register_script(APPEND_ATTEMPT_SCRIPT),
        )

    async def ping(self) -> None:
        """Check Redis and translate all client failures."""
        try:
            _ = await self.client.ping()
        except RedisError as error:
            raise RedisUnavailableError from error

    async def activate_attempt(self, run_id: RunId, attempt: int) -> None:
        """Advance current attempt only when it is not older than Redis state."""
        if type(attempt) is not int or attempt < 1:
            raise StaleAttemptError
        try:
            result = await self._activate_script(
                keys=[RedisKeyspace.attempt(run_id)],
                args=[str(attempt), str(self.settings.stream_ttl_seconds)],
                client=self.client,
            )
            valid = int(str(result)) == 1
        except (RedisError, TypeError, ValueError) as error:
            raise RedisUnavailableError from error
        if not valid:
            raise StaleAttemptError

    async def append(self, envelope: StreamEnvelope) -> RedisStreamId:
        """Atomically fence, append, and refresh both transient key TTLs."""
        payload = json.dumps(
            dict(envelope.payload),
            ensure_ascii=False,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        try:
            result = await self._append_script(
                keys=[
                    RedisKeyspace.stream(envelope.run_id),
                    RedisKeyspace.attempt(envelope.run_id),
                ],
                args=[
                    str(envelope.attempt),
                    str(self.settings.stream_maxlen),
                    str(envelope.run_id),
                    envelope.kind.value,
                    str(envelope.sequence),
                    envelope.occurred_at.isoformat(),
                    payload,
                    str(self.settings.stream_ttl_seconds),
                ],
                client=self.client,
            )
            values = cast("list[object]", result)
            cursor_value = str(values[1]) if values and int(str(values[0])) == 1 else ""
        except (RedisError, IndexError, TypeError, ValueError) as error:
            raise RedisUnavailableError from error
        if not cursor_value:
            raise StaleAttemptError
        return RedisStreamId(cursor_value)

    async def read(
        self, run_id: RunId, cursor: RedisStreamId | None = None
    ) -> AsyncGenerator[tuple[RedisStreamId, StreamEnvelope], None]:
        """Replay from the cursor and continue with blocking XREAD."""
        stream = RedisKeyspace.stream(run_id)
        current = str(cursor) if cursor is not None else "0-0"
        saw_entry = False
        try:
            exists = await self.client.exists(stream)
        except (RedisError, TypeError, ValueError) as error:
            raise RedisUnavailableError from error
        if not exists and cursor is not None:
            raise StreamExpiredError
        try:
            while True:
                rows = await self.client.xread(
                    {stream: current}, count=DEFAULT_READ_COUNT, block=DEFAULT_BLOCK_MILLISECONDS
                )
                if not rows:
                    exists = await self.client.exists(stream)
                    if not exists and (cursor is not None or saw_entry):
                        _raise_stream_expired()
                    continue
                for _, entries in rows:
                    for raw_id, fields in entries:
                        stream_id = RedisStreamId(str(raw_id))
                        current = str(stream_id)
                        saw_entry = True
                        yield stream_id, _decode_envelope(fields, run_id)
        except StreamExpiredError:
            raise
        except (RedisError, TypeError, ValueError) as error:
            raise RedisUnavailableError from error

    async def close(self) -> None:
        """Close the owned Redis connection pool."""
        try:
            await self.client.aclose()
        except RedisError as error:
            raise RedisUnavailableError from error


def _decode_envelope(fields: Mapping[str, str], expected_run_id: RunId) -> StreamEnvelope:
    """Decode Redis hash fields into a bounded transient envelope."""
    try:
        raw_payload = cast("object", json.loads(fields["payload"]))
        if type(raw_payload) is not dict:
            return _invalid_envelope()
        raw_mapping = cast("dict[object, object]", raw_payload)
        if any(
            type(key) is not str or type(value) is not str for key, value in raw_mapping.items()
        ):
            return _invalid_envelope()
        envelope = StreamEnvelope(
            RunId(fields["run_id"]),
            int(fields["attempt"]),
            RedisStreamKind(fields["kind"]),
            int(fields["sequence"]),
            datetime.fromisoformat(fields["occurred_at"]),
            cast("Mapping[str, str]", raw_payload),
        )
        if envelope.run_id != expected_run_id:
            return _invalid_envelope()
    except (KeyError, TypeError, ValueError) as error:
        raise RedisUnavailableError from error
    else:
        return envelope


def _invalid_envelope() -> StreamEnvelope:
    """Raise the typed envelope boundary without nesting a direct raise."""
    raise ValueError


def _raise_stream_expired() -> None:
    """Raise the typed stream expiry boundary from a nested read loop."""
    raise StreamExpiredError
