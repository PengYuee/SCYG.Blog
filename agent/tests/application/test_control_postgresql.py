"""Behavior regressions against the migrated isolated PostgreSQL truth store."""

from datetime import timedelta
from hashlib import sha256
from uuid import UUID, uuid4

import anyio
import pytest
from sqlalchemy import func, select
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.event_store import NotificationChannel, PostgreSQLEventStore
from scyg_agent.adapters.database.event_subscription import open_listener_connection
from scyg_agent.adapters.database.run_records import (
    AgentRunResultRecord,
    InteractionRecord,
    RunRecord,
    SuccessfulOperationRecord,
)
from scyg_agent.agents.contracts import Capability
from scyg_agent.application.control import (
    KEY_LOCK_NAMESPACE,
    ControlApplication,
    ControlError,
    ControlSnapshot,
    ResumeCommand,
)
from scyg_agent.application.event_subscription import EventSubscriptionService
from tests.adapters.database.t12_postgres_support import anyio_backend, t12_database

__all__ = ["anyio_backend", "t12_database"]

type DatabaseFixture = tuple[AsyncEngine, async_sessionmaker[AsyncSession]]


def application(sessions: async_sessionmaker[AsyncSession]) -> ControlApplication:
    store = PostgreSQLEventStore(
        sessions,
        "postgresql://unused",
        NotificationChannel.parse("agent_events"),
        open_listener_connection,
    )
    return ControlApplication(sessions, "agent_events", EventSubscriptionService(store))


