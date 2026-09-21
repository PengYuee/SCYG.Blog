"""Worker 续租、丢租和终态竞态测试."""

from collections.abc import AsyncIterator
from datetime import timedelta
from typing import final

import pytest

from scyg_agent.domain.ports.terminal_commit import (
    TerminalCancellationRequested,
    TerminalLeaseLost,
)
from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    CommandId,
    EventId,
    InteractionId,
    Run,
    RunCancelled,
    RunId,
    RunStatus,
    RuntimeKind,
)
from scyg_agent.domain.runs.repository import (
    CancellationRequested,
    LeaseLost,
    RenewResult,
)
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.deep.models import DeepFailureKind, DeepRuntimeError
from scyg_agent.runtimes.normalization import NormalizationContext
from scyg_agent.runtimes.outputs import RuntimeNativeOutput, SimpleRuntimeOutput
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRouter
from scyg_agent.runtimes.simple.results import CompletionFinished
from scyg_agent.worker import WorkerConfig, WorkerDependencies
from scyg_agent.worker.executor import execute_lease
from tests.runtimes.fakes import FakeRuntimeAdapter

from .test_executor import (
    NOW,
    BlockingAdapter,
    FailureAdapter,
    LoadedRepository,
    MissingRepository,
    RecordingCommitter,
    RenewingRepository,
    lease,
    router,
    running_run,
)


async def assert_renewal_control(renewal: RenewResult, expected_commits: int) -> None:
    """驱动一次续租控制结果并断言提交次数."""
    run = running_run()
    current_lease = lease()
    committer = RecordingCommitter(run)
    runtime_router = RuntimeRouter(
        default_registry(
            BlockingAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-block", RuntimeKind.DEEP)),
        )
    )
    await execute_lease(
        current_lease,
        WorkerDependencies(RenewingRepository(run, renewal), committer, lambda: NOW),
        runtime_router,
        WorkerConfig(lease_duration=timedelta(milliseconds=10), renewal_fraction=0.1),
    )
    assert len(committer.requests) == expected_commits


@pytest.mark.anyio
async def test_renewal_lease_loss_suppresses_terminal_commit() -> None:
    """Given 丢租, When 续租观察结果, Then 终止运行时且不提交."""
    await assert_renewal_control(LeaseLost(RunId("run_12345678")), 0)


@pytest.mark.anyio
async def test_renewal_cancellation_commits_cancelled_terminal() -> None:
    """Given 取消, When 续租观察结果, Then 终止运行时并提交取消."""
    current_lease = lease()
    await assert_renewal_control(
        CancellationRequested(current_lease.run_id, NOW, current_lease, replayed=False), 1
    )


@pytest.mark.anyio
async def test_missing_run_stops_before_route_or_commit() -> None:
    """Given 租约目标不存在, When 执行, Then 无副作用返回."""
    run = running_run()
    committer = RecordingCommitter(run)
    await execute_lease(
        lease(),
        WorkerDependencies(MissingRepository(), committer, lambda: NOW),
        router(),
        WorkerConfig(),
    )
    assert committer.requests == []


@pytest.mark.anyio
async def test_failed_native_output_maps_failed_and_lease_loss_is_suppressed() -> None:
    """Given 原生失败且提交时丢租, When 执行, Then 只尝试一个 FAILED 事务."""
    run = running_run()
    committer = RecordingCommitter(run, TerminalLeaseLost(run.id))
    runtime_router = RuntimeRouter(
        default_registry(
            FailureAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-failure", RuntimeKind.DEEP)),
        )
    )
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        runtime_router,
        WorkerConfig(),
    )
    assert committer.requests[0].completion.status is RunStatus.FAILED


