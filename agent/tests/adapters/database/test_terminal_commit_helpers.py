"""No-endpoint tests for fenced terminal transaction branches."""

from dataclasses import replace
from datetime import UTC, datetime, timedelta
from uuid import UUID

import pytest
from sqlalchemy.ext.asyncio import async_sessionmaker

from scyg_agent.adapters.database.event_codec import serialize_event
from scyg_agent.adapters.database.journal_records import EventRecord
from scyg_agent.adapters.database.run_records import AgentRunResultRecord, RunRecord
from scyg_agent.adapters.database.terminal_commit import (
    PostgreSQLTerminalCommitter,
    TerminalStage,
)
from scyg_agent.domain.ports.audit_store import AuditFact
from scyg_agent.domain.ports.event_store import AppendRequest
from scyg_agent.domain.ports.idempotency import AuditMetadata
from scyg_agent.domain.ports.terminal_commit import (
    InvalidTerminalCommitRequestError,
    TerminalCancellationRequested,
    TerminalCommitRequest,
    TerminalCommitted,
    TerminalEventConflict,
    TerminalLeaseLost,
    TerminalReplay,
    TerminalResult,
    TerminalStateConflict,
)
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    ExecutionOwnerId,
    RunCancelled,
    RunFailed,
    RunId,
    RunStatus,
    RunSucceeded,
    UserId,
)
from scyg_agent.domain.runs.repository import CompletionRequest, LeaseGuard, LeaseToken
from tests.adapters.database.scripted_session_support import (
    FakeResult,
    ScriptedSession,
    session,
)

NOW = datetime(2026, 7, 12, 14, tzinfo=UTC)
RUN_ID = RunId("run_terminal01")
COMMAND_ID = CommandId("cmd_terminal01")
TOKEN = LeaseToken(UUID("12345678-1234-5678-9234-567812345678"))
OWNER = ExecutionOwnerId("worker_terminal01")


class RecordingFailpoint:
    """Record every completed transactional write stage."""

    def __init__(self) -> None:
        self.stages: list[TerminalStage] = []

    async def reach(self, stage: TerminalStage) -> None:
        self.stages.append(stage)


def _record(
    status: RunStatus = RunStatus.RUNNING,
    *,
    cancellation_requested: bool = False,
) -> RunRecord:
    running = status is RunStatus.RUNNING
    return RunRecord(
        run_id=str(RUN_ID),
        owner_user_id="user-terminal",
        operation_id="op:terminal",
        task_type="summary",
        runtime_kind="simple",
        runtime_version="v1",
        revision=2,
        status=status.value,
        created_at=NOW - timedelta(minutes=1),
        updated_at=NOW - timedelta(minutes=1),
        attempt=1,
        next_attempt_at=None,
        lease_owner=str(OWNER) if running else None,
        lease_token=TOKEN.value if running else None,
        lease_expires_at=NOW + timedelta(minutes=5) if running else None,
        cancellation_requested_at=NOW if cancellation_requested else None,
        pending_interaction_id=None,
        terminal_at=NOW
        if status in {RunStatus.SUCCEEDED, RunStatus.FAILED, RunStatus.CANCELLED}
        else None,
        terminal_metadata=None,
        error_code=None,
        error_message=None,
        error_metadata=None,
    )


def _request(status: RunStatus = RunStatus.SUCCEEDED) -> TerminalCommitRequest:
    event = RunSucceeded(EventId("evt_terminal01"), COMMAND_ID, NOW, RUN_ID, 3)
    guard = LeaseGuard(RUN_ID, OWNER, TOKEN, 2, NOW)
    audit = AuditFact(
        "audit-terminal-1",
        RUN_ID,
        UserId("user-terminal"),
        COMMAND_ID,
        None,
        "terminal_commit",
        status.value,
        NOW,
        AuditMetadata("source", "worker"),
    )
    return TerminalCommitRequest(
        CompletionRequest(guard, status), AppendRequest(RUN_ID, (event,)), audit
    )


def _result_request() -> TerminalCommitRequest:
    request = _request()
    return replace(
        request,
        result=TerminalResult("v1", "write", {"markdown": "# 正文"}, "a" * 64),
    )


def _cancel_request() -> TerminalCommitRequest:
    event = RunCancelled(EventId("evt_terminal02"), COMMAND_ID, NOW, RUN_ID, 3)
    guard = LeaseGuard(RUN_ID, OWNER, TOKEN, 2, NOW)
    audit = AuditFact(
        "audit-terminal-2",
        RUN_ID,
        UserId("user-terminal"),
        COMMAND_ID,
        None,
        "terminal_commit",
        "cancelled",
        NOW,
        AuditMetadata("source", "worker"),
    )
    return TerminalCommitRequest(
        CompletionRequest(guard, RunStatus.CANCELLED),
        AppendRequest(RUN_ID, (event,)),
        audit,
    )


def _event_record() -> EventRecord:
    event = _request().events.events[0]
    kind, payload = serialize_event(event)
    return EventRecord(
        event_id=str(event.event_id),
        run_id=str(RUN_ID),
        seq=1,
        revision=event.revision,
        kind=kind.value,
        occurred_at=event.occurred_at,
        payload=payload,
    )


