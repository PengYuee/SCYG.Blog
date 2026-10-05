"""Worker 单租约执行和围栏提交测试."""

from collections.abc import AsyncIterator
from dataclasses import replace
from datetime import UTC, datetime, timedelta
from typing import cast, final
from uuid import UUID

import anyio
import pytest

from scyg_agent.adapters.redis import (
    RedisStreamId,
    RedisStreamKind,
    RedisStreamStore,
    RedisUnavailableError,
    StreamEnvelope,
)
from scyg_agent.agents import (
    AgentFailure,
    ArticleDraft,
    Capability,
)
from scyg_agent.agents import (
    FailureKind as AgentFailureKind,
)
from scyg_agent.agents.runner import AgentFailed, AgentRunOutcome, AgentSucceeded
from scyg_agent.domain.ports.terminal_commit import (
    TerminalCancellationRequested,
    TerminalCommitRequest,
    TerminalCommitResult,
    TerminalReplay,
)
from scyg_agent.domain.runs import (
    ExecutionOwnerId,
    Run,
    RunCancelled,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
)
from scyg_agent.domain.runs.repository import (
    ClaimRequest,
    GetResult,
    NotFound,
    RenewRequest,
    RenewResult,
    RunLease,
)
from scyg_agent.domain.runs.repository_values import LeaseToken
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.outputs import RuntimeNativeOutput, SimpleRuntimeOutput
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRouter
from scyg_agent.runtimes.simple.results import (
    CompletionFinished,
    FailureKind,
    ProviderDelta,
    ProviderFailure,
)
from scyg_agent.worker import WorkerConfig, WorkerDependencies
from scyg_agent.worker.executor import execute_lease
from tests.runtimes.fakes import FakeRuntimeAdapter, make_run

NOW = datetime(2026, 7, 12, 19, tzinfo=UTC)
OWNER = ExecutionOwnerId("worker_executor01")


@final
class LoadedRepository:
    """返回一个已认领 Run 的执行测试仓储."""

    def __init__(self, run: Run) -> None:
        self.run = run

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """单执行测试不经过调度认领."""
        raise AssertionError(request)

    async def get(self, run_id: RunId) -> GetResult:
        """返回精确测试 Run."""
        assert run_id == self.run.id
        return self.run

    async def renew(self, request: RenewRequest) -> RenewResult:
        """即时输出会在首次续租前结束."""
        raise AssertionError(request)


@final
class RecordingCommitter:
    """记录终态请求并返回幂等重放."""

    def __init__(
        self,
        run: Run,
        first_result: TerminalCommitResult | None = None,
    ) -> None:
        self.run = run
        self.first_result = first_result
        self.requests: list[TerminalCommitRequest] = []

    async def commit(self, request: TerminalCommitRequest) -> TerminalCommitResult:
        """记录原子请求."""
        self.requests.append(request)
        if len(self.requests) == 1 and self.first_result is not None:
            return self.first_result
        return TerminalReplay(self.run)


@final
class MissingRepository:
    """返回不存在结果以证明执行安全停止."""

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        raise AssertionError(request)

    async def get(self, run_id: RunId) -> GetResult:
        return NotFound(run_id)

    async def renew(self, request: RenewRequest) -> RenewResult:
        raise AssertionError(request)


@final
class FailureAdapter:
    """产生一个已清洗 SIMPLE 失败终态."""

    identity = AdapterIdentity("failure-worker", RuntimeKind.SIMPLE)

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        _ = run
        yield SimpleRuntimeOutput(ProviderFailure(FailureKind.EMPTY))

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        return self.execute(run)


@final
class RecordingStreamStore:
    """Record transient envelopes and optionally fail every append."""

    def __init__(self, *, fail_append: bool = False) -> None:
        self.fail_append = fail_append
        self.activations: list[tuple[RunId, int]] = []
        self.envelopes: list[StreamEnvelope] = []

    async def activate_attempt(self, run_id: RunId, attempt: int) -> None:
        self.activations.append((run_id, attempt))

    async def append(self, envelope: StreamEnvelope) -> RedisStreamId:
        if self.fail_append:
            raise RedisUnavailableError
        self.envelopes.append(envelope)
        return RedisStreamId(f"{len(self.envelopes)}-0")


