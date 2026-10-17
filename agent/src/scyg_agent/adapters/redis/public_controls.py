"""Redis-time public quota reservations and token-fenced execution leases."""

from dataclasses import dataclass
from hashlib import sha256
from typing import final
from uuid import uuid4

import anyio
from pydantic import TypeAdapter, ValidationError
from redis.exceptions import RedisError

from scyg_agent.adapters.redis.streams import RedisClientProtocol
from scyg_agent.config import PublicSearchSettings

_RESERVATION_RESULT: TypeAdapter[tuple[int, int]] = TypeAdapter(tuple[int, int])

# One hash tag keeps every script key in the same Redis Cluster slot. Counters
# are day fields in one hash; each field expires logically after 48 hours.
_RESERVE = """
local now = tonumber(redis.call('TIME')[1])
local day = math.floor(now / 86400)
local token = ARGV[1]
local oldday = redis.call('HGET', KEYS[2], 'day')
local state = redis.call('HGET', KEYS[2], 'state')
if state == 'reserved' or state == 'consumed' then
 redis.call('HSET', KEYS[2], 'token', token)
 redis.call('EXPIRE', KEYS[2], 86400)
 return {1, tonumber(oldday)}
end
local used = tonumber(redis.call('HGET', KEYS[1], tostring(day)) or '0')
if used >= tonumber(ARGV[2]) then return {0, (day + 1) * 86400 - now} end
redis.call('HINCRBY', KEYS[1], tostring(day), 1)
redis.call('HDEL', KEYS[1], tostring(day - 3))
redis.call('EXPIRE', KEYS[1], 259200)
redis.call('HSET', KEYS[2], 'day', day, 'token', token, 'state', 'reserved')
redis.call('EXPIRE', KEYS[2], 86400)
return {1, day}
"""
_TRANSITION = """
if redis.call('HGET', KEYS[2], 'token') ~= ARGV[1] or
 redis.call('HGET', KEYS[2], 'day') ~= ARGV[2] or
 redis.call('HGET', KEYS[2], 'state') ~= 'reserved' then return 0 end
redis.call('HSET', KEYS[2], 'state', ARGV[3])
if ARGV[3] == 'refunded' then
 local used = tonumber(redis.call('HGET', KEYS[1], ARGV[2]) or '0')
 if used > 0 then redis.call('HINCRBY', KEYS[1], ARGV[2], -1) end
end
return 1
"""
_ACQUIRE = """
local now = tonumber(redis.call('TIME')[1])
local expiry = tonumber(redis.call('HGET', KEYS[1], 'expiry') or '0')
if expiry > now then return 0 end
redis.call('HSET', KEYS[1], 'token', ARGV[1], 'expiry', now + tonumber(ARGV[2]))
redis.call('EXPIRE', KEYS[1], ARGV[2])
return 1
"""
_RENEW = """
local now = tonumber(redis.call('TIME')[1])
if redis.call('HGET', KEYS[1], 'token') ~= ARGV[1] or
 tonumber(redis.call('HGET', KEYS[1], 'expiry') or '0') <= now then return 0 end
redis.call('HSET', KEYS[1], 'expiry', now + tonumber(ARGV[2]))
redis.call('EXPIRE', KEYS[1], ARGV[2])
return 1
"""
_RELEASE = """
if redis.call('HGET', KEYS[1], 'token') ~= ARGV[1] then return 0 end
return redis.call('DEL', KEYS[1])
"""


class PublicControlsUnavailableError(Exception):
    """Redis outcome is unavailable or unknown; do not fail open."""

    def __init__(self) -> None:
        """Expose a stable credential-free failure."""
        super().__init__("Public controls unavailable")


@final
class PublicQuotaExceededError(Exception):
    """The UTC natural-day quota has no remaining reservations."""

    def __init__(self, retry_after_seconds: int) -> None:
        """Carry the Redis-clock delay without exposing internal keys."""
        self.retry_after_seconds = retry_after_seconds
        super().__init__("Public daily quota exhausted")


@dataclass(frozen=True, slots=True)
class PublicReservation:
    """Fence one owner-operation reservation on its original UTC day."""

    token: str
    day: int
    operation_key: str


@final
class RedisPublicControls:
    """Borrow the existing Redis client without owning its lifecycle."""

    def __init__(self, client: RedisClientProtocol, settings: PublicSearchSettings) -> None:
        """Borrow existing scripts and their shared connection pool."""
        self._client = client
        self.settings = settings
        self._root = f"{settings.key_prefix}:{{public-search}}"
        self._scripts = {
            script: client.register_script(script)
            for script in (_RESERVE, _TRANSITION, _ACQUIRE, _RENEW, _RELEASE)
        }

    async def _eval(self, script: str, keys: tuple[str, ...], *args: object) -> object:
        try:
            with anyio.fail_after(2):
                return await self._scripts[script](
                    keys=list(keys), args=[str(arg) for arg in args], client=self._client
                )
        except (RedisError, TimeoutError, OSError):
            raise PublicControlsUnavailableError from None

    async def reserve(self, owner: str, key: str) -> PublicReservation:
        """Atomically reserve or continue the same owner's operation."""
        operation = sha256(f"{owner}\0{key}".encode()).hexdigest()
        token = uuid4().hex
        result = await self._eval(
            _RESERVE,
            (f"{self._root}:days", f"{self._root}:op:{operation}"),
            token,
            self.settings.daily_creations,
        )
        try:
            accepted, day_or_retry = _RESERVATION_RESULT.validate_python(result)
        except ValidationError:
            raise PublicControlsUnavailableError from None
        if accepted == 0:
            raise PublicQuotaExceededError(day_or_retry)
        return PublicReservation(token, day_or_retry, operation)

    async def _transition(self, reservation: PublicReservation, state: str) -> None:
        _ = await self._eval(
            _TRANSITION,
            (f"{self._root}:days", f"{self._root}:op:{reservation.operation_key}"),
            reservation.token,
            str(reservation.day),
            state,
        )

    async def consume(self, reservation: PublicReservation) -> None:
        """Mark a committed operation consumed without refunding it."""
        await self._transition(reservation, "consumed")

    async def refund(self, reservation: PublicReservation) -> None:
        """Refund only this still-reserved token on its original day."""
        await self._transition(reservation, "refunded")

    async def acquire_execution(self) -> str | None:
        """Acquire the single shared execution slot without waiting."""
        token = uuid4().hex
        acquired = await self._eval(
            _ACQUIRE, (f"{self._root}:execution",), token, self.settings.lease_seconds
        )
        return token if acquired == 1 else None

    async def renew_execution(self, token: str) -> bool:
        """Renew only the token's still-valid Redis-time lease."""
        return (
            await self._eval(
                _RENEW, (f"{self._root}:execution",), token, self.settings.lease_seconds
            )
            == 1
        )

    async def release_execution(self, token: str) -> None:
        """Release this token only after its local activity exits."""
        _ = await self._eval(_RELEASE, (f"{self._root}:execution",), token)