@pytest.mark.anyio
async def test_normalized_cancelled_output_maps_cancelled_status(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Given 规范化取消终态, When 执行, Then 原子提交 CANCELLED."""
    run = running_run()
    committer = RecordingCommitter(run)

    def cancelled_events(
        normalized_run: Run,
        context: NormalizationContext,
        outputs: tuple[RuntimeNativeOutput, ...],
    ) -> tuple[RunCancelled, ...]:
        _ = outputs
        return (
            RunCancelled(
                EventId("evt_cancelnorm01"),
                CommandId("cmd_cancelnorm01"),
                context.occurred_at,
                normalized_run.id,
                normalized_run.revision,
            ),
        )

    monkeypatch.setattr("scyg_agent.worker.executor.normalize_runtime", cancelled_events)
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        router(),
        WorkerConfig(),
    )
    assert committer.requests[0].completion.status is RunStatus.CANCELLED


@pytest.mark.anyio
async def test_approval_required_commits_waiting_input_with_interaction(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Given DEEP 审批中断, When 规范化, Then 原子提交 WAITING_INPUT."""
    run = running_run()
    committer = RecordingCommitter(run)
    interaction_id = InteractionId("int_waiting001")

    def approval_events(
        normalized_run: Run,
        context: NormalizationContext,
        outputs: tuple[RuntimeNativeOutput, ...],
    ) -> tuple[ApprovalRequiredEvent, ...]:
        _ = outputs
        return (
            ApprovalRequiredEvent(
                EventId("evt_waiting0001"),
                CommandId("cmd_waiting0001"),
                context.occurred_at,
                normalized_run.id,
                normalized_run.revision,
                interaction_id,
            ),
        )

    monkeypatch.setattr("scyg_agent.worker.executor.normalize_runtime", approval_events)
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        router(),
        WorkerConfig(),
    )

    completion = committer.requests[0].completion
    assert completion.status is RunStatus.WAITING_INPUT
    assert completion.pending_interaction_id == interaction_id


@final
class ErrorAdapter:
    """在原生输出前抛出封闭 Deep 运行时错误."""

    identity = AdapterIdentity("error-worker", RuntimeKind.SIMPLE)

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        if run.id == RunId("run_12345678"):
            raise DeepRuntimeError(DeepFailureKind.INVALID_INPUT)
        yield SimpleRuntimeOutput(CompletionFinished("stop"))

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        return self.execute(run)


@final
class TimeoutAdapter:
    """模拟 Deep fail_after 抛出的内置超时."""

    identity = AdapterIdentity("timeout-worker", RuntimeKind.SIMPLE)

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        if run.id == RunId("run_12345678"):
            raise TimeoutError
        yield SimpleRuntimeOutput(CompletionFinished("stop"))

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        return self.execute(run)


@pytest.mark.anyio
async def test_runtime_error_isolated_as_failed_terminal() -> None:
    """Given 单 Run 运行时错误, When 执行, Then 收敛 FAILED 而不传播."""
    run = running_run()
    committer = RecordingCommitter(run)
    runtime_router = RuntimeRouter(
        default_registry(
            ErrorAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-error", RuntimeKind.DEEP)),
        )
    )

    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        runtime_router,
        WorkerConfig(),
    )

    assert committer.requests[0].completion.status is RunStatus.FAILED


@pytest.mark.anyio
async def test_runtime_timeout_isolated_as_failed_terminal() -> None:
    """Given Deep 超时, When 执行, Then 不击穿 Worker 并提交 FAILED."""
    run = running_run()
    committer = RecordingCommitter(run)
    runtime_router = RuntimeRouter(
        default_registry(
            TimeoutAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-timeout", RuntimeKind.DEEP)),
        )
    )
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        runtime_router,
        WorkerConfig(),
    )
    assert committer.requests[0].completion.status is RunStatus.FAILED


@pytest.mark.anyio
async def test_runtime_failure_honors_concurrent_cancellation() -> None:
    """Given 失败提交观察取消, When 收敛, Then 重试唯一 CANCELLED."""
    run = running_run()
    committer = RecordingCommitter(run, TerminalCancellationRequested(run.id))
    runtime_router = RuntimeRouter(
        default_registry(
            ErrorAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-error-cancel", RuntimeKind.DEEP)),
        )
    )
    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        runtime_router,
        WorkerConfig(),
    )
    assert [request.completion.status for request in committer.requests] == [
        RunStatus.FAILED,
        RunStatus.CANCELLED,
    ]
