"""Public quota and lease behavior against an explicitly configured test Redis."""

import os
from collections.abc import AsyncIterator
from dataclasses import replace
from uuid import uuid4

import pytest
from pydantic import SecretStr

from scyg_agent.adapters.redis import RedisSettings, RedisStreamClient
from scyg_agent.adapters.redis.public_controls import (
    PublicQuotaExceededError,
    RedisPublicControls,
)
from scyg_agent.config import PublicSearchSettings


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.fixture
async def public_controls() -> AsyncIterator[tuple[RedisPublicControls, RedisStreamClient]]:
    endpoint = os.environ.get("SCYG_TEST_REDIS_URL")
    if not endpoint:
        pytest.skip("SCYG_TEST_REDIS_URL is required for real Redis public policy tests")
    stream = RedisStreamClient.create(RedisSettings(SecretStr(endpoint)))
    prefix = f"scyg-test:{uuid4().hex}"
    controls = RedisPublicControls(
        stream.client, PublicSearchSettings(key_prefix=prefix, daily_creations=1)
    )
    try:
        yield controls, stream
    finally:
        cleanup = stream.client.register_script("""
local cursor = '0'
repeat
 local result = redis.call('SCAN', cursor, 'MATCH', ARGV[1] .. ':*')
 cursor = result[1]
 for _, key in ipairs(result[2]) do redis.call('DEL', key) end
until cursor == '0'
return 1
""")
        _ = await cleanup(keys=[], args=[prefix], client=stream.client)
        await stream.close()


@pytest.mark.anyio
async def test_reservation_replay_refund_token_and_consumed_quota(
    public_controls: tuple[RedisPublicControls, RedisStreamClient],
) -> None:
    controls, _stream = public_controls
    first = await controls.reserve("owner", "key")
    resumed = await controls.reserve("owner", "key")
    await controls.refund(first)
    with pytest.raises(PublicQuotaExceededError) as exhausted:
        _ = await controls.reserve("other", "key")
    assert 0 < exhausted.value.retry_after_seconds <= 86400
    await controls.refund(resumed)
    await controls.refund(resumed)
    replacement = await controls.reserve("owner", "replacement")
    await controls.consume(replacement)
    await controls.refund(replacement)
    with pytest.raises(PublicQuotaExceededError):
        _ = await controls.reserve("owner", "third")


@pytest.mark.anyio
async def test_two_workers_share_execution_and_release_only_their_token(
    public_controls: tuple[RedisPublicControls, RedisStreamClient],
) -> None:
    first, stream = public_controls
    second = RedisPublicControls(stream.client, first.settings)
    token = await first.acquire_execution()
    assert token is not None
    assert await second.acquire_execution() is None
    await second.release_execution(uuid4().hex)
    assert await first.renew_execution(token)
    await first.release_execution(token)
    successor = await second.acquire_execution()
    assert successor is not None
    assert not await first.renew_execution(token)
    await first.release_execution(token)
    assert await second.renew_execution(successor)
    await second.release_execution(successor)


@pytest.mark.anyio
async def test_late_refund_changes_only_reservation_original_day(
    public_controls: tuple[RedisPublicControls, RedisStreamClient],
) -> None:
    controls, stream = public_controls
    reservation = await controls.reserve("owner", "late")
    previous = replace(reservation, day=reservation.day - 1)
    root = f"{controls.settings.key_prefix}:{{public-search}}"
    seed = stream.client.register_script("""
redis.call('HSET', KEYS[1], ARGV[1], 1)
redis.call('HSET', KEYS[2], 'day', ARGV[1])
return 1
""")
    _ = await seed(
        keys=[f"{root}:days", f"{root}:op:{reservation.operation_key}"],
        args=[str(previous.day)],
        client=stream.client,
    )
    await controls.refund(previous)
    await controls.refund(previous)
    read = stream.client.register_script(
        "return {redis.call('HGET', KEYS[1], ARGV[1]), redis.call('HGET', KEYS[1], ARGV[2])}"
    )
    observed = await read(
        keys=[f"{root}:days"], args=[str(previous.day), str(reservation.day)], client=stream.client
    )
    assert observed == ["0", "1"]
