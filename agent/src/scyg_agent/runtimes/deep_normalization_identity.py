"""Deep 规范化入口的 T14 兼容性与品牌身份校验."""

from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.tool_store import ToolIntent
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    Run,
    RunId,
    RuntimeKind,
    RuntimeSelection,
    SubmitInput,
    TaskType,
    ToolCallId,
)
from scyg_agent.runtimes.deep.models import ProposalEnvelope
from scyg_agent.runtimes.profiles import deep_profile_for_task

from .normalization_common import NormalizationContext, NormalizationError


def validate_deep_identity(
    run: Run, context: NormalizationContext, proposal: ProposalEnvelope
) -> None:
    """复用 T14 画像并拒绝所有品牌身份子类或错配."""
    if type(run) is not Run or type(context) is not NormalizationContext:
        reason = "运行时、任务、版本或身份不匹配"
        raise NormalizationError(reason)
    expected = deep_profile_for_task(run.task_type) if type(run.task_type) is TaskType else None
    intent = proposal.intent
    resolution = proposal.resolution
    submission = resolution.command
    command = submission.command
    valid = (
        type(run.id) is RunId
        and type(run.task_type) is TaskType
        and type(run.runtime) is RuntimeSelection
        and type(run.runtime.kind) is RuntimeKind
        and type(run.runtime.version) is str
        and type(context.command_id) is CommandId
        and expected is not None
        and run.runtime == expected.selection
        and type(intent) is ToolIntent
        and type(intent.run_id) is RunId
        and type(intent.tool_call_id) is ToolCallId
        and type(intent.operation_id) is OperationId
        and type(intent.approval_interaction_id) is InteractionId
        and type(resolution.interaction_id) is InteractionId
        and type(submission) is CommandSubmission
        and type(submission.command_id) is CommandId
        and type(submission.run_id) is RunId
        and type(command) is SubmitInput
        and type(command.command_id) is CommandId
        and type(command.event_id) is EventId
        and type(command.interaction_id) is InteractionId
        and type(proposal.approval_token) is str
        and intent.run_id == run.id
    )
    if not valid:
        reason = "运行时、任务、版本或身份不匹配"
        raise NormalizationError(reason)
