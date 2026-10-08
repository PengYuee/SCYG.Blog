"""Lease admission and renewal never overwrite a cancellation or another owner."""

from datetime import timedelta
from typing import override

import anyio
import pytest

from scyg_agent.agents.runner import AgentRunOutcome
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.runs import Run, RunStatus
from scyg_agent.domain.runs.repository import (
    CancellationRequested,
    LeaseLost,
    Renewed,
    RenewRequest,
    RenewResult,
    RunLease,
)
from scyg_agent.worker import WorkerConfig, WorkerDependencies
from scyg_agent.worker.executor import execute_lease

from .test_executor import (
    NOW,
    LoadedRepository,
    RecordingCommitter,
    RecordingStreamStore,
    StructuredRunner,
    lease,
    running_run,
)


class RenewingRepository(LoadedRepository):
    def __init__(self, run: Run, result: RenewResult, *, reject_admission: bool = False) -> None:
        super().__init__(run)
        self.result: RenewResult = result
        self.reject_admission: bool = reject_admission

    @override
    async def renew(self, request: RenewRequest) -> RenewResult:
        self.renewals.append(request)
        if self.reject_admission or len(self.renewals) > 1:
            return self.result
        return Renewed(lease())


class BlockingRunner:
    def __init__(self) -> None:
        self.started: anyio.Event = anyio.Event()
        self.drained: anyio.Event = anyio.Event()

    async def execute(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        assert run.id == lease.run_id
        self.started.set()
        try:
            await anyio.sleep_forever()
        finally:
            self.drained.set()
        message = "Blocking runner must be cancelled"
        raise AssertionError(message)

    async def resume(
        self,
        run: Run,
        command: CommandSubmission,
        lease: RunLease,
    ) -> AgentRunOutcome:
        assert command.run_id == run.id
        return await self.execute(run, lease)


@pytest.mark.anyio
@pytest.mark.parametrize("cancelled", [False, True])
async def test_renewal_cancels_and_drains_runner_without_terminal_override(
    *,
    cancelled: bool,
) -> None:
    run = running_run()
    result = (
        CancellationRequested(
            run.id,
            NOW,
            None,
            replayed=False,
        )
        if cancelled
        else LeaseLost(run.id)
    )
    repository = RenewingRepository(run, result)
    runner = BlockingRunner()
    committer = RecordingCommitter(run)
    store = RecordingStreamStore()
    await execute_lease(
        lease(),
        WorkerDependencies(
            repository,
            committer,
            lambda: NOW,
            runner,
            store.port(),
        ),
        WorkerConfig(
            lease_duration=timedelta(milliseconds=10),
            renewal_fraction=0.1,
        ),
    )
    assert runner.started.is_set()
    assert runner.drained.is_set()
    assert len(repository.renewals) == 2
    assert committer.requests == []
    assert store.envelopes == []


@pytest.mark.anyio
@pytest.mark.parametrize("cancelled", [False, True])
async def test_authoritative_admission_rejects_revoked_token_before_runner(
    *,
    cancelled: bool,
) -> None:
    run = running_run()
    result = (
        CancellationRequested(
            run.id,
            NOW,
            None,
            replayed=False,
        )
        if cancelled
        else LeaseLost(run.id)
    )
    repository = RenewingRepository(run, result, reject_admission=True)
    runner = StructuredRunner()
    committer = RecordingCommitter(run)
    store = RecordingStreamStore()
    await execute_lease(
        lease(),
        WorkerDependencies(repository, committer, lambda: NOW, runner, store.port()),
        WorkerConfig(),
    )
    assert runner.calls == 0
    assert committer.requests == []
    assert store.activations == []
    assert store.envelopes == []


class ErrorRunner(StructuredRunner):
    @override
    async def execute(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        assert run.id == lease.run_id
        message = "Third-party execution failed"
        raise RuntimeError(message)


@pytest.mark.anyio
async def test_runner_error_isolated_as_sanitized_failed_terminal() -> None:
    run = running_run()
    committer = RecordingCommitter(run)
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW, ErrorRunner()),
        WorkerConfig(),
    )
    request = committer.requests[0]
    assert request.completion.status is RunStatus.FAILED
    assert request.error_message == "AgentRunner 执行失败"
    assert "Third-party" not in request.error_message
