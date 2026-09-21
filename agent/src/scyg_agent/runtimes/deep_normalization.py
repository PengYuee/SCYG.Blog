"""T17 Deep 结果的严格领域事件规范化."""

from typing import assert_never

from scyg_agent.domain.ports.tool_store import ToolOperation, ToolOutcomeStatus
from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    ApprovalResolvedEvent,
    DomainEvent,
    OperationId,
    Run,
    RunFailed,
    RunId,
    RunSucceeded,
    ToolCallId,
    ToolFailed,
    ToolOutcomeUnknown,
    ToolStarted,
    ToolSucceeded,
)
from scyg_agent.runtimes.deep.models import (
    ApprovalDecision,
    ApprovalReply,
    ApprovalRequired,
    DeepResult,
    ExecutionInFlight,
    ExternalOutcomeUnknownResult,
    ProposalEnvelope,
    Rejected,
    ToolFinished,
)
from scyg_agent.runtimes.deep_normalization_identity import validate_deep_identity
from scyg_agent.runtimes.normalization_common import (
    DeepFailureState,
    NormalizationContext,
    NormalizationError,
    event_common,
)


def normalize_deep(  # noqa: C901, PLR0912, PLR0915
    run: Run,
    context: NormalizationContext,
    proposal: ProposalEnvelope,
    reply: ApprovalReply | None,
    result: DeepResult | DeepFailureState,
) -> tuple[DomainEvent, ...]:
    """按提案、审批、工具与图终态顺序映射 T17 结果."""
    allowed_types = (
        ApprovalRequired,
        Rejected,
        ToolFinished,
        ExternalOutcomeUnknownResult,
        ExecutionInFlight,
        DeepFailureState,
    )
    if type(proposal) is not ProposalEnvelope or type(result) not in allowed_types:
        reason = "结果类型不受支持"
        raise NormalizationError(reason)
    validate_deep_identity(run, context, proposal)
    interaction_id = proposal.intent.approval_interaction_id
    tool_call_id = proposal.intent.tool_call_id
    if type(result) is ApprovalRequired:
        if reply is not None or type(result.run_id) is not RunId or result.run_id != run.id:
            reason = "待审批结果身份或阶段无效"
            raise NormalizationError(reason)
        return (
            ApprovalRequiredEvent(
                *event_common(run, context, 0, "approval_required", str(interaction_id)),
                interaction_id,
            ),
        )
    if reply is None:
        reason = "工具终态缺少审批决定"
        raise NormalizationError(reason)
    if (
        type(reply) is not ApprovalReply
        or reply.token != proposal.approval_token
        or proposal.resolution.interaction_id != interaction_id
    ):
        reason = "审批身份不匹配"
        raise NormalizationError(reason)
    approved = reply.decision is ApprovalDecision.APPROVE
    events: list[DomainEvent] = [
        ApprovalRequiredEvent(
            *event_common(run, context, 0, "approval_required", str(interaction_id)),
            interaction_id,
        )
    ]
    events.append(
        ApprovalResolvedEvent(
            *event_common(
                run,
                context,
                1,
                "approval_resolved",
                f"{interaction_id}:{approved}",
            ),
            interaction_id,
            approved,
        )
    )
    match result:  # noqa: RUF100  # noqa: MATCH_OK - 前置精确类型闭集已完备。
        case Rejected(run_id=result_run_id):
            if approved or type(result_run_id) is not RunId or result_run_id != run.id:
                reason = "拒绝结果与审批决定不匹配"
                raise NormalizationError(reason)
            events.append(
                RunFailed(*event_common(run, context, 2, "rejected", str(interaction_id)))
            )
            return tuple(events)
        case ToolFinished(run_id=result_run_id, outcome=outcome):
            if (
                not approved
                or type(result_run_id) is not RunId
                or type(outcome) is not ToolOperation
                or type(outcome.run_id) is not RunId
                or type(outcome.tool_call_id) is not ToolCallId
                or type(outcome.operation_id) is not OperationId
                or result_run_id != run.id
                or outcome.run_id != run.id
                or outcome.tool_call_id != tool_call_id
            ):
                reason = "工具结果身份或审批决定不匹配"
                raise NormalizationError(reason)
            if type(outcome.status) is not ToolOutcomeStatus:
                reason = "工具状态类型不受支持"
                raise NormalizationError(reason)
            events.append(
                ToolStarted(
                    *event_common(run, context, 2, "tool_started", str(tool_call_id)), tool_call_id
                )
            )
            match outcome.status:  # noqa: RUF100  # noqa: MATCH_OK - 精确工具状态闭集已完备。
                case ToolOutcomeStatus.SUCCEEDED:
                    events.append(
                        ToolSucceeded(
                            *event_common(run, context, 3, "tool_succeeded", str(tool_call_id)),
                            tool_call_id,
                        )
                    )
                    events.append(
                        RunSucceeded(
                            *event_common(
                                run,
                                context,
                                4,
                                "graph_succeeded",
                                f"{interaction_id}:{tool_call_id}",
                            )
                        )
                    )
                case ToolOutcomeStatus.FAILED:
                    events.append(
                        ToolFailed(
                            *event_common(run, context, 3, "tool_failed", str(tool_call_id)),
                            tool_call_id,
                        )
                    )
                    events.append(
                        RunFailed(
                            *event_common(
                                run,
                                context,
                                4,
                                "graph_failed",
                                f"{interaction_id}:{tool_call_id}",
                            )
                        )
                    )
                case ToolOutcomeStatus.EXTERNAL_OUTCOME_UNKNOWN:
                    events.append(
                        ToolOutcomeUnknown(
                            *event_common(run, context, 3, "tool_unknown", str(tool_call_id)),
                            tool_call_id,
                        )
                    )
                    events.append(
                        RunFailed(
                            *event_common(
                                run,
                                context,
                                4,
                                "graph_failed",
                                f"{interaction_id}:{tool_call_id}",
                            )
                        )
                    )
            return tuple(events)
        case ExternalOutcomeUnknownResult(run_id=result_run_id):
            if not approved or type(result_run_id) is not RunId or result_run_id != run.id:
                reason = "外部未知结果身份或审批决定不匹配"
                raise NormalizationError(reason)
            events.append(
                ToolStarted(
                    *event_common(run, context, 2, "tool_started", str(tool_call_id)), tool_call_id
                )
            )
            events.append(
                ToolOutcomeUnknown(
                    *event_common(run, context, 3, "tool_unknown", str(tool_call_id)), tool_call_id
                )
            )
            events.append(
                RunFailed(
                    *event_common(
                        run,
                        context,
                        4,
                        "graph_failed",
                        f"{interaction_id}:{tool_call_id}",
                    )
                )
            )
            return tuple(events)
        case ExecutionInFlight(run_id=result_run_id):
            if not approved or type(result_run_id) is not RunId or result_run_id != run.id:
                reason = "执行中结果身份或审批决定不匹配"
                raise NormalizationError(reason)
            events.append(
                ToolStarted(
                    *event_common(run, context, 2, "tool_started", str(tool_call_id)), tool_call_id
                )
            )
            return tuple(events)
        case DeepFailureState(kind=kind):
            events.append(RunFailed(*event_common(run, context, 2, "deep_failed", kind.value)))
            return tuple(events)
        case ApprovalRequired():
            reason = "重复待审批结果"
            raise NormalizationError(reason)
    assert_never(result)
