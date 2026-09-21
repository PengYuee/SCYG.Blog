"""将一个封闭运行时原生输出流规范化且仅规范化一次."""

from typing import assert_never

from scyg_agent.domain.runs import DomainEvent, Run
from scyg_agent.runtimes.outputs import (
    DeepRuntimeOutput,
    RuntimeNativeOutput,
    SimpleRuntimeOutput,
)

from .deep_normalization import normalize_deep
from .normalization_common import DeepFailureState, NormalizationContext, NormalizationError
from .simple_normalization import normalize_simple


def normalize_runtime(
    run: Run,
    context: NormalizationContext,
    outputs: tuple[RuntimeNativeOutput, ...],
) -> tuple[DomainEvent, ...]:
    """按精确输出族调用唯一 T18 映射器."""
    if not outputs:
        reason = "原生输出不能为空"
        raise NormalizationError(reason)
    first = outputs[0]
    match first:  # noqa: RUF100  # noqa: MATCH_OK - RuntimeNativeOutput 静态闭集已完整分派。
        case SimpleRuntimeOutput():
            if not all(type(output) is SimpleRuntimeOutput for output in outputs):
                reason = "运行时输出族不能混合"
                raise NormalizationError(reason)
            return normalize_simple(
                run,
                context,
                tuple(output.outcome for output in outputs if type(output) is SimpleRuntimeOutput),
            )
        case DeepRuntimeOutput(proposal=proposal, reply=reply, result=result):
            if len(outputs) != 1 or type(first) is not DeepRuntimeOutput:
                reason = "DEEP 执行必须产生一个原生结果"
                raise NormalizationError(reason)
            return normalize_deep(run, context, proposal, reply, result)
    assert_never(first)


__all__ = (
    "DeepFailureState",
    "NormalizationContext",
    "NormalizationError",
    "normalize_deep",
    "normalize_runtime",
    "normalize_simple",
)
