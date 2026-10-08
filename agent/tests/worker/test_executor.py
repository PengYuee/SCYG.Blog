"""Recipe Worker admission, fenced commits, and post-commit transient events."""

from dataclasses import replace
from datetime import UTC, datetime, timedelta
from typing import cast, override
from uuid import UUID

import pytest

from scyg_agent.adapters.redis import (
    RedisStreamId,
    RedisStreamKind,
    RedisStreamStore,
    RedisUnavailableError,
    StreamEnvelope,
)
from scyg_agent.agents import AgentFailure, ApprovalRequest, ArticleDraft, Capability
from scyg_agent.agents import FailureKind as AgentFailureKind
from scyg_agent.agents.runner import (
    AgentFailed,
    AgentRunOutcome,
    AgentSucceeded,
    AgentWaitingForApproval,
)
from scyg_agent.domain.ports.audit_store import StoredAuditFact
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.event_store import EventCursor, StoredEvent
from scyg_agent.domain.ports.terminal_commit import (
    TerminalCancellationRequested,
    TerminalCommitRequest,
    TerminalCommitResult,
    TerminalCommitted,
    TerminalLeaseLost,
    TerminalReplay,
    TerminalStateConflict,
)
from scyg_agent.domain.runs import (
    ExecutionOwnerId,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.repository import (
    ClaimRequest,
    GetResult,
    NotFound,
    Renewed,
    RenewRequest,
    RenewResult,
    RunLease,
)
from scyg_agent.domain.runs.repository_values import LeaseToken
from scyg_agent.worker import WorkerConfig, WorkerDependencies
from scyg_agent.worker.executor import execute_lease

NOW = datetime(2026, 7, 12, 19, tzinfo=UTC)
OWNER = ExecutionOwnerId("worker_executor01")
RUN_ID = RunId("run_12345678")


def running_run() -> Run:
    return Run(
        RUN_ID,
        UserId("user_worker"),
        TaskType.COMPOSE,
        RuntimeSelection(
            RuntimeKind.DEEP,
            "v1",
        ),
        2,
        RunStatus.RUNNING,
        NOW,
        NOW,
        1,
        OWNER,
        None,
    )


def lease(*, cancelled: bool = False) -> RunLease:
    return RunLease(
        RUN_ID,
        OWNER,
        LeaseToken(UUID(int=1)),
        2,
        1,
        NOW + timedelta(minutes=1),
        NOW if cancelled else None,
    )


class LoadedRepository:
    def __init__(self, run: Run) -> None:
        self.run: Run = run
        self.renewals: list[RenewRequest] = []

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        raise AssertionError(request)

    async def get(self, run_id: RunId) -> GetResult:
        assert run_id == self.run.id
        return self.run

    async def renew(self, request: RenewRequest) -> RenewResult:
        self.renewals.append(request)
        return Renewed(replace(lease(), expires_at=request.guard.now + request.lease_duration))


class MissingRepository(LoadedRepository):
    @override
    async def get(self, run_id: RunId) -> GetResult:
        return NotFound(run_id)


class RecordingCommitter:
    def __init__(
        self,
        run: Run,
        first_result: TerminalCommitResult | None = None,
        trace: list[str] | None = None,
    ) -> None:
        self.run: Run = run
        self.first_result: TerminalCommitResult | None = first_result
        self.requests: list[TerminalCommitRequest] = []
        self.trace: list[str] = trace if trace is not None else []

    async def commit(self, request: TerminalCommitRequest) -> TerminalCommitResult:
        self.requests.append(request)
        self.trace.append("commit")
        if self.first_result is not None:
            return self.first_result
        completed = replace(
            self.run,
            status=request.completion.status,
            revision=request.completion.guard.expected_revision + 1,
            execution_owner=None,
            pending_interaction_id=request.completion.pending_interaction_id,
        )
        return TerminalCommitted(
            completed,
            tuple(
                StoredEvent(EventCursor(index + 1), event)
                for (
                    index,
                    event,
                ) in enumerate(request.events.events)
            ),
            StoredAuditFact(
                1,
                request.audit,
            ),
        )


class StructuredRunner:
    def __init__(self, outcome: AgentRunOutcome | None = None) -> None:
        self.calls: int = 0
        self.outcome: AgentRunOutcome = (
            outcome
            if outcome is not None
            else AgentSucceeded(
                Capability.WRITE, ArticleDraft(title="标题", outline="大纲", markdown="# 正文")
            )
        )

    async def execute(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        assert run.id == lease.run_id
        self.calls += 1
        return self.outcome

    async def resume(
        self,
        run: Run,
        command: CommandSubmission,
        lease: RunLease,
    ) -> AgentRunOutcome:
        assert command.run_id == run.id
        return await self.execute(run, lease)


class RecordingStreamStore:
    def __init__(
        self,
        *,
        fail_append: bool = False,
        fail_activate: bool = False,
        trace: list[str] | None = None,
    ) -> None:
        self.fail_append: bool = fail_append
        self.fail_activate: bool = fail_activate
        self.activations: list[tuple[RunId, int]] = []
        self.envelopes: list[StreamEnvelope] = []
        self.trace: list[str] = trace if trace is not None else []

    async def activate_attempt(self, run_id: RunId, attempt: int) -> None:
        if self.fail_activate:
            raise RedisUnavailableError
        self.activations.append((run_id, attempt))

    async def append(self, envelope: StreamEnvelope) -> RedisStreamId:
        if self.fail_append:
            raise RedisUnavailableError
        self.trace.append("publish")
        self.envelopes.append(envelope)
        return RedisStreamId(f"{len(self.envelopes)}-0")

    def port(self) -> RedisStreamStore:
        return cast("RedisStreamStore", cast("object", self))


@pytest.mark.anyio
async def test_agent_result_commits_before_transient_terminal() -> None:
    run = running_run()
    trace: list[str] = []
    committer = RecordingCommitter(run, trace=trace)
    store = RecordingStreamStore(trace=trace)
    repository = LoadedRepository(run)
    runner = StructuredRunner()
    await execute_lease(
        lease(),
        WorkerDependencies(repository, committer, lambda: NOW, runner, store.port()),
        WorkerConfig(),
    )
    request = committer.requests[0]
    assert request.completion.status is RunStatus.SUCCEEDED
    assert request.events.events[-1].revision == 3
    assert request.result is not None
    assert request.result.capability == "write"
    result = ArticleDraft.model_validate(request.result.payload)
    assert result.markdown == "# 正文"
    assert trace == ["commit", "publish"]
    assert len(repository.renewals) == 1
    assert runner.calls == 1
    assert store.envelopes[0].kind is RedisStreamKind.TERMINAL


@pytest.mark.anyio
async def test_failure_fields_are_attached_to_atomic_commit() -> None:
    run = running_run()
    runner = StructuredRunner(
        AgentFailed(
            AgentFailure(
                kind=AgentFailureKind.DEPENDENCY, message="Blog 依赖不可用", retryable=True
            ),
        )
    )
    committer = RecordingCommitter(run)
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW, runner),
        WorkerConfig(),
    )
    request = committer.requests[0]
    assert request.completion.status is RunStatus.FAILED
    assert request.error_code == AgentFailureKind.DEPENDENCY.value
    assert request.error_message == "Blog 依赖不可用"
    assert request.result is None


