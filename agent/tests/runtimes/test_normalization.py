"""T18 框架结果到领域事件协议的规范化契约测试。"""

from dataclasses import replace
from datetime import UTC, datetime
from typing import Literal

import pytest

from scyg_agent.domain.ports.idempotency import ResultReference
from scyg_agent.domain.ports.tool_store import ToolOutcomeStatus
from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    ApprovalResolvedEvent,
    CommandId,
    RunFailed,
    RunSucceeded,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    TextDelta,
    TokenUsage,
    ToolCallId,
    ToolFailed,
    ToolOutcomeUnknown,
    ToolStarted,
    ToolSucceeded,
)
from scyg_agent.runtimes.deep import (
    ApprovalDecision,
    ApprovalReply,
    ApprovalRequired,
    ExecutionInFlight,
    ExternalOutcomeUnknownResult,
    Rejected,
    ToolFinished,
)
from scyg_agent.runtimes.deep.models import DeepFailureKind
from scyg_agent.runtimes.normalization import (
    DeepFailureState,
    NormalizationContext,
    NormalizationError,
    normalize_deep,
    normalize_simple,
)
from scyg_agent.runtimes.simple import CompletionFinished, ProviderDelta, ProviderFailure
from scyg_agent.runtimes.simple.results import FailureKind, ProviderResult
from tests.runtimes.deep.test_runtime import proposal
from tests.runtimes.fakes import make_run

NOW = datetime(2026, 7, 12, 9, 0, tzinfo=UTC)


def _context() -> NormalizationContext:
    return NormalizationContext(CommandId("cmd_normalize1"), NOW)


def test_simple_preserves_delta_usage_and_terminal_order() -> None:
    # Given: T15 已验证的有序增量、用量与成功终态。
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))
    outcomes = (
        ProviderDelta("甲"),
        ProviderDelta("乙"),
        CompletionFinished("stop", 2, 3, 5),
    )

    # When: 同一规范化边界映射完整物理结果。
    events = normalize_simple(run, _context(), outcomes)

    # Then: 增量不合并且用量严格位于唯一终态之前。
    assert [type(event) for event in events] == [TextDelta, TextDelta, TokenUsage, RunSucceeded]
    assert events[0] == TextDelta(events[0].event_id, events[0].command_id, NOW, run.id, 1, "甲")
    assert events[1] == TextDelta(events[1].event_id, events[1].command_id, NOW, run.id, 1, "乙")


def test_simple_failure_is_one_terminal_without_provider_detail() -> None:
    # Given: T15 的封闭失败类别。
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))

    # When: 规范化失败。
    events = normalize_simple(
        run,
        _context(),
        (ProviderFailure(FailureKind.TIMEOUT),),
    )

    # Then: 只暴露无提供方正文的领域失败。
    assert len(events) == 1
    assert type(events[0]) is RunFailed


@pytest.mark.parametrize(
    "outcomes",
    [
        (),
        (ProviderDelta("甲"),),
        (CompletionFinished("stop"), ProviderDelta("晚到")),
        (CompletionFinished("stop"), ProviderFailure(FailureKind.TIMEOUT)),
    ],
)
def test_simple_rejects_missing_or_nonfinal_terminal(
    outcomes: tuple[ProviderResult, ...],
) -> None:
    # Given: 缺失、重复或终态后仍有回调的非法序列。
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))

    # When/Then: 边界拒绝而不是重排或猜测。
    with pytest.raises(NormalizationError):
        _ = normalize_simple(run, _context(), outcomes)


def test_simple_replay_has_exact_event_identities() -> None:
    # Given: 同一 Run、命令和已验证结果。
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))
    outcomes = (ProviderDelta("稳定"), CompletionFinished("stop"))

    # When: 崩溃重试再次规范化相同结果。
    first = normalize_simple(run, _context(), outcomes)
    second = normalize_simple(run, _context(), outcomes)

    # Then: T11 可按完全相同身份幂等重放。
    assert first == second
    assert tuple(event.event_id for event in first) == tuple(event.event_id for event in second)


def test_deep_proposal_stops_before_tool_start() -> None:
    # Given: T17 在工具 I/O 前持久中断的提案。
    envelope = proposal()
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
    )

    # When: 规范化待审批结果。
    events = normalize_deep(
        run, _context(), envelope, None, ApprovalRequired(run.id, envelope.approval_token)
    )

    # Then: 仅发布审批需求, 不能提前声称工具已开始。
    assert [type(event) for event in events] == [ApprovalRequiredEvent]


def test_deep_rejection_is_terminal_without_tool_event() -> None:
    # Given: 与提案匹配的拒绝决定。
    envelope = proposal()
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
    )

    # When: 规范化拒绝终态。
    events = normalize_deep(
        run,
        _context(),
        envelope,
        ApprovalReply(envelope.approval_token, ApprovalDecision.REJECT),
        Rejected(run.id),
    )

    # Then: 审批事实后直接失败, 绝不产生工具调用事实。
    assert [type(event) for event in events] == [
        ApprovalRequiredEvent,
        ApprovalResolvedEvent,
        RunFailed,
    ]


def test_deep_failed_tool_has_legal_lifecycle_and_deterministic_replay() -> None:
    # Given: 已批准且由 T12 持久化的工具失败终态。
    envelope = proposal()
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
    )
    reply = ApprovalReply(envelope.approval_token, ApprovalDecision.APPROVE)
    outcome = ToolFinished(run.id, envelope.fallback_outcome)

    # When: 相同崩溃重试输入被规范化两次。
    first = normalize_deep(run, _context(), envelope, reply, outcome)
    second = normalize_deep(run, _context(), envelope, reply, outcome)

    # Then: 顺序合法且身份完全一致。
    assert [type(event) for event in first] == [
        ApprovalRequiredEvent,
        ApprovalResolvedEvent,
        ToolStarted,
        ToolFailed,
        RunFailed,
    ]
    assert first == second


