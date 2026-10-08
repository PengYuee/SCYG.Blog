"""Five generated RPCs exercise production transactions and PostgreSQL listeners."""

import json
from typing import cast
from uuid import uuid4

import grpc
import pytest
from sqlalchemy import func, select
from tests.integration.t20.postgres_support import T20Database, grpc_stub, migration_environment

from scyg_agent.adapters.database.run_records import (
    InteractionRecord,
    RunRecord,
    SuccessfulOperationRecord,
)
from scyg_agent.application.event_cursor import encode_event_cursor
from scyg_agent.domain.ports.event_store import EventCursor
from scyg_agent.domain.runs import RunId
from scyg_agent.generated.proto.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.proto.scyg.agent.v1 import common_pb2

USER = "user-t20"


def request(
    key: str | None = None,
    payload: bytes = b'{"message":"persisted input"}',
) -> service_pb2.CreateRunRequest:
    return service_pb2.CreateRunRequest(
        user_id=USER,
        idempotency_key=key or str(uuid4()),
        capability=common_pb2.AGENT_CAPABILITY_CHAT,
        json_payload=payload,
    )


def get_request(run_id: str, user: str = USER) -> service_pb2.GetRunRequest:
    return service_pb2.GetRunRequest(user_id=user, run_id=run_id)


@pytest.mark.anyio
async def test_create_get_and_successful_key_survive_server_and_engine_reconstruction(
    database: T20Database,
) -> None:
    create = request()
    async with grpc_stub(database.application()) as stub:
        with pytest.raises(grpc.aio.AioRpcError) as failure:
            _ = await stub.CreateRun(request(create.idempotency_key, b"null"), timeout=5)
        assert failure.value.code() is grpc.StatusCode.INVALID_ARGUMENT
        # A deadline-bound business error must roll back without reserving its key.
        first = await stub.CreateRun(create, timeout=5)
        assert first.status == common_pb2.RUN_STATUS_QUEUED
        assert first.capability == common_pb2.AGENT_CAPABILITY_CHAT
        assert first.recipe_id == "chat-v1"
        assert not first.HasField("result_json")
    await database.close()
    environment = migration_environment()
    rebuilt = T20Database.create(
        environment["SCYG_AGENT_DATABASE_URL"],
        environment["SCYG_T20_LISTENER_DSN"],
    )
    try:
        async with grpc_stub(rebuilt.application()) as stub:
            fetched = await stub.GetRun(get_request(first.run_id), timeout=5)
            assert fetched == first
            # A successful key binds a Run, not a request digest or RPC name.
            replay = await stub.CreateRun(request(create.idempotency_key, b"{}"), timeout=5)
            assert replay == fetched
            cross_rpc = await stub.CancelRun(
                service_pb2.CancelRunRequest(
                    user_id=USER,
                    run_id="missing-run",
                    idempotency_key=create.idempotency_key,
                ),
                timeout=5,
            )
            assert cross_rpc == fetched
            for call in (
                stub.GetRun(get_request(first.run_id, "other-owner"), timeout=5),
                stub.CancelRun(
                    service_pb2.CancelRunRequest(
                        user_id="other-owner",
                        run_id=first.run_id,
                        idempotency_key=str(uuid4()),
                    ),
                    timeout=5,
                ),
            ):
                with pytest.raises(grpc.aio.AioRpcError) as captured:
                    _ = await call
                assert captured.value.code() is grpc.StatusCode.NOT_FOUND
        async with rebuilt.sessions() as session:
            row = await session.get(RunRecord, first.run_id)
            assert row is not None
            assert row.input_payload == {"message": "persisted input"}
            assert row.initial_message == "persisted input"
            assert (
                await session.execute(
                    select(func.count()).select_from(RunRecord),
                )
            ).scalar_one() == 1
            assert (
                await session.execute(
                    select(func.count()).select_from(SuccessfulOperationRecord),
                )
            ).scalar_one() == 1
    finally:
        await rebuilt.close()


async def waiting_interaction(
    database: T20Database,
    run_id: str,
    kind: str = "confirmation",
) -> str:
    """Seed the durable worker-produced wait state, not a mocked control response."""
    interaction_id = f"int_{uuid4().hex}"
    async with database.sessions.begin() as session:
        row = await session.get(RunRecord, run_id)
        assert row is not None
        row.status = "waiting_input"
        row.pending_interaction_id = interaction_id
        row.revision += 1
        session.add(
            InteractionRecord(
                interaction_id=interaction_id,
                run_id=run_id,
                kind=kind,
                status="pending",
                requested_at=row.updated_at,
                request_semantic_digest="a" * 64,
                request_payload={"prompt": "Continue?"},
            )
        )
    return interaction_id


