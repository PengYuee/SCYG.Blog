"""运行时规范化共享的身份和错误值."""

from dataclasses import dataclass
from datetime import datetime
from hashlib import sha256
from typing import override

from scyg_agent.domain.runs import CommandId, EventId, Run, RunId
from scyg_agent.runtimes.deep.models import DeepFailureKind


@dataclass(frozen=True, slots=True)
class NormalizationContext:
    """绑定一次确定性规范化所需的命令身份和时间."""

    command_id: CommandId
    occurred_at: datetime


@dataclass(frozen=True, slots=True)
class NormalizationError(ValueError):
    """拒绝不属于已确认闭集或违反顺序的运行时结果."""

    reason: str

    @override
    def __str__(self) -> str:
        return f"运行时结果无法规范化: {self.reason}"


@dataclass(frozen=True, slots=True)
class DeepFailureState:
    """承载已在 Deep 边界清洗的封闭失败状态."""

    kind: DeepFailureKind


def event_common(
    run: Run,
    context: NormalizationContext,
    index: int,
    kind: str,
    payload: str,
) -> tuple[EventId, CommandId, datetime, RunId, int]:
    """生成可在崩溃重试中精确复现的事件身份."""
    material = "\x1f".join(
        (str(run.id), str(context.command_id), str(run.attempt), str(index), kind, payload)
    )
    digest = sha256(material.encode()).hexdigest()[:24]
    return EventId(f"evt_{digest}"), context.command_id, context.occurred_at, run.id, run.revision
