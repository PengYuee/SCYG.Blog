"""T18 事件 codec 版本和严格字段回归测试。"""

from datetime import UTC, datetime

import pytest

from scyg_agent.adapters.database.event_codec import (
    MalformedEventRecordError,
    deserialize_event,
    serialize_event,
)
from scyg_agent.adapters.database.journal_records import EventRecord
from scyg_agent.domain.runs import CommandId, EventId, RunId, RunSucceeded

NOW = datetime(2026, 7, 12, 10, 0, tzinfo=UTC)


def _record(*, kind: str = "run_succeeded", payload: dict[str, str]) -> EventRecord:
    return EventRecord(
        event_id="evt_codec0001",
        run_id="run_codec0001",
        seq=1,
        revision=1,
        kind=kind,
        occurred_at=NOW,
        payload=payload,
    )


def test_codec_emits_version_and_round_trips_v1() -> None:
    event = RunSucceeded(
        EventId("evt_codec0001"),
        CommandId("cmd_codec0001"),
        NOW,
        RunId("run_codec0001"),
        1,
    )

    kind, payload = serialize_event(event)
    decoded = deserialize_event(_record(kind=kind.value, payload=payload))

    assert payload["schema_version"] == "1"
    assert decoded == event


def test_codec_accepts_existing_versionless_v1_row() -> None:
    decoded = deserialize_event(_record(payload={"command_id": "cmd_codec0001"}))

    assert type(decoded) is RunSucceeded


@pytest.mark.parametrize(
    ("kind", "payload"),
    [
        ("run_succeeded", {"command_id": "cmd_codec0001", "content": "illegal"}),
        ("run_succeeded", {"schema_version": "2", "command_id": "cmd_codec0001"}),
        ("unknown_kind", {"schema_version": "1", "command_id": "cmd_codec0001"}),
        ("run_succeeded", {"schema_version": "1"}),
    ],
)
def test_codec_rejects_extra_unknown_version_kind_and_missing_fields(
    kind: str, payload: dict[str, str]
) -> None:
    with pytest.raises(MalformedEventRecordError):
        _ = deserialize_event(_record(kind=kind, payload=payload))