@pytest.mark.anyio
@pytest.mark.parametrize("payload_present", [False, True])
async def test_resume_persists_decision_and_distinguishes_missing_from_null(
    database: T20Database,
    *,
    payload_present: bool,
) -> None:
    async with grpc_stub(database.application()) as stub:
        created = await stub.CreateRun(request(), timeout=5)
    interaction_id = await waiting_interaction(database, created.run_id)
    resume = service_pb2.ResumeRunRequest(
        user_id=USER,
        run_id=created.run_id,
        interaction_id=interaction_id,
        idempotency_key=str(uuid4()),
        decision="approve",
    )
    if payload_present:
        resume.payload_json = b"null"
    async with grpc_stub(database.application()) as stub:
        waiting = await stub.GetRun(get_request(created.run_id), timeout=5)
        assert waiting.status == common_pb2.RUN_STATUS_WAITING_FOR_APPROVAL
        assert waiting.pending_interaction.interaction_id == interaction_id
        assert json.loads(waiting.pending_interaction.payload_json) == {"prompt": "Continue?"}
        wrong_owner = service_pb2.ResumeRunRequest()
        wrong_owner.CopyFrom(resume)
        wrong_owner.user_id = "other-owner"
        with pytest.raises(grpc.aio.AioRpcError) as captured:
            _ = await stub.ResumeRun(wrong_owner, timeout=5)
        assert captured.value.code() is grpc.StatusCode.NOT_FOUND
        resumed = await stub.ResumeRun(resume, timeout=5)
        assert resumed.status == common_pb2.RUN_STATUS_QUEUED
        assert not resumed.HasField("pending_interaction")
    async with grpc_stub(database.application()) as rebuilt:
        assert await rebuilt.GetRun(get_request(created.run_id), timeout=5) == resumed
        assert await rebuilt.ResumeRun(resume, timeout=5) == resumed
    async with database.sessions() as session:
        row = await session.get(InteractionRecord, interaction_id)
        assert row is not None
        assert row.status == "resolved"
        assert row.decision == "approve"
        assert row.payload_present is payload_present
        assert row.response_payload is None
        run = await session.get(RunRecord, created.run_id)
        assert run is not None
        assert run.status == "pending_resume"
        assert run.pending_interaction_id is None
        assert run.next_attempt_at is not None


@pytest.mark.anyio
async def test_stream_ready_before_first_event_cancel_delivery_reconnect_and_owner_errors(
    database: T20Database,
) -> None:
    async with grpc_stub(database.application()) as stub:
        created = await stub.CreateRun(request(), timeout=5)
        stream = stub.StreamRunEvents(
            service_pb2.StreamRunEventsRequest(user_id=USER, run_id=created.run_id),
            timeout=5,
        )
        try:
            # Empty journal readiness must not wait for an event or ten-second heartbeat.
            metadata = await stream.initial_metadata()
            assert ("scyg-subscription-ready", "1") in tuple(metadata)
            cancelled = await stub.CancelRun(
                service_pb2.CancelRunRequest(
                    user_id=USER,
                    run_id=created.run_id,
                    idempotency_key=str(uuid4()),
                ),
                timeout=5,
            )
            assert cancelled.status == common_pb2.RUN_STATUS_CANCELLED
            frames = [event.frame async for event in stream]
            assert len(frames) == 2
            assert all(frame.endswith(b"\n\n") for frame in frames)
            data = [
                cast("dict[str, object]", json.loads(frame.decode().split("data: ", 1)[1]))
                for frame in frames
            ]
            assert [event["kind"] for event in data] == ["status_changed", "run_cancelled"]
            assert all(event["run_id"] == created.run_id for event in data)
            cursor = frames[0].decode().split("\n", 1)[0].removeprefix("id: ")
        finally:
            _ = stream.cancel()
    async with grpc_stub(database.application()) as rebuilt:
        assert await rebuilt.GetRun(get_request(created.run_id), timeout=5) == cancelled
        replay = rebuilt.StreamRunEvents(
            service_pb2.StreamRunEventsRequest(
                user_id=USER,
                run_id=created.run_id,
                after_event_id=cursor,
            ),
            timeout=5,
        )
        assert ("scyg-subscription-ready", "1") in tuple(await replay.initial_metadata())
        replayed = [event.frame async for event in replay]
        assert replayed == frames[1:]
        second = await rebuilt.CreateRun(request(), timeout=5)
        for user, run_id, after, expected in (
            ("other-owner", created.run_id, None, grpc.StatusCode.NOT_FOUND),
            (USER, "missing-run", None, grpc.StatusCode.NOT_FOUND),
            (USER, second.run_id, cursor, grpc.StatusCode.INVALID_ARGUMENT),
            (USER, created.run_id, "not-a-cursor", grpc.StatusCode.INVALID_ARGUMENT),
            (
                USER,
                created.run_id,
                encode_event_cursor(RunId(created.run_id), EventCursor(999)),
                grpc.StatusCode.FAILED_PRECONDITION,
            ),
        ):
            incoming = service_pb2.StreamRunEventsRequest(user_id=user, run_id=run_id)
            if after is not None:
                incoming.after_event_id = after
            denied = rebuilt.StreamRunEvents(incoming, timeout=5)
            assert ("scyg-subscription-ready", "1") not in tuple(await denied.initial_metadata())
            with pytest.raises(grpc.aio.AioRpcError) as captured:
                _ = await denied.read()
            assert captured.value.code() is expected
    async with database.sessions() as session:
        row = await session.get(RunRecord, created.run_id)
        assert row is not None
        assert row.status == "cancelled"
        assert row.terminal_at is not None
        assert row.cancellation_requested_at is not None
        assert row.lease_owner is None
        assert row.lease_token is None
        assert row.lease_expires_at is None
        assert row.next_attempt_at is None
