"""Deep Runtime 持久审批与围栏执行行为。"""

from dataclasses import dataclass
from datetime import timedelta
from uuid import UUID

import pytest
from langgraph.checkpoint.memory import InMemorySaver

from scyg_agent.adapters.blog_grpc import BlogFailure, FailureKind, SearchArticles
from scyg_agent.adapters.blog_grpc.contracts import BlogCommand, BlogResult
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest, ResultMetadata
from scyg_agent.domain.ports.interaction_store import (
    AlreadyResolved,
    InteractionResolution,
    InteractionResolveResult,
)
from scyg_agent.domain.ports.tool_store import (
    ClaimRequest,
    ExternalOutcomeUnknown,
    FirstClaim,
    TerminalReplay,
    ToolClaimResult,
    ToolFence,
    ToolFenceResult,
    ToolIntent,
    ToolIntentPrepared,
    ToolOperation,
    ToolOutcomeStatus,
)
from scyg_agent.domain.runs import OperationId, RunId, TaskType, ToolCallId
from scyg_agent.domain.runs.repository import LeaseToken
from scyg_agent.runtimes.deep import (
    ApprovalDecision,
    ApprovalReply,
    ApprovalRequired,
    DeepDependencies,
    DeepGraphInput,
    DeepRuntime,
    DeepRuntimeError,
    ExternalOutcomeUnknownResult,
    ProposalEnvelope,
    Rejected,
    ToolFinished,
)
from scyg_agent.runtimes.deep.engine import BlogClient
from scyg_agent.runtimes.profiles import RESEARCH_DEEP_V1_PROFILE, ToolPermission
from tests.adapters.blog_grpc.test_client import identity
from tests.adapters.database.scripted_session_support import NOW, resolution


class FakePersistence:
    """提供确定性审批与工具围栏状态机。"""

    def __init__(self) -> None:
        self.prepared: bool = False
        self.rejected: bool = False
        self.claimed: bool = False
        self.rpc_started: bool = False
        self.terminal: ToolOperation | None = None

    async def resolve_and_prepare(
        self, request: InteractionResolution, intent: ToolIntent
    ) -> ToolIntentPrepared:
        _ = request
        self.prepared = True
        return ToolIntentPrepared(intent.operation_id, replayed=False)

    async def resolve(self, request: InteractionResolution) -> InteractionResolveResult:
        self.rejected = True
        return AlreadyResolved(request.interaction_id, request.result_reference)

    async def claim(self, request: ClaimRequest) -> ToolClaimResult:
        if self.terminal is not None:
            return TerminalReplay(self.terminal)
        if self.rpc_started:
            return ExternalOutcomeUnknown(request.operation_id)
        self.claimed = True
        return FirstClaim(fence=ToolFence(request.operation_id, request.token, 1, request.now))

    async def mark_rpc_started(self, fence: ToolFence) -> ToolFenceResult | FirstClaim:
        self.rpc_started = True
        return FirstClaim(fence)

    async def complete(self, fence: ToolFence, outcome: ToolOperation) -> ToolFenceResult:
        _ = fence
        self.terminal = outcome
        return TerminalReplay(outcome)


class FakeClient:
    """记录 T16 公共调用表面。"""

    def __init__(self) -> None:
        self.calls: int = 0

    async def invoke(self, tool_name: str, version: str, command: BlogCommand) -> BlogResult:
        _ = (tool_name, version, command)
        self.calls += 1
        return BlogFailure(FailureKind.NOT_FOUND, retryable=False)


@dataclass(frozen=True, slots=True)
class FixedOutcomes:
    """将 T16 结果映射为清洗终态。"""

    operation: ToolOperation

    def from_blog_result(self, proposal: ProposalEnvelope, result: BlogResult) -> ToolOperation:
        _ = (proposal, result)
        return self.operation


def proposal() -> ProposalEnvelope:
    """构造固定搜索审批提案。"""
    operation_id = OperationId("deep:research:search:1")
    intent = ToolIntent(
        ToolCallId("tool_deepsearch1"),
        operation_id,
        RunId("run_deepsearch1"),
        ToolPermission.SEARCH_ARTICLES.value,
        RequestDigest.parse("a" * 64),
        resolution().interaction_id,
        NOW,
        AuditMetadata("source", "deep-runtime"),
    )
    command = SearchArticles(identity(), "query", 10)
    operation = ToolOperation(
        intent.tool_call_id,
        operation_id,
        intent.run_id,
        intent.tool_name,
        intent.request_digest,
        ToolOutcomeStatus.FAILED,
        None,
        ResultMetadata("classification", "not_found"),
        "not_found",
        NOW,
        intent.audit_metadata,
    )
    return ProposalEnvelope("approval-token-1", intent, resolution(), command, operation)


