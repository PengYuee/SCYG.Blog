"""Cursor boundaries and complete application event frames."""

import base64
import json
from datetime import UTC, datetime
from typing import cast

import pytest

from scyg_agent.application.event_cursor import (
    InvalidEventCursorError,
    decode_event_cursor,
    encode_event_cursor,
)
from scyg_agent.application.event_subscription import (
    HEARTBEAT_FRAME,
    HEARTBEAT_SECONDS,
    EventSubscriptionService,
    OpenedEventStream,
    encode_event_frame,
)
from scyg_agent.domain.ports.event_store import EventCursor, EventStore, StoredEvent
from scyg_agent.domain.runs import CommandId, EventId, RunId, RunSucceeded

RUN_ID = RunId("run_00000001")


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


def _raw(value: object) -> str:
    return base64.urlsafe_b64encode(json.dumps(value).encode()).rstrip(b"=").decode()


def test_cursor_round_trip_is_versioned_and_run_bound() -> None:
    cursor = encode_event_cursor(RUN_ID, EventCursor(19))
    assert len(cursor) <= 256
    assert decode_event_cursor(RUN_ID, cursor) == EventCursor(19)
    assert decode_event_cursor(RUN_ID, None) is None
    with pytest.raises(InvalidEventCursorError):
        _ = decode_event_cursor(RunId("run_00000002"), cursor)


@pytest.mark.parametrize(
    "raw",
    [
        "",
        "0",
        "-1",
        "x" * 257,
        "not+base64",
        "e30=",
        _raw({"v": 0, "runId": str(RUN_ID), "seq": 0}),
        _raw({"v": True, "runId": str(RUN_ID), "seq": 0}),
        _raw({"v": 1, "runId": str(RUN_ID), "seq": -1}),
        _raw({"v": 1, "runId": str(RUN_ID), "seq": True}),
        _raw({"v": 1, "runId": str(RUN_ID), "seq": 1.0}),
        _raw({"v": 1, "runId": str(RUN_ID), "seq": "1"}),
    ],
)
def test_invalid_cursor_is_rejected(raw: str) -> None:
    with pytest.raises(InvalidEventCursorError):
        _ = decode_event_cursor(RUN_ID, raw)


def _stored() -> StoredEvent:
    return StoredEvent(
        EventCursor(3),
        RunSucceeded(
            EventId("evt_00000001"),
            CommandId("cmd_00000001"),
            datetime(2026, 1, 1, tzinfo=UTC),
            RUN_ID,
            1,
        ),
    )


def test_event_frame_is_complete_and_has_opaque_cursor() -> None:
    stored = _stored()
    frame = encode_event_frame(stored)
    assert frame.endswith("\n\n")
    lines = frame.splitlines()
    assert decode_event_cursor(RUN_ID, lines[0].removeprefix("id: ")) == EventCursor(3)
    assert lines[1] == "event: run_succeeded"
    payload = cast("dict[str, object]", json.loads(lines[2].removeprefix("data: ")))
    assert payload["event_id"] == str(stored.event.event_id)
    assert payload["run_id"] == str(RUN_ID)


class FakeSubscription:
    def __init__(self) -> None:
        self.timeouts: list[float] = []
        self.closed: bool = False

    async def next_event(self, wait_seconds: float = 10.0) -> StoredEvent | None:
        self.timeouts.append(wait_seconds)
        if len(self.timeouts) == 1:
            return None
        if len(self.timeouts) == 2:
            return _stored()
        raise StopAsyncIteration

    async def aclose(self) -> None:
        self.closed = True


@pytest.mark.anyio
async def test_frames_emit_ten_second_heartbeat_and_terminal_eof() -> None:
    subscription = FakeSubscription()
    stream = OpenedEventStream(subscription)
    frames = [frame async for frame in stream.frames()]
    assert frames == [HEARTBEAT_FRAME, encode_event_frame(_stored())]
    assert subscription.timeouts == [HEARTBEAT_SECONDS] * 3
    assert HEARTBEAT_SECONDS == 10.0
    assert subscription.closed


@pytest.mark.anyio
async def test_service_awaits_open_and_can_close_before_frame_iteration() -> None:
    subscription = FakeSubscription()

    class Store:
        async def open_subscription(
            self, run_id: RunId, cursor: EventCursor | None
        ) -> FakeSubscription:
            assert run_id == RUN_ID
            assert cursor is None
            return subscription

    service = EventSubscriptionService(cast("EventStore", cast("object", Store())))
    stream = await service.open_events(RUN_ID, None)
    await stream.aclose()
    assert subscription.closed
