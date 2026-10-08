"""Pure event-store cursor and terminal projection contracts."""

from datetime import UTC, datetime

import pytest

from scyg_agent.domain.ports.event_store import (
    AppendRequest,
    EventCursor,
    InvalidAppendRequestError,
    InvalidEventCursorError,
    InvalidReplayLimitError,
    StoredEvent,
    TerminalSnapshot,
    project_terminal_snapshot,
    validate_replay_limit,
)
from scyg_agent.domain.runs import CommandId, EventId, RunId, RunStatus, RunSucceeded


def test_terminal_snapshot_is_derived_from_durable_terminal_event() -> None:
    # Given: one durable terminal event at a stable sequence.
    event = RunSucceeded(
        EventId("evt_00000001"),
        CommandId("cmd_00000001"),
        datetime(2026, 7, 12, tzinfo=UTC),
        RunId("run_00000001"),
        3,
    )
    stored = StoredEvent(EventCursor(7), event)

    # When: the pure projection derives a terminal snapshot.
    snapshot = project_terminal_snapshot((stored,))

    # Then: the snapshot retains durable identity, cursor, status, and revision.
    assert snapshot == TerminalSnapshot(
        RunId("run_00000001"), EventCursor(7), EventId("evt_00000001"), RunStatus.SUCCEEDED, 3
    )


def test_cursor_and_append_value_objects_reject_invalid_domains() -> None:
    # Given: a negative cursor and an empty append batch.
    run_id = RunId("run_00000001")

    # When/Then: construction rejects each invalid domain with typed diagnostics.
    with pytest.raises(InvalidEventCursorError, match="nonnegative"):
        _ = EventCursor(-1)
    with pytest.raises(InvalidAppendRequestError, match="exactly one run"):
        _ = AppendRequest(run_id, ())


def test_terminal_projection_returns_none_without_terminal_event() -> None:
    # Given/When/Then: an empty durable journal has no terminal snapshot.
    assert project_terminal_snapshot(()) is None


def test_append_request_rejects_duplicate_event_ids_before_database_io() -> None:
    # Given: two events in one batch reuse the same stable identity.
    event = RunSucceeded(
        EventId("evt_00000001"),
        CommandId("cmd_00000001"),
        datetime(2026, 7, 12, tzinfo=UTC),
        RunId("run_00000001"),
        1,
    )

    # When/Then: construction rejects the duplicate with a value-free typed boundary error.
    with pytest.raises(InvalidAppendRequestError) as captured:
        _ = AppendRequest(event.run_id, (event, event))
    assert str(captured.value) == "append batch event identities must be unique"


@pytest.mark.parametrize("limit", [0, -1, 1001])
def test_replay_limit_rejects_nonpositive_and_excessive_values(limit: int) -> None:
    # Given/When/Then: invalid replay limits fail before an adapter can acquire a session.
    with pytest.raises(InvalidReplayLimitError, match="between 1 and 1000"):
        _ = validate_replay_limit(limit)