@pytest.mark.parametrize(
    "result_kind",
    [
        "external_unknown",
        "in_flight",
        "deep_failure",
    ],
)
def test_deep_closed_non_tool_terminal_variants(
    result_kind: Literal["external_unknown", "in_flight", "deep_failure"],
) -> None:
    # Given: 已批准提案与一个 T17 封闭结果变体。
    envelope = proposal()
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
    )
    reply = ApprovalReply(envelope.approval_token, ApprovalDecision.APPROVE)

    # When: 规范化结果。
    match result_kind:  # noqa: RUF100  # noqa: MATCH_OK - Literal 三分支已完整映射。
        case "external_unknown":
            result = ExternalOutcomeUnknownResult(run.id)
            expected_tail = [ToolStarted, ToolOutcomeUnknown, RunFailed]
        case "in_flight":
            result = ExecutionInFlight(run.id)
            expected_tail = [ToolStarted]
        case "deep_failure":
            result = DeepFailureState(DeepFailureKind.PERSISTENCE_FAILURE)
            expected_tail = [RunFailed]
    events = normalize_deep(run, _context(), envelope, reply, result)

    # Then: 公共前缀后保持该变体的稳定语义。
    assert [type(event) for event in events[2:]] == expected_tail


def test_deep_external_unknown_tool_operation_is_terminal_failure() -> None:
    # Given: T12 工具终态自身标记外部结果未知。
    envelope = proposal()
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
    )
    unknown = replace(
        envelope.fallback_outcome,
        status=ToolOutcomeStatus.EXTERNAL_OUTCOME_UNKNOWN,
    )

    # When: 规范化批准后的工具结果。
    events = normalize_deep(
        run,
        _context(),
        envelope,
        ApprovalReply(envelope.approval_token, ApprovalDecision.APPROVE),
        ToolFinished(run.id, unknown),
    )

    # Then: 未知外部结果不会被伪装为成功或自动重试。
    assert [type(event) for event in events[-2:]] == [ToolOutcomeUnknown, RunFailed]


@pytest.mark.parametrize(
    "finished",
    [
        CompletionFinished("stop", 1, None, 1),
        CompletionFinished("stop", None, 1, 1),
        CompletionFinished("stop", 1, 1, None),
    ],
)
def test_simple_rejects_partial_usage(finished: CompletionFinished) -> None:
    # Given: T15 边界之后仍不完整的用量终态。
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))

    # When/Then: 规范化器拒绝猜测缺失计数。
    with pytest.raises(NormalizationError, match="令牌用量不完整"):
        _ = normalize_simple(run, _context(), (finished,))


def test_normalizers_reject_wrong_runtime_and_missing_approval() -> None:
    # Given: 与规范化路径不匹配的运行时和缺失审批决定。
    envelope = proposal()
    deep_run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
    )

    # When/Then: 两个身份/阶段错误均在边界被拒绝。
    with pytest.raises(NormalizationError, match="运行时选择不匹配"):
        _ = normalize_simple(deep_run, _context(), (CompletionFinished("stop"),))
    with pytest.raises(NormalizationError, match="缺少审批决定"):
        _ = normalize_deep(deep_run, _context(), envelope, None, Rejected(deep_run.id))


def test_deep_success_and_identity_rejections_cover_closed_boundary() -> None:
    # Given: 一个合法提案、成功工具终态和若干阶段身份冲突。
    envelope = proposal()
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
    )
    reply = ApprovalReply(envelope.approval_token, ApprovalDecision.APPROVE)
    succeeded = replace(
        envelope.fallback_outcome,
        status=ToolOutcomeStatus.SUCCEEDED,
        result_reference=ResultReference("article:stable"),
        error_code=None,
    )

    # When: 规范化成功路径。
    success_events = normalize_deep(
        run, _context(), envelope, reply, ToolFinished(run.id, succeeded)
    )

    # Then: 成功工具事实严格先于图成功终态。
    assert [type(event) for event in success_events[-2:]] == [ToolSucceeded, RunSucceeded]
    with pytest.raises(NormalizationError, match="身份或阶段无效"):
        _ = normalize_deep(run, _context(), envelope, reply, ApprovalRequired(run.id, "token"))
    with pytest.raises(NormalizationError, match="拒绝结果与审批决定不匹配"):
        _ = normalize_deep(run, _context(), envelope, reply, Rejected(run.id))
    wrong = replace(envelope.fallback_outcome, tool_call_id=ToolCallId("tool_wrongcall1"))
    with pytest.raises(NormalizationError, match="工具结果身份"):
        _ = normalize_deep(run, _context(), envelope, reply, ToolFinished(run.id, wrong))


def test_simple_rejects_subclass_impostor_and_error_is_stable() -> None:
    # Given: 一个继承合法结果类型的冒充对象。
    class DeltaImpostor(ProviderDelta):
        """模拟框架绕过精确类型闭集."""

    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))

    # When/Then: 精确类型检查拒绝冒充且错误不反射内容。
    with pytest.raises(NormalizationError, match="结果类型不受支持") as captured:
        _ = normalize_simple(run, _context(), (DeltaImpostor("secret-marker"),))
    assert "secret-marker" not in str(captured.value)
