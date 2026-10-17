"""Behavior regressions against the migrated isolated PostgreSQL truth store."""

import json
from datetime import datetime, timedelta
from hashlib import sha256
from typing import cast
from uuid import UUID, uuid4

import anyio
import pytest
from sqlalchemy import func, select
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.ext.asyncio import (
    AsyncEngine,
    AsyncSession,
    AsyncSessionTransaction,
    async_sessionmaker,
)

from scyg_agent.adapters.database.event_store import NotificationChannel, PostgreSQLEventStore
from scyg_agent.adapters.database.event_subscription import open_listener_connection
from scyg_agent.adapters.database.run_records import (
    AgentRunResultRecord,
    InteractionRecord,
    RunRecord,
    SuccessfulOperationRecord,
)
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.adapters.redis.public_controls import (
    PublicQuotaExceededError,
    PublicReservation,
)
from scyg_agent.agents.contracts import Capability
from scyg_agent.application.control import (
    KEY_LOCK_NAMESPACE,
    ControlApplication,
    ControlError,
    ControlSnapshot,
    PublicControlConfig,
    ResumeCommand,
)
from scyg_agent.application.event_subscription import EventSubscriptionService
from scyg_agent.config import PublicSearchSettings
from scyg_agent.domain.runs import ContentScope, ExecutionOwnerId, Run, RunId, RuntimeKind
from scyg_agent.domain.runs.repository import ClaimRequest
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
    first = await app.create(
        "owner", key, Capability.CHAT, b'{"message":"hello"}', content_scope=ContentScope.AUTHOR
    )
    replay = await app.create(
        "owner", key, Capability.WRITE, b"null", content_scope=ContentScope.AUTHOR
    )
    cancelled = await app.cancel(
        "owner", str(uuid4()), first.run_id, content_scope=ContentScope.AUTHOR
    )
    cross_rpc = await app.resume(
        "owner",
        key,
        ResumeCommand("different-run", "different-interaction", "invalid", None),
        content_scope=ContentScope.AUTHOR,
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
        _ = await app.create(
            "owner", key, Capability.CHAT, b"{}", content_scope=ContentScope.AUTHOR
        )
    assert failure.value.code == "INVALID_ARGUMENT"
    first = await app.create(
        "owner", key, Capability.CHAT, b'{"message":"first"}', content_scope=ContentScope.AUTHOR
    )
    async with sessions.begin() as session:
        record = (await session.execute(select(SuccessfulOperationRecord))).scalar_one()
        record.expires_at = record.succeeded_at - timedelta(seconds=1)
    second = await app.create(
        "owner", key, Capability.CHAT, b'{"message":"second"}', content_scope=ContentScope.AUTHOR
    )
    assert second.run_id != first.run_id


@pytest.mark.anyio
async def test_concurrent_same_key_creates_only_one_run(t12_database: DatabaseFixture) -> None:
    _, sessions = t12_database
    app = application(sessions)
    key = str(uuid4())
    results: list[ControlSnapshot] = []

    async def create() -> None:
        results.append(
            await app.create(
                "owner",
                key,
                Capability.CHAT,
                b'{"message":"hello"}',
                content_scope=ContentScope.AUTHOR,
            )
        )

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
    first = await app.create(
        "owner",
        str(uuid4()),
        Capability.CHAT,
        b'{"message":"hello"}',
        content_scope=ContentScope.AUTHOR,
    )
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
    snapshot = await app.get("owner", first.run_id, content_scope=ContentScope.AUTHOR)
    assert snapshot.result_present
    assert snapshot.result is None
    for run_id in (first.run_id, "legal-but-absent.~"):
        with pytest.raises(ControlError) as failure:
            _ = await app.get("nonowner", run_id, content_scope=ContentScope.AUTHOR)
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
    first = await app.create(
        "owner",
        str(uuid4()),
        Capability.WRITE,
        b'{"topic":"article"}',
        content_scope=ContentScope.AUTHOR,
    )
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
    waiting = await app.get("owner", first.run_id, content_scope=ContentScope.AUTHOR)
    assert waiting.interaction is not None
    assert waiting.interaction.payload == {"title": "draft"}
    resumed = await app.resume(
        "owner",
        str(uuid4()),
        ResumeCommand(first.run_id, "opaque-interaction", "approve", payload),
        content_scope=ContentScope.AUTHOR,
    )
    assert resumed.status == "pending_resume"
    assert resumed.interaction is None
    with pytest.raises(ControlError) as failure:
        _ = await app.resume(
            "owner",
            str(uuid4()),
            ResumeCommand(first.run_id, "opaque-interaction", "reject", None),
            content_scope=ContentScope.AUTHOR,
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
    first = await app.create(
        "owner",
        str(uuid4()),
        Capability.WRITE,
        b'{"topic":"article"}',
        content_scope=ContentScope.AUTHOR,
    )
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
    cancelled = await app.cancel(
        "owner", str(uuid4()), first.run_id, content_scope=ContentScope.AUTHOR
    )
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
    first = await app.create(
        "owner", key, Capability.CHAT, b'{"message":"first"}', content_scope=ContentScope.AUTHOR
    )
    hash_value = int.from_bytes(
        sha256(f"owner:{UUID(key)}".encode()).digest()[:4],
        "big",
        signed=True,
    )
    results: list[ControlSnapshot] = []
    started = anyio.Event()

    async def recreate() -> None:
        started.set()
        results.append(
            await app.create(
                "owner",
                key,
                Capability.CHAT,
                b'{"message":"second"}',
                content_scope=ContentScope.AUTHOR,
            )
        )

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
    first = await app.create(
        "owner",
        str(uuid4()),
        Capability.CHAT,
        b'{"message":"hello"}',
        content_scope=ContentScope.AUTHOR,
    )
    async with sessions.begin() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        run.status = "succeeded"
    key = str(uuid4())
    with pytest.raises(ControlError) as failure:
        _ = await app.cancel("owner", key, first.run_id, content_scope=ContentScope.AUTHOR)
    assert failure.value.code == "FAILED_PRECONDITION"
    created = await app.create(
        "owner", key, Capability.CHAT, b'{"message":"retry"}', content_scope=ContentScope.AUTHOR
    )
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
        _ = await app.create(
            "owner", key, Capability.CHAT, invalid, content_scope=ContentScope.AUTHOR
        )
    assert (failure.value.code, failure.value.message) == ("INVALID_ARGUMENT", "请求参数无效")
    async with sessions() as session:
        assert (
            await session.execute(select(func.count()).select_from(RunRecord))
        ).scalar_one() == 0
        assert (
            await session.execute(select(func.count()).select_from(SuccessfulOperationRecord))
        ).scalar_one() == 0
    first = await app.create(
        "owner", key, Capability.CHAT, b'{"message":"hello"}', content_scope=ContentScope.AUTHOR
    )
    replay = await app.create(
        "owner", key, Capability.CHAT, invalid, content_scope=ContentScope.AUTHOR
    )
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
    first = await app.create(
        "owner",
        str(uuid4()),
        Capability.WRITE,
        b'{"topic":"article"}',
        content_scope=ContentScope.AUTHOR,
    )
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
        _ = await app.resume("owner", key, command, content_scope=ContentScope.AUTHOR)
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
        "owner",
        key,
        ResumeCommand(first.run_id, "persistence-interaction", "approve", valid),
        content_scope=ContentScope.AUTHOR,
    )
    assert resumed.status == "pending_resume"
    replay = await app.resume("owner", key, command, content_scope=ContentScope.AUTHOR)
    assert replay.run_id == resumed.run_id
    assert replay.status == resumed.status
    async with sessions() as session:
        interaction = (await session.execute(select(InteractionRecord))).scalar_one()
        assert interaction.payload_present
        assert interaction.response_payload == {
            "value": 10**300,
            "nested": ["\U0001f600", None, "literal \\u0000"],
        }


class QuotaControls:
    """Record admission quota transitions and inject deterministic exhaustion."""

    def __init__(self) -> None:
        self.reserved: list[PublicReservation] = []
        self.consumed: list[PublicReservation] = []
        self.refunded: list[PublicReservation] = []
        self.exhausted: bool = False

    async def reserve(self, owner: str, key: str) -> PublicReservation:
        if self.exhausted:
            raise PublicQuotaExceededError(42)
        value = PublicReservation(uuid4().hex, 20000, f"{owner}:{key}")
        self.reserved.append(value)
        return value

    async def consume(self, reservation: PublicReservation) -> None:
        self.consumed.append(reservation)

    async def refund(self, reservation: PublicReservation) -> None:
        self.refunded.append(reservation)


def public_application(
    sessions: async_sessionmaker[AsyncSession], controls: QuotaControls
) -> ControlApplication:
    store = PostgreSQLEventStore(
        sessions,
        "postgresql://unused",
        NotificationChannel.parse("agent_events"),
        open_listener_connection,
    )
    return ControlApplication(
        sessions,
        "agent_events",
        EventSubscriptionService(store),
        public=PublicControlConfig(settings=PublicSearchSettings(), controls=controls),
    )


@pytest.mark.anyio
async def test_public_admission_replay_and_owner_scope(t12_database: DatabaseFixture) -> None:
    _, sessions = t12_database
    controls = QuotaControls()
    app = public_application(sessions, controls)
    key = str(uuid4())
    first = await app.create(
        "anonymous",
        key,
        Capability.SEARCH,
        b'{"query":"public"}',
        content_scope=ContentScope.PUBLIC,
    )
    for _ in range(9):
        _ = await app.create(
            "anonymous",
            str(uuid4()),
            Capability.SEARCH,
            b'{"query":"public"}',
            content_scope=ContentScope.PUBLIC,
        )
    replay = await app.create(
        "anonymous", key, Capability.WRITE, b"invalid JSON", content_scope=ContentScope.PUBLIC
    )
    assert replay.run_id == first.run_id
    assert len(controls.reserved) == len(controls.consumed) == 10
    with pytest.raises(ControlError) as full:
        _ = await app.create(
            "anonymous",
            str(uuid4()),
            Capability.SEARCH,
            b'{"query":"public"}',
            content_scope=ContentScope.PUBLIC,
        )
    assert full.value.code == "RESOURCE_EXHAUSTED"
    assert len(controls.reserved) == 10
    for owner, scope in (("other", ContentScope.PUBLIC), ("anonymous", ContentScope.AUTHOR)):
        with pytest.raises(ControlError) as denied:
            _ = await app.get(owner, first.run_id, content_scope=scope)
        assert denied.value.code == "NOT_FOUND"
    _ = await app.cancel("anonymous", str(uuid4()), first.run_id, content_scope=ContentScope.PUBLIC)
    _ = await app.create(
        "anonymous",
        str(uuid4()),
        Capability.SEARCH,
        b'{"query":"public"}',
        content_scope=ContentScope.PUBLIC,
    )
    assert not controls.refunded


@pytest.mark.anyio
async def test_public_prevalidation_quota_and_definite_rollback(
    t12_database: DatabaseFixture, monkeypatch: pytest.MonkeyPatch
) -> None:
    _, sessions = t12_database
    controls = QuotaControls()
    app = public_application(sessions, controls)
    for capability, payload in (
        (Capability.WRITE, b'{"topic":"no"}'),
        (Capability.SEARCH, b'{"query":"a","max_results":6}'),
    ):
        with pytest.raises(ControlError) as invalid:
            _ = await app.create(
                "anonymous", str(uuid4()), capability, payload, content_scope=ContentScope.PUBLIC
            )
        assert invalid.value.code == "INVALID_ARGUMENT"
    assert not controls.reserved
    controls.exhausted = True
    with pytest.raises(ControlError) as exhausted:
        _ = await app.create(
            "anonymous",
            str(uuid4()),
            Capability.SEARCH,
            b'{"query":"valid"}',
            content_scope=ContentScope.PUBLIC,
        )
    assert exhausted.value.retry_after_seconds == 42
    controls.exhausted = False

    async def failed_insert(*_args: object, **_kwargs: object) -> None:
        message = "database unavailable"
        raise OSError(message)

    monkeypatch.setattr(PostgreSQLRunRepository, "create_in_session", failed_insert)
    with pytest.raises(OSError, match="database unavailable"):
        _ = await app.create(
            "anonymous",
            str(uuid4()),
            Capability.SEARCH,
            b'{"query":"valid"}',
            content_scope=ContentScope.PUBLIC,
        )
    assert controls.refunded == controls.reserved
    assert not controls.consumed


@pytest.mark.anyio
async def test_public_commit_unknown_never_refunds(
    t12_database: DatabaseFixture, monkeypatch: pytest.MonkeyPatch
) -> None:
    _, sessions = t12_database
    controls = QuotaControls()
    app = public_application(sessions, controls)

    async def unknown_commit(_self: AsyncSessionTransaction) -> None:
        message = "commit acknowledgment lost"
        raise OSError(message)

    monkeypatch.setattr(AsyncSessionTransaction, "commit", unknown_commit)
    with pytest.raises(OSError, match="commit acknowledgment lost"):
        _ = await app.create(
            "anonymous",
            str(uuid4()),
            Capability.SEARCH,
            b'{"query":"valid"}',
            content_scope=ContentScope.PUBLIC,
        )
    assert len(controls.reserved) == 1
    assert not controls.refunded
    assert not controls.consumed


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("kind", "invalid", "valid"),
    cast(
        "list[tuple[str, list[object], object]]",
        [
            ("text_input", [None, "  ", "x" * 16001, {"text": "wrong"}], "  useful requirements  "),
            (
                "selection",
                [
                    None,
                    {"article_ids": [1, 1]},
                    {"article_ids": [3]},
                    {"article_ids": [True]},
                    {"article_ids": [], "extra": 1},
                ],
                {"article_ids": []},
            ),
        ],
    ),
)
async def test_resume_business_validation_preserves_pending_and_once_only(
    t12_database: DatabaseFixture, kind: str, invalid: list[object], valid: object
) -> None:
    _, sessions = t12_database
    app = application(sessions)
    first = await app.create(
        "owner",
        str(uuid4()),
        Capability.WRITE,
        b'{"topic":"article"}',
        content_scope=ContentScope.AUTHOR,
    )
    request: dict[str, object] = {
        "interaction_id": "business-input",
        "kind": kind,
        "title": "Preparation",
        "prompt": "Choose",
    }
    if kind == "selection":
        request.update(
            options=[
                {
                    "article_id": item,
                    "title": f"Article {item}",
                    "digest": "",
                    "status": "published",
                }
                for item in (1, 2)
            ],
            min_selections=0,
            max_selections=2,
        )
    else:
        request.update(min_length=1, max_length=16000)
    async with sessions.begin() as session:
        run = (await session.execute(select(RunRecord))).scalar_one()
        run.status = "waiting_input"
        run.pending_interaction_id = "business-input"
        session.add(
            InteractionRecord(
                interaction_id="business-input",
                run_id=first.run_id,
                kind=kind,
                status="pending",
                requested_at=run.created_at,
                request_semantic_digest="0" * 64,
                request_payload=request,
            )
        )
    decision = "select" if kind == "selection" else "submit"
    for payload in invalid:
        with pytest.raises(ControlError) as error:
            _ = await app.resume(
                "owner",
                str(uuid4()),
                ResumeCommand(
                    first.run_id, "business-input", decision, json.dumps(payload).encode()
                ),
                content_scope=ContentScope.AUTHOR,
            )
        assert error.value.code == "INVALID_ARGUMENT"
        snapshot = await app.get("owner", first.run_id, content_scope=ContentScope.AUTHOR)
        assert snapshot.interaction is not None
    key = str(uuid4())
    command = ResumeCommand(first.run_id, "business-input", decision, json.dumps(valid).encode())
    _ = await app.resume("owner", key, command, content_scope=ContentScope.AUTHOR)
    _ = await app.resume("owner", key, command, content_scope=ContentScope.AUTHOR)
    with pytest.raises(ControlError) as processed:
        _ = await app.resume("owner", str(uuid4()), command, content_scope=ContentScope.AUTHOR)
    assert processed.value.code == "FAILED_PRECONDITION"
    async with sessions() as session:
        interaction = (await session.execute(select(InteractionRecord))).scalar_one()
        assert interaction.response_payload == (
            "useful requirements" if kind == "text_input" else valid
        )


