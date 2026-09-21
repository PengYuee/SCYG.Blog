"""T15 Simple 结果的严格领域事件规范化."""

from typing import Final, assert_never

from scyg_agent.domain.runs import (
    CommandId,
    DomainEvent,
    Run,
    RunFailed,
    RunId,
    RunSucceeded,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    TextDelta,
    TokenUsage,
)
from scyg_agent.runtimes.profiles import SIMPLE_V1_PROFILE, deep_profile_for_task
from scyg_agent.runtimes.simple.results import (
    CompletionFinished,
    ProviderDelta,
    ProviderFailure,
    ProviderResult,
)

from .normalization_common import NormalizationContext, NormalizationError, event_common

MAX_TOKEN_USAGE: Final = 2_147_483_647


def normalize_simple(  # noqa: C901
    run: Run,
    context: NormalizationContext,
    outcomes: tuple[ProviderResult, ...],
) -> tuple[DomainEvent, ...]:
    """保持增量顺序并在完整用量之后产生唯一终态."""
    if (
        type(run) is not Run
        or type(run.id) is not RunId
        or type(run.task_type) is not TaskType
        or type(run.runtime) is not RuntimeSelection
        or type(run.runtime.kind) is not RuntimeKind
        or type(context) is not NormalizationContext
        or type(context.command_id) is not CommandId
        or deep_profile_for_task(run.task_type) is not None
        or run.runtime != SIMPLE_V1_PROFILE.selection
    ):
        reason = "运行时选择不匹配"
        raise NormalizationError(reason)
    events: list[DomainEvent] = []
    terminal_seen = False
    for index, outcome in enumerate(outcomes):
        if type(outcome) not in (ProviderDelta, CompletionFinished, ProviderFailure):
            reason = "结果类型不受支持"
            raise NormalizationError(reason)
        if terminal_seen:
            reason = "终态之后仍有结果"
            raise NormalizationError(reason)
        match outcome:  # noqa: RUF100  # noqa: MATCH_OK - 精确类型闭集已完备。
            case ProviderDelta(content=content):
                if content == "":
                    reason = "文本增量不能为空"
                    raise NormalizationError(reason)
                events.append(
                    TextDelta(*event_common(run, context, index, "text", content), content)
                )
                continue
            case CompletionFinished(
                prompt_tokens=prompt,
                completion_tokens=completion,
                total_tokens=total,
            ):
                if (prompt is None) != (completion is None) or (prompt is None) != (total is None):
                    reason = "令牌用量不完整"
                    raise NormalizationError(reason)
                if prompt is not None and completion is not None and total is not None:
                    _validate_usage(prompt, completion, total)
                    events.append(
                        TokenUsage(
                            *event_common(
                                run, context, index, "usage", f"{prompt}:{completion}:{total}"
                            ),
                            prompt,
                            completion,
                            total,
                        )
                    )
                events.append(RunSucceeded(*event_common(run, context, index, "succeeded", "")))
                terminal_seen = True
                continue
            case ProviderFailure(kind=kind):
                events.append(RunFailed(*event_common(run, context, index, "failed", kind.value)))
                terminal_seen = True
                continue
        assert_never(outcome)
    if not terminal_seen:
        reason = "缺少终态"
        raise NormalizationError(reason)
    return tuple(events)


def _validate_usage(prompt: int, completion: int, total: int) -> None:
    """拒绝非精确整数、越界或内部不一致的令牌用量."""
    values = (prompt, completion, total)
    if any(type(value) is not int or value < 0 or value > MAX_TOKEN_USAGE for value in values):
        reason = "令牌用量超出允许范围"
        raise NormalizationError(reason)
    if prompt + completion != total:
        reason = "令牌用量不一致"
        raise NormalizationError(reason)