@pytest.mark.anyio
async def test_terminal_commit_writes_all_stages_in_one_session(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    row = _record()
    script = ScriptedSession(
        [
            FakeResult(),
            FakeResult(row),
            FakeResult(rows=()),
            FakeResult(0),
            FakeResult(),
            FakeResult(0),
        ]
    )
    failpoint = RecordingFailpoint()
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events", failpoint)

    result = await committer.commit_in_session(session(monkeypatch, script), _request())

    assert isinstance(result, TerminalCommitted)
    assert result.run.status is RunStatus.SUCCEEDED
    assert failpoint.stages == list(TerminalStage)
    assert script.flushes == 3
    assert row.lease_token is None


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("row", "commit_request", "expected"),
    [
        (_record(), _request(), TerminalLeaseLost),
        (_record(cancellation_requested=True), _request(), TerminalCancellationRequested),
    ],
)
async def test_terminal_commit_reports_fence_or_cancellation_before_writes(
    monkeypatch: pytest.MonkeyPatch,
    row: RunRecord,
    commit_request: TerminalCommitRequest,
    expected: type[TerminalLeaseLost] | type[TerminalCancellationRequested],
) -> None:
    if expected is TerminalLeaseLost:
        row.lease_token = UUID("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
    script = ScriptedSession([FakeResult(), FakeResult(row)])
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events")

    result = await committer.commit_in_session(session(monkeypatch, script), commit_request)

    assert isinstance(result, expected)
    assert script.added == []
    assert script.flushes == 0


@pytest.mark.anyio
async def test_terminal_commit_adds_structured_result_before_run_update(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    row = _record()
    script = ScriptedSession(
        [
            FakeResult(),
            FakeResult(row),
            FakeResult(rows=()),
            FakeResult(0),
            FakeResult(),
            FakeResult(0),
            FakeResult(),
        ]
    )
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events")

    result = await committer.commit_in_session(session(monkeypatch, script), _result_request())
    assert isinstance(result, TerminalCommitted)

    stored = script.added[-1]
    assert isinstance(stored, AgentRunResultRecord)
    assert stored.capability == "write"
    assert stored.result_payload == {"markdown": "# 正文"}
    assert row.status == RunStatus.SUCCEEDED.value


@pytest.mark.anyio
async def test_requested_cancellation_commits_exact_cancelled_terminal(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    row = _record(cancellation_requested=True)
    script = ScriptedSession(
        [
            FakeResult(),
            FakeResult(row),
            FakeResult(rows=()),
            FakeResult(0),
            FakeResult(),
            FakeResult(0),
        ]
    )
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events")

    result = await committer.commit_in_session(session(monkeypatch, script), _cancel_request())

    assert isinstance(result, TerminalCommitted)
    assert result.run.status is RunStatus.CANCELLED
    assert row.cancellation_requested_at is None


@pytest.mark.anyio
async def test_terminal_commit_replays_identical_terminal(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    row = _record(RunStatus.SUCCEEDED)
    script = ScriptedSession([FakeResult(), FakeResult(row), FakeResult(rows=(_event_record(),))])
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events")

    result = await committer.commit_in_session(session(monkeypatch, script), _request())

    assert isinstance(result, TerminalReplay)
    assert script.added == []


@pytest.mark.anyio
async def test_terminal_commit_rejects_different_existing_terminal(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    row = _record(RunStatus.FAILED)
    script = ScriptedSession([FakeResult(), FakeResult(row)])
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events")

    result = await committer.commit_in_session(session(monkeypatch, script), _request())

    assert isinstance(result, TerminalStateConflict)


def test_terminal_request_rejects_cross_run_revision_and_event_kind() -> None:
    request = _request()
    with pytest.raises(InvalidTerminalCommitRequestError):
        _ = TerminalCommitRequest(
            request.completion,
            request.events,
            replace(request.audit, run_id=RunId("run_terminal99")),
        )
    wrong_revision = replace(request.events.events[0], revision=2)
    with pytest.raises(InvalidTerminalCommitRequestError):
        _ = TerminalCommitRequest(
            request.completion, AppendRequest(RUN_ID, (wrong_revision,)), request.audit
        )
    wrong_kind = RunFailed(EventId("evt_terminal03"), COMMAND_ID, NOW, RUN_ID, 3)
    with pytest.raises(InvalidTerminalCommitRequestError):
        _ = TerminalCommitRequest(
            request.completion, AppendRequest(RUN_ID, (wrong_kind,)), request.audit
        )


@pytest.mark.anyio
async def test_terminal_commit_contains_missing_corrupt_and_replay_conflicts(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events")
    missing = await committer.commit_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(), FakeResult(None)])), _request()
    )
    corrupt_row = _record()
    corrupt_row.status = "future"
    corrupt = await committer.commit_in_session(
        session(monkeypatch, ScriptedSession([FakeResult(), FakeResult(corrupt_row)])),
        _request(),
    )
    replay_missing = await committer.commit_in_session(
        session(
            monkeypatch,
            ScriptedSession(
                [FakeResult(), FakeResult(_record(RunStatus.SUCCEEDED)), FakeResult(rows=())]
            ),
        ),
        _request(),
    )

    assert isinstance(missing, TerminalLeaseLost)
    assert isinstance(corrupt, TerminalStateConflict)
    assert isinstance(replay_missing, TerminalStateConflict)


@pytest.mark.anyio
async def test_terminal_replay_detects_immutable_event_conflict(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    event_record = _event_record()
    event_record.kind = "run_failed"
    script = ScriptedSession(
        [FakeResult(), FakeResult(_record(RunStatus.SUCCEEDED)), FakeResult(rows=(event_record,))]
    )
    committer = PostgreSQLTerminalCommitter(async_sessionmaker(), "agent_events")

    result = await committer.commit_in_session(session(monkeypatch, script), _request())

    assert isinstance(result, TerminalEventConflict)