@final
class StreamingAdapter:
    """Emit several small text deltas followed by a successful terminal."""

    identity = AdapterIdentity("stream-worker", RuntimeKind.SIMPLE)

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        _ = run
        for content in ("ab", "cd", "ef"):
            yield SimpleRuntimeOutput(ProviderDelta(content))
        yield SimpleRuntimeOutput(CompletionFinished("stop"))

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        return self.execute(run)


@final
class RenewingRepository:
    """在首次续租返回指定控制结果."""

    def __init__(self, run: Run, result: RenewResult) -> None:
        self.run = run
        self.result = result

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        raise AssertionError(request)

    async def get(self, run_id: RunId) -> GetResult:
        assert run_id == self.run.id
        return self.run

    async def renew(self, request: RenewRequest) -> RenewResult:
        _ = request
        return self.result


@final
class BlockingAdapter:
    """保持执行活动直到续租控制路径取消作用域."""

    identity = AdapterIdentity("blocking-worker", RuntimeKind.SIMPLE)

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        _ = run
        blocked = anyio.Event()
        await blocked.wait()
        yield SimpleRuntimeOutput(CompletionFinished("stop"))

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        return self.execute(run)


@final
class StructuredRunner:
    """Return one capability-validated result for the Worker path."""

    async def execute(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        _ = run, lease
        return AgentSucceeded(
            Capability.WRITE,
            ArticleDraft(title="标题", outline="大纲", markdown="# 正文"),
        )

    async def resume(self, run: Run, command: object, lease: RunLease) -> AgentRunOutcome:
        _ = command
        return await self.execute(run, lease)


@final
class FailedRunner:
    """Return one sanitized dependency failure for the Worker path."""

    async def execute(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        _ = run, lease
        return AgentFailed(
            AgentFailure(
                kind=AgentFailureKind.DEPENDENCY,
                message="Blog 依赖不可用",
                retryable=True,
            )
        )

    async def resume(self, run: Run, command: object, lease: RunLease) -> AgentRunOutcome:
        _ = command
        return await self.execute(run, lease)


def running_run() -> Run:
    """构造与租约 revision 一致的运行中 SIMPLE Run."""
    base = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))
    return replace(
        base,
        revision=2,
        status=RunStatus.RUNNING,
        updated_at=NOW,
        attempt=1,
        execution_owner=OWNER,
    )


def lease(*, cancelled: bool = False) -> RunLease:
    """构造活动围栏并可携带已持久化取消请求."""
    return RunLease(
        RunId("run_12345678"),
        OWNER,
        LeaseToken(UUID("12345678-1234-5678-9234-567812345678")),
        2,
        1,
        NOW + timedelta(minutes=1),
        NOW if cancelled else None,
    )


def router() -> RuntimeRouter:
    """构造共享精确注册适配器的静态目录."""
    return RuntimeRouter(
        default_registry(
            FakeRuntimeAdapter(AdapterIdentity("simple-worker", RuntimeKind.SIMPLE)),
            FakeRuntimeAdapter(AdapterIdentity("deep-worker", RuntimeKind.DEEP)),
        )
    )


@pytest.mark.anyio
async def test_agent_runner_result_is_attached_to_terminal_commit() -> None:
    """Given AgentRunner success, When Worker commits, Then result payload is atomic input."""
    run = running_run()
    committer = RecordingCommitter(run)

    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW, StructuredRunner()),
        router(),
        WorkerConfig(),
    )

    request = committer.requests[0]
    assert request.completion.status is RunStatus.SUCCEEDED
    assert request.result is not None
    assert request.result.capability == "write"
    assert request.result.payload["markdown"] == "# 正文"


@pytest.mark.anyio
async def test_agent_runner_failure_is_attached_to_terminal_commit() -> None:
    """Given sanitized Agent failure, When Worker commits, Then code and message persist."""
    run = running_run()
    committer = RecordingCommitter(run)

    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW, FailedRunner()),
        router(),
        WorkerConfig(),
    )

    request = committer.requests[0]
    assert request.completion.status is RunStatus.FAILED
    assert request.error_code == AgentFailureKind.DEPENDENCY.value
    assert request.error_message == "Blog 依赖不可用"


@pytest.mark.anyio
async def test_native_output_is_normalized_once_and_committed_atomically() -> None:
    """Given SIMPLE 原生终态, When 执行, Then 提交唯一成功批."""
    run = running_run()
    committer = RecordingCommitter(run)

    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        router(),
        WorkerConfig(),
    )

    assert len(committer.requests) == 1
    request = committer.requests[0]
    assert request.completion.status is RunStatus.SUCCEEDED
    assert request.events.events[-1].revision == 3