@pytest.mark.anyio
async def test_approval_request_kind_and_payload_commit_atomically() -> None:
    run = running_run()
    approval = ApprovalRequest(interaction_id="int_waiting001", title="标题", outline="大纲")
    runner = StructuredRunner(AgentWaitingForApproval(approval))
    committer = RecordingCommitter(run)
    store = RecordingStreamStore()
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW, runner, store.port()),
        WorkerConfig(),
    )
    request = committer.requests[0]
    assert request.completion.status is RunStatus.WAITING_INPUT
    assert str(request.completion.pending_interaction_id) == approval.interaction_id
    assert request.interaction_payload == approval.model_dump(mode="json")
    assert request.interaction_kind == approval.kind
    assert store.envelopes[0].kind is RedisStreamKind.APPROVAL_REQUIRED


@pytest.mark.anyio
@pytest.mark.parametrize(
    "lost",
    ["cancelled", "expired", "revision", "owner", "attempt", "missing"],
)
async def test_invalid_admission_never_runs_or_publishes(lost: str) -> None:
    run = running_run()
    current = lease(cancelled=lost == "cancelled")
    if lost == "expired":
        current = replace(current, expires_at=NOW)
    elif lost == "revision":
        run = replace(run, revision=3)
    elif lost == "owner":
        run = replace(run, execution_owner=ExecutionOwnerId("worker_other0001"))
    elif lost == "attempt":
        run = replace(run, attempt=2)
    repository = MissingRepository(run) if lost == "missing" else LoadedRepository(run)
    runner = StructuredRunner()
    committer = RecordingCommitter(run)
    store = RecordingStreamStore()
    await execute_lease(
        current,
        WorkerDependencies(repository, committer, lambda: NOW, runner, store.port()),
        WorkerConfig(),
    )
    assert runner.calls == 0
    assert committer.requests == []
    assert repository.renewals == []
    assert store.activations == []


@pytest.mark.anyio
@pytest.mark.parametrize("rejected", ["lost", "conflict", "cancelled", "replay"])
async def test_rejected_commit_never_publishes_or_overwrites_cancelled(rejected: str) -> None:
    run = running_run()
    results: dict[str, TerminalCommitResult] = {
        "lost": TerminalLeaseLost(run.id),
        "conflict": TerminalStateConflict(run.id),
        "cancelled": TerminalCancellationRequested(run.id),
        "replay": TerminalReplay(replace(run, status=RunStatus.CANCELLED, execution_owner=None)),
    }
    committer = RecordingCommitter(run, results[rejected])
    store = RecordingStreamStore()
    await execute_lease(
        lease(),
        WorkerDependencies(
            LoadedRepository(run),
            committer,
            lambda: NOW,
            StructuredRunner(),
            store.port(),
        ),
        WorkerConfig(),
    )
    assert len(committer.requests) == 1
    assert committer.requests[0].completion.status is RunStatus.SUCCEEDED
    assert store.envelopes == []


@pytest.mark.anyio
async def test_redis_failure_after_commit_cannot_rewrite_success() -> None:
    run = running_run()
    committer = RecordingCommitter(run)
    store = RecordingStreamStore(fail_append=True)
    await execute_lease(
        lease(),
        WorkerDependencies(
            LoadedRepository(run),
            committer,
            lambda: NOW,
            StructuredRunner(),
            store.port(),
        ),
        WorkerConfig(),
    )
    assert len(committer.requests) == 1
    assert committer.requests[0].completion.status is RunStatus.SUCCEEDED
    assert store.envelopes == []


@pytest.mark.anyio
async def test_redis_activation_failure_commits_failure_before_runner() -> None:
    run = running_run()
    committer = RecordingCommitter(run)
    runner = StructuredRunner()
    store = RecordingStreamStore(fail_activate=True)
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW, runner, store.port()),
        WorkerConfig(),
    )
    assert runner.calls == 0
    assert committer.requests[0].completion.status is RunStatus.FAILED
    assert committer.requests[0].error_code == "redis_failure"