@pytest.mark.anyio
async def test_success_key_replays_across_rpc_and_invalid_business_payload(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    key = str(uuid4())
    first = await app.create("owner", key, Capability.CHAT, b'{"message":"hello"}')
    replay = await app.create("owner", key, Capability.WRITE, b"null")
    cancelled = await app.cancel("owner", str(uuid4()), first.run_id)
    cross_rpc = await app.resume(
        "owner",
        key,
        ResumeCommand("different-run", "different-interaction", "invalid", None),
    )
    assert replay.run_id == first.run_id
    assert cross_rpc.run_id == first.run_id
    assert cross_rpc.status == cancelled.status == "cancelled"
    async with sessions() as session:
        assert (
            await session.execute(
                select(func.count()).select_from(RunRecord),
            )
        ).scalar_one() == 1


@pytest.mark.anyio
async def test_failed_create_does_not_reserve_key_and_expired_success_can_create(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    key = str(uuid4())
    with pytest.raises(ControlError) as failure:
        _ = await app.create("owner", key, Capability.CHAT, b"{}")
    assert failure.value.code == "INVALID_ARGUMENT"
    first = await app.create("owner", key, Capability.CHAT, b'{"message":"first"}')
    async with sessions.begin() as session:
        record = (await session.execute(select(SuccessfulOperationRecord))).scalar_one()
        record.expires_at = record.succeeded_at - timedelta(seconds=1)
    second = await app.create("owner", key, Capability.CHAT, b'{"message":"second"}')
    assert second.run_id != first.run_id


@pytest.mark.anyio
async def test_concurrent_same_key_creates_only_one_run(t12_database: DatabaseFixture) -> None:
    _, sessions = t12_database
    app = application(sessions)
    key = str(uuid4())
    results: list[ControlSnapshot] = []

    async def create() -> None:
        results.append(await app.create("owner", key, Capability.CHAT, b'{"message":"hello"}'))

    async with anyio.create_task_group() as group:
        for _ in range(8):
            _ = group.start_soon(create)
    assert len({result.run_id for result in results}) == 1


@pytest.mark.anyio
async def test_owner_snapshot_result_null_and_pending_pointer(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    first = await app.create("owner", str(uuid4()), Capability.CHAT, b'{"message":"hello"}')
    assert not first.result_present
    async with sessions.begin() as session:
        session.add(
            AgentRunResultRecord(
                run_id=first.run_id,
                schema_version="v1",
                capability="chat",
                result_payload=JSONB.NULL,
                result_digest="0" * 64,
            ),
        )
    snapshot = await app.get("owner", first.run_id)
    assert snapshot.result_present
    assert snapshot.result is None
    for run_id in (first.run_id, "legal-but-absent.~"):
        with pytest.raises(ControlError) as failure:
            _ = await app.get("nonowner", run_id)
        assert failure.value.code == "NOT_FOUND"


@pytest.mark.anyio
@pytest.mark.parametrize(("payload", "present"), [(None, False), (b"null", True)])
async def test_resume_preserves_payload_presence_and_new_key_cannot_replace_decision(
    t12_database: DatabaseFixture,
    payload: bytes | None,
    *,
    present: bool,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    first = await app.create("owner", str(uuid4()), Capability.WRITE, b'{"topic":"article"}')
    async with sessions.begin() as session:
        run = (
            await session.execute(
                select(RunRecord).where(RunRecord.run_id == first.run_id),
            )
        ).scalar_one()
        run.status = "waiting_input"
        run.pending_interaction_id = "opaque-interaction"
        session.add(
            InteractionRecord(
                interaction_id="opaque-interaction",
                run_id=first.run_id,
                kind="confirmation",
                status="pending",
                requested_at=run.created_at,
                request_semantic_digest="0" * 64,
                request_payload={"title": "draft"},
            ),
        )
    waiting = await app.get("owner", first.run_id)
    assert waiting.interaction is not None
    assert waiting.interaction.payload == {"title": "draft"}
    resumed = await app.resume(
        "owner",
        str(uuid4()),
        ResumeCommand(first.run_id, "opaque-interaction", "approve", payload),
    )
    assert resumed.status == "pending_resume"
    assert resumed.interaction is None
    with pytest.raises(ControlError) as failure:
        _ = await app.resume(
            "owner",
            str(uuid4()),
            ResumeCommand(first.run_id, "opaque-interaction", "reject", None),
        )
    assert failure.value.code == "FAILED_PRECONDITION"
    async with sessions() as session:
        interaction = (await session.execute(select(InteractionRecord))).scalar_one()
        assert interaction.payload_present is present
        assert interaction.response_payload is None
        assert interaction.decision == "approve"


@pytest.mark.anyio
async def test_cancel_revokes_lease_and_keeps_interaction_history(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    first = await app.create("owner", str(uuid4()), Capability.WRITE, b'{"topic":"article"}')
    async with sessions.begin() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        run.status = "waiting_input"
        run.pending_interaction_id = "cancel-history"
        session.add(
            InteractionRecord(
                interaction_id="cancel-history",
                run_id=run.run_id,
                kind="confirmation",
                status="pending",
                requested_at=run.created_at,
                request_semantic_digest="0" * 64,
            ),
        )
    cancelled = await app.cancel("owner", str(uuid4()), first.run_id)
    assert cancelled.status == "cancelled"
    assert cancelled.interaction is None
    async with sessions() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        assert run.lease_token is None
        assert run.lease_owner is None
        assert run.lease_expires_at is None
        assert (await session.execute(select(InteractionRecord))).scalar_one().status == "pending"


@pytest.mark.anyio
async def test_waiting_key_lock_rechecks_database_expiry(t12_database: DatabaseFixture) -> None:
    _, sessions = t12_database
    app = application(sessions)
    key = str(uuid4())
    first = await app.create("owner", key, Capability.CHAT, b'{"message":"first"}')
    hash_value = int.from_bytes(
        sha256(f"owner:{UUID(key)}".encode()).digest()[:4],
        "big",
        signed=True,
    )
    results: list[ControlSnapshot] = []
    started = anyio.Event()

    async def recreate() -> None:
        started.set()
        results.append(await app.create("owner", key, Capability.CHAT, b'{"message":"second"}'))

    async with anyio.create_task_group() as group, sessions.begin() as session:
        _ = await session.execute(
            select(func.pg_advisory_xact_lock(KEY_LOCK_NAMESPACE, hash_value)),
        )
        record = (await session.execute(select(SuccessfulOperationRecord))).scalar_one()
        record.expires_at = (
            await session.execute(
                select(func.clock_timestamp()),
            )
        ).scalar_one() + timedelta(milliseconds=100)
        await session.flush()
        _ = group.start_soon(recreate)
        await started.wait()
        await anyio.sleep(0.15)
    # The waiting application can only observe the post-lock expired record.
    assert len(results) == 1
    assert results[0].run_id != first.run_id


@pytest.mark.anyio
async def test_cannot_cancel_success_terminal_and_failure_key_is_reusable(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    first = await app.create("owner", str(uuid4()), Capability.CHAT, b'{"message":"hello"}')
    async with sessions.begin() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        run.status = "succeeded"
    key = str(uuid4())
    with pytest.raises(ControlError) as failure:
        _ = await app.cancel("owner", key, first.run_id)
    assert failure.value.code == "FAILED_PRECONDITION"
    created = await app.create("owner", key, Capability.CHAT, b'{"message":"retry"}')
    assert created.run_id != first.run_id


@pytest.mark.anyio
async def test_unpersistable_create_is_invalid_and_does_not_claim_success_key(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    key = str(uuid4())
    invalid = b'{"message":"hello\\u0000world"}'
    with pytest.raises(ControlError) as failure:
        _ = await app.create("owner", key, Capability.CHAT, invalid)
    assert (failure.value.code, failure.value.message) == ("INVALID_ARGUMENT", "请求参数无效")
    async with sessions() as session:
        assert (
            await session.execute(select(func.count()).select_from(RunRecord))
        ).scalar_one() == 0
        assert (
            await session.execute(select(func.count()).select_from(SuccessfulOperationRecord))
        ).scalar_one() == 0
    first = await app.create("owner", key, Capability.CHAT, b'{"message":"hello"}')
    replay = await app.create("owner", key, Capability.CHAT, invalid)
    assert replay.run_id == first.run_id


@pytest.mark.anyio
@pytest.mark.parametrize(
    "payload",
    [
        b'{"value":1e400}',
        b'{"nested":["hello\\u0000world"]}',
        b'{"nested":{"bad\\u0000key":null}}',
        b'{"nested":["\\ud800"]}',
    ],
)
async def test_unpersistable_resume_preserves_interaction_and_reusable_success_key(
    t12_database: DatabaseFixture,
    payload: bytes,
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    first = await app.create("owner", str(uuid4()), Capability.WRITE, b'{"topic":"article"}')
    async with sessions.begin() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        run.status = "waiting_input"
        run.pending_interaction_id = "persistence-interaction"
        session.add(
            InteractionRecord(
                interaction_id="persistence-interaction",
                run_id=first.run_id,
                kind="confirmation",
                status="pending",
                requested_at=run.created_at,
                request_semantic_digest="0" * 64,
            ),
        )
    key = str(uuid4())
    command = ResumeCommand(first.run_id, "persistence-interaction", "approve", payload)
    with pytest.raises(ControlError) as failure:
        _ = await app.resume("owner", key, command)
    assert (failure.value.code, failure.value.message) == (
        "INVALID_ARGUMENT",
        "交互响应 payload 无效",
    )
    async with sessions() as session:
        interaction = (await session.execute(select(InteractionRecord))).scalar_one()
        assert interaction.status == "pending"
        assert interaction.decision is None
        assert (
            await session.execute(
                select(SuccessfulOperationRecord).where(
                    SuccessfulOperationRecord.idempotency_key == UUID(key)
                )
            )
        ).scalar_one_or_none() is None
    valid = b'{"value":1e300,"nested":["\\ud83d\\ude00",null,"literal \\u005cu0000"]}'
    resumed = await app.resume(
        "owner", key, ResumeCommand(first.run_id, "persistence-interaction", "approve", valid)
    )
    assert resumed.status == "pending_resume"
    replay = await app.resume("owner", key, command)
    assert replay.run_id == resumed.run_id
    assert replay.status == resumed.status
    async with sessions() as session:
        interaction = (await session.execute(select(InteractionRecord))).scalar_one()
        assert interaction.payload_present
        assert interaction.response_payload == {
            "value": 10**300,
            "nested": ["\U0001f600", None, "literal \\u0000"],
        }