@pytest.mark.anyio
async def test_preobserved_cancellation_commits_only_cancelled_terminal() -> None:
    """Given 租约携带取消请求, When 执行, Then 不调用运行时并提交取消."""
    run = running_run()
    committer = RecordingCommitter(run)

    await execute_lease(
        lease(cancelled=True),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        router(),
        WorkerConfig(),
    )

    request = committer.requests[0]
    assert request.completion.status is RunStatus.CANCELLED
    assert isinstance(request.events.events[-1], RunCancelled)


@pytest.mark.anyio
async def test_agent_runner_cancellation_drops_failure_details() -> None:
    """Given preobserved cancellation, When Agent path commits, Then no failure fields leak."""
    run = running_run()
    committer = RecordingCommitter(run)

    await execute_lease(
        lease(cancelled=True),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW, StructuredRunner()),
        router(),
        WorkerConfig(),
    )

    request = committer.requests[0]
    assert request.completion.status is RunStatus.CANCELLED
    assert request.error_code is None
    assert request.error_message is None


@pytest.mark.anyio
async def test_cancellation_winning_terminal_race_retries_as_cancelled() -> None:
    """Given 成功提交观察到取消, When 处理竞态, Then 第二次仅提交取消终态."""
    run = running_run()
    committer = RecordingCommitter(run, TerminalCancellationRequested(run.id))

    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        router(),
        WorkerConfig(),
    )

    assert [request.completion.status for request in committer.requests] == [
        RunStatus.SUCCEEDED,
        RunStatus.CANCELLED,
    ]


@pytest.mark.anyio
async def test_persisted_runtime_selection_mismatch_commits_failed_terminal() -> None:
    """Given 持久选择漂移, When 执行, Then 不调用适配器并提交失败."""
    run = replace(
        running_run(),
        runtime=RuntimeSelection(RuntimeKind.DEEP, "v1"),
    )
    committer = RecordingCommitter(run)

    await execute_lease(
        lease(),
        WorkerDependencies(LoadedRepository(run), committer, lambda: NOW),
        router(),
        WorkerConfig(),
    )

    assert committer.requests[0].completion.status is RunStatus.FAILED


@pytest.mark.anyio
async def test_stream_text_is_batched_and_flushed_before_terminal() -> None:
    """Small text deltas are coalesced and the remainder precedes the terminal frame."""
    run = running_run()
    committer = RecordingCommitter(run)
    store = RecordingStreamStore()
    runtime_router = RuntimeRouter(
        default_registry(
            StreamingAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-stream", RuntimeKind.DEEP)),
        )
    )

    await execute_lease(
        lease(),
        WorkerDependencies(
            LoadedRepository(run),
            committer,
            lambda: NOW,
            stream_store=cast("RedisStreamStore", cast("object", store)),
        ),
        runtime_router,
        WorkerConfig(stream_flush_chars=5, stream_flush_interval=timedelta(seconds=30)),
    )

    assert store.activations == [(run.id, 1)]
    assert [item.kind for item in store.envelopes] == [
        RedisStreamKind.TEXT_DELTA,
        RedisStreamKind.TEXT_DELTA,
        RedisStreamKind.TERMINAL,
    ]
    assert [item.payload["text"] for item in store.envelopes[:2]] == ["abcde", "f"]
    assert committer.requests[0].completion.status is RunStatus.SUCCEEDED


@pytest.mark.anyio
async def test_redis_write_failure_commits_redis_failure_without_success() -> None:
    """A transient-stream write failure must not produce a pseudo-success result."""
    run = running_run()
    committer = RecordingCommitter(run)
    store = RecordingStreamStore(fail_append=True)
    runtime_router = RuntimeRouter(
        default_registry(
            StreamingAdapter(),
            FakeRuntimeAdapter(AdapterIdentity("deep-stream-failure", RuntimeKind.DEEP)),
        )
    )

    await execute_lease(
        lease(),
        WorkerDependencies(
            LoadedRepository(run),
            committer,
            lambda: NOW,
            stream_store=cast("RedisStreamStore", cast("object", store)),
        ),
        runtime_router,
        WorkerConfig(stream_flush_chars=5, stream_flush_interval=timedelta(seconds=30)),
    )

    request = committer.requests[0]
    assert request.completion.status is RunStatus.FAILED
    assert request.result is None
    assert request.error_code == "redis_failure"
    assert request.error_message == "Redis 流存储不可用"
