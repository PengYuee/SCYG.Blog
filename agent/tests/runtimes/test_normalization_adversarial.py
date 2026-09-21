"""T18 独立审查缺陷的精确回归测试。"""

from dataclasses import replace
from datetime import UTC, datetime

import pytest

from scyg_agent.domain.ports.idempotency import ResultReference
from scyg_agent.domain.ports.tool_store import ToolOutcomeStatus
from scyg_agent.domain.runs import (
    ApprovalResolvedEvent,
    CommandId,
    InteractionId,
    Run,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    ToolCallId,
)
from scyg_agent.runtimes.deep import (
    ApprovalDecision,
    ApprovalReply,
    ApprovalRequired,
    ProposalEnvelope,
    Rejected,
    ToolFinished,
)
from scyg_agent.runtimes.normalization import (
    NormalizationContext,
    NormalizationError,
    normalize_deep,
    normalize_simple,
)
from scyg_agent.runtimes.simple import CompletionFinished, ProviderDelta
from scyg_agent.runtimes.simple.results import ProviderResult
from tests.runtimes.deep.test_runtime import proposal
from tests.runtimes.fakes import make_run

NOW = datetime(2026, 7, 12, 10, 0, tzinfo=UTC)
MAX_USAGE = 2_147_483_647


def _deep_values() -> tuple[ProposalEnvelope, Run, NormalizationContext]:
    envelope = proposal()
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=envelope.intent.run_id,
        attempt=2,
    )
    context = NormalizationContext(CommandId("cmd_adversary1"), NOW)
    return envelope, run, context


def test_wrong_approval_token_fails_before_event_creation() -> None:
    envelope, run, context = _deep_values()
    reply = ApprovalReply("wrong-token", ApprovalDecision.REJECT)

    with pytest.raises(NormalizationError, match="审批身份不匹配"):
        _ = normalize_deep(run, context, envelope, reply, Rejected(run.id))


def test_deep_subclass_is_rejected_by_exact_type_boundary() -> None:
    class RejectedImpostor(Rejected):
        """模拟 Deep 结果子类冒充。"""

    envelope, run, context = _deep_values()
    reply = ApprovalReply(envelope.approval_token, ApprovalDecision.REJECT)

    with pytest.raises(NormalizationError, match="结果类型不受支持"):
        _ = normalize_deep(run, context, envelope, reply, RejectedImpostor(run.id))


@pytest.mark.parametrize(
    "outcomes",
    [
        (ProviderDelta(""), CompletionFinished("stop")),
        (CompletionFinished("stop", prompt_tokens=True, completion_tokens=0, total_tokens=1),),
        (CompletionFinished("stop", -1, 0, -1),),
        (CompletionFinished("stop", MAX_USAGE + 1, 0, MAX_USAGE + 1),),
        (CompletionFinished("stop", 1, 1, 3),),
    ],
)
def test_simple_rejects_malformed_typed_values(
    outcomes: tuple[ProviderResult, ...],
) -> None:
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))
    context = NormalizationContext(CommandId("cmd_adversary1"), NOW)

    with pytest.raises(NormalizationError):
        _ = normalize_simple(run, context, outcomes)


def test_approval_and_terminal_ids_include_interaction_and_attempt() -> None:
    envelope, run, context = _deep_values()
    reply = ApprovalReply(envelope.approval_token, ApprovalDecision.APPROVE)
    succeeded = replace(
        envelope.fallback_outcome,
        status=ToolOutcomeStatus.SUCCEEDED,
        result_reference=ResultReference("article:stable"),
        error_code=None,
    )
    first = normalize_deep(run, context, envelope, reply, ToolFinished(run.id, succeeded))
    changed_intent = replace(
        envelope.intent,
        tool_call_id=ToolCallId("tool_othercall1"),
        approval_interaction_id=replace(envelope.resolution.interaction_id, value="int_otherone1"),
    )
    changed_submit = replace(
        envelope.resolution.command.command,
        interaction_id=changed_intent.approval_interaction_id,
    )
    changed_submission = replace(envelope.resolution.command, command=changed_submit)
    changed_resolution = replace(
        envelope.resolution,
        interaction_id=changed_intent.approval_interaction_id,
        command=changed_submission,
    )
    changed = replace(envelope, intent=changed_intent, resolution=changed_resolution)
    changed_outcome = replace(succeeded, tool_call_id=changed_intent.tool_call_id)
    second = normalize_deep(
        run,
        context,
        changed,
        ApprovalReply(changed.approval_token, ApprovalDecision.APPROVE),
        ToolFinished(run.id, changed_outcome),
    )
    third = normalize_deep(
        replace(run, attempt=3), context, envelope, reply, ToolFinished(run.id, succeeded)
    )

    assert (
        next(event for event in first if type(event) is ApprovalResolvedEvent).event_id
        != next(event for event in second if type(event) is ApprovalResolvedEvent).event_id
    )
    assert first[-1].event_id != second[-1].event_id
    assert first[-1].event_id != third[-1].event_id


@pytest.mark.parametrize(
    "run",
    [
        make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v2")),
    ],
)
def test_deep_rejects_wrong_t14_task_or_version(run: Run) -> None:
    envelope = proposal()
    incompatible = replace(run, id=envelope.intent.run_id)

    with pytest.raises(NormalizationError, match="任务、版本或身份不匹配"):
        _ = normalize_deep(
            incompatible,
            NormalizationContext(CommandId("cmd_adversary1"), NOW),
            envelope,
            None,
            ApprovalRequired(incompatible.id, envelope.approval_token),
        )


def test_deep_rejects_consistently_installed_interaction_id_subclass() -> None:
    class InteractionImpostor(InteractionId):
        """模拟值相等的交互身份子类."""

    envelope, run, context = _deep_values()
    impostor = InteractionImpostor(str(envelope.intent.approval_interaction_id))
    command = replace(envelope.resolution.command.command, interaction_id=impostor)
    submission = replace(envelope.resolution.command, command=command)
    resolution = replace(envelope.resolution, interaction_id=impostor, command=submission)
    changed = replace(
        envelope,
        intent=replace(envelope.intent, approval_interaction_id=impostor),
        resolution=resolution,
    )

    with pytest.raises(NormalizationError, match="任务、版本或身份不匹配"):
        _ = normalize_deep(
            run,
            context,
            changed,
            None,
            ApprovalRequired(run.id, changed.approval_token),
        )