@pytest.mark.anyio
async def test_claim_scope_and_database_deadline_survive_reclaim(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    app = public_application(sessions, QuotaControls())
    public = await app.create(
        "anonymous",
        str(uuid4()),
        Capability.SEARCH,
        b'{"query":"valid"}',
        content_scope=ContentScope.PUBLIC,
    )
    author = await app.create(
        "owner",
        str(uuid4()),
        Capability.SEARCH,
        b'{"query":"valid"}',
        content_scope=ContentScope.AUTHOR,
    )
    repository = PostgreSQLRunRepository(sessions)
    async with sessions() as session:
        now = cast("datetime", (await session.execute(select(func.clock_timestamp()))).scalar_one())
    author_leases = await repository.claim(
        ClaimRequest(
            5,
            ExecutionOwnerId("worker_author_scope"),
            RuntimeKind.DEEP,
            now,
            timedelta(seconds=1),
            content_scope=ContentScope.AUTHOR,
        )
    )
    assert [str(lease.run_id) for lease in author_leases] == [author.run_id]
    public_lease = (
        await repository.claim(
            ClaimRequest(
                1,
                ExecutionOwnerId("worker_public_scope"),
                RuntimeKind.DEEP,
                now,
                timedelta(seconds=1),
                content_scope=ContentScope.PUBLIC,
                public_timeout=timedelta(seconds=180),
            )
        )
    )[0]
    first = await repository.get(RunId(public.run_id))
    assert isinstance(first, Run)
    assert first.public_execution_deadline is not None
    assert first.public_execution_deadline >= now + timedelta(seconds=180)
    _ = await repository.claim(
        ClaimRequest(
            1,
            ExecutionOwnerId("worker_public_retry"),
            RuntimeKind.DEEP,
            public_lease.expires_at + timedelta(seconds=1),
            timedelta(seconds=1),
            content_scope=ContentScope.PUBLIC,
            public_timeout=timedelta(seconds=999),
        )
    )
    reclaimed = await repository.get(RunId(public.run_id))
    assert isinstance(reclaimed, Run)
    assert reclaimed.public_execution_deadline == first.public_execution_deadline


@pytest.mark.anyio
async def test_public_admission_shared_across_control_instances(
    t12_database: DatabaseFixture,
) -> None:
    _, sessions = t12_database
    controls = QuotaControls()
    applications = [public_application(sessions, controls) for _ in range(2)]
    accepted: list[str] = []
    rejected: list[str] = []

    async def create(index: int) -> None:
        try:
            snapshot = await applications[index % 2].create(
                "anonymous",
                str(uuid4()),
                Capability.SEARCH,
                b'{"query":"valid"}',
                content_scope=ContentScope.PUBLIC,
            )
        except ControlError as error:
            rejected.append(error.code)
        else:
            accepted.append(snapshot.run_id)

    async with anyio.create_task_group() as group:
        for index in range(12):
            _ = group.start_soon(create, index)
    assert len(set(accepted)) == 10
    assert rejected == ["RESOURCE_EXHAUSTED", "RESOURCE_EXHAUSTED"]
    assert len(controls.reserved) == 10
