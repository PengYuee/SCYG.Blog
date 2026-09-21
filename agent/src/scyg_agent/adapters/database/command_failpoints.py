"""命令事务测试故障点。."""

from dataclasses import dataclass
from enum import StrEnum
from typing import Final, Protocol, override


class CommandStage(StrEnum):
    """标识原子命令事务中可观测的持久化边界。."""

    RUN_UPDATED = "run_updated"
    EVENT_APPENDED = "event_appended"
    COMMAND_COMPLETED = "command_completed"
    BEFORE_AUDIT_ADD = "before_audit_add"
    AFTER_AUDIT_ADD = "after_audit_add"
    AFTER_AUDIT_FLUSH = "after_audit_flush"


class CommandFailpoint(Protocol):
    """允许测试在指定事务阶段注入确定性失败。."""

    async def reach(self, stage: CommandStage) -> None:
        """观察一个阶段或抛出类型化测试故障。."""
        ...  # pragma: no cover


@dataclass(frozen=True, slots=True)
class NoCommandFailpoint:
    """生产默认的无操作故障点。."""

    async def reach(self, stage: CommandStage) -> None:
        """保持生产事务路径不变。."""
        _ = stage


@dataclass(frozen=True, slots=True)
class InjectedCommandFailureError(RuntimeError):
    """报告测试指定的事务阶段故障。."""

    stage: CommandStage

    @override
    def __str__(self) -> str:
        """返回稳定且不包含请求内容的诊断。."""
        return f"injected command failure at {self.stage.value}"


NO_COMMAND_FAILPOINT: Final = NoCommandFailpoint()