def runtime(saver: InMemorySaver, persistence: FakePersistence, client: BlogClient) -> DeepRuntime:
    """以固定依赖构建研究图。"""
    dependencies = DeepDependencies(
        RESEARCH_DEEP_V1_PROFILE,
        persistence,
        client,
        FixedOutcomes(proposal().fallback_outcome),
        lambda: LeaseToken(UUID(int=1)),
        lambda: NOW,
        timedelta(minutes=5),
        lambda result: isinstance(result, AlreadyResolved),
    )
    return DeepRuntime.compile(RESEARCH_DEEP_V1_PROFILE, saver, dependencies)


@pytest.fixture
def anyio_backend() -> str:
    """LangGraph 异步执行使用 asyncio 后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_interrupt_reject_and_restart_before_decision_never_call_tool() -> None:
    # Given
    saver = InMemorySaver()
    persistence = FakePersistence()
    client = FakeClient()
    request = DeepGraphInput(TaskType.RESEARCH, proposal())
    waiting = await runtime(saver, persistence, client).start(request)
    # When
    rebuilt = runtime(saver, persistence, client)
    recovered = await rebuilt.pending(request.proposal.intent.run_id)
    rejected = await rebuilt.resume(
        request.proposal.intent.run_id, ApprovalReply("approval-token-1", ApprovalDecision.REJECT)
    )
    # Then
    assert isinstance(waiting, ApprovalRequired)
    assert recovered == waiting
    assert isinstance(rejected, Rejected)
    assert persistence.rejected
    assert client.calls == 0


@pytest.mark.anyio
async def test_approve_prepares_marks_rpc_and_replays_terminal_once() -> None:
    # Given
    saver = InMemorySaver()
    persistence = FakePersistence()
    client = FakeClient()
    request = DeepGraphInput(TaskType.RESEARCH, proposal())
    _ = await runtime(saver, persistence, client).start(request)
    graph = runtime(saver, persistence, client)
    # When
    finished = await graph.resume(
        request.proposal.intent.run_id, ApprovalReply("approval-token-1", ApprovalDecision.APPROVE)
    )
    replay = await graph.terminal(request.proposal.intent.run_id)
    # Then
    assert isinstance(finished, ToolFinished)
    assert replay == finished
    assert persistence.prepared
    assert persistence.claimed
    assert persistence.rpc_started
    assert client.calls == 1


@pytest.mark.anyio
async def test_rpc_started_unknown_outcome_never_calls_again() -> None:
    # Given
    saver = InMemorySaver()
    persistence = FakePersistence()
    persistence.rpc_started = True
    client = FakeClient()
    request = DeepGraphInput(TaskType.RESEARCH, proposal())
    _ = await runtime(saver, persistence, client).start(request)
    # When
    result = await runtime(saver, persistence, client).resume(
        request.proposal.intent.run_id, ApprovalReply("approval-token-1", ApprovalDecision.APPROVE)
    )
    # Then
    assert isinstance(result, ExternalOutcomeUnknownResult)
    assert client.calls == 0


@pytest.mark.anyio
async def test_wrong_token_task_and_missing_thread_fail_without_tool_io() -> None:
    # Given: 研究画像、未记录线程和错误任务。
    saver = InMemorySaver()
    persistence = FakePersistence()
    client = FakeClient()
    graph = runtime(saver, persistence, client)
    request = DeepGraphInput(TaskType.RESEARCH, proposal())

    # When/Then: 缺失线程和任务漂移均在工具调用前失败。
    with pytest.raises(DeepRuntimeError):
        _ = await graph.pending(RunId("run_missing001"))
    with pytest.raises(DeepRuntimeError):
        _ = await graph.start(DeepGraphInput(TaskType.COMPOSE, proposal()))
    _ = await graph.start(request)
    with pytest.raises(DeepRuntimeError):
        _ = await graph.resume(
            request.proposal.intent.run_id,
            ApprovalReply("wrong-token", ApprovalDecision.APPROVE),
        )
    assert client.calls == 0


@pytest.mark.anyio
async def test_duplicate_resume_replays_checkpointed_terminal_without_second_call() -> None:
    # Given: 已完成一次批准工具调用。
    saver = InMemorySaver()
    persistence = FakePersistence()
    client = FakeClient()
    request = DeepGraphInput(TaskType.RESEARCH, proposal())
    graph = runtime(saver, persistence, client)
    _ = await graph.start(request)
    first = await graph.resume(
        request.proposal.intent.run_id,
        ApprovalReply("approval-token-1", ApprovalDecision.APPROVE),
    )

    # When: 相同决定再次到达已终止检查点。
    replay = await graph.resume(
        request.proposal.intent.run_id,
        ApprovalReply("approval-token-1", ApprovalDecision.APPROVE),
    )

    # Then: 返回稳定终态且 T16 只调用一次。
    assert replay == first
    assert client.calls == 1
