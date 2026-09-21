"""Worker 生命周期的封闭错误."""

from dataclasses import dataclass
from enum import StrEnum
from typing import override


class WorkerState(StrEnum):
    """描述单进程 Worker 的完整生命周期."""

    NEW = "new"
    RUNNING = "running"
    STOPPING = "stopping"
    STOPPED = "stopped"


@dataclass(frozen=True, slots=True)
class InvalidWorkerTransitionError(RuntimeError):
    """拒绝重复启动或无效停止."""

    current: WorkerState
    operation: str

    @override
    def __str__(self) -> str:
        return f"Worker 生命周期转换无效: {self.current.value}/{self.operation}"
