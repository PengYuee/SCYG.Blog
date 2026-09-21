"""创建前选择与恢复时一致性校验路由."""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, override

from scyg_agent.domain.runs import (
    InvalidEnumValueError,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
)

from .base import ensure_registered_identity
from .registry import RegistryEntry, RuntimeKey, RuntimeRegistry

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

    from scyg_agent.domain.runs import Run

    from .base import AdapterIdentity, RuntimeAdapter
    from .outputs import RuntimeNativeOutput
    from .profiles import RuntimeProfile


@dataclass(frozen=True, slots=True)
class UnknownRuntimeVersion:
    """报告任务存在但请求版本未获批准."""

    # task_type 是已知但请求版本未注册的闭集任务。
    task_type: TaskType
    # version 是通过 T08 形状校验但未注册的版本。
    version: str

    @override
    def __str__(self) -> str:
        """返回稳定的中文未知版本错误."""
        return f"运行时版本未注册: {self.task_type.value}/{self.version}"


@dataclass(frozen=True, slots=True)
class UnknownTaskType:
    """报告原始任务不属于 T08 闭集."""

    # task_type 是未识别的原始任务文本。
    task_type: str

    @override
    def __str__(self) -> str:
        """返回不会误报版本的稳定中文错误."""
        return f"未知任务类型: {self.task_type}"


@dataclass(frozen=True, slots=True)
class RuntimeSelectionMismatch:
    """报告持久化选择与当前静态目录不一致."""

    # task_type 是待恢复 Run 的任务。
    task_type: TaskType
    # persisted 是 Run 中不可变的原始选择。
    persisted: RuntimeSelection
    # registered 是当前目录对同任务批准的选择。
    registered: RuntimeSelection

    @override
    def __str__(self) -> str:
        """返回稳定的中文恢复不匹配错误."""
        return f"持久化运行时选择不匹配: {self.task_type.value}"


@dataclass(frozen=True, slots=True)
class RuntimeRoute:
    """返回创建或恢复可使用的完整冻结路由."""

    # key 是静态目录键。
    key: RuntimeKey
    # selection 是创建时保存或恢复时验证后的选择。
    selection: RuntimeSelection
    # profile 是能力、资源和工具权限画像。
    profile: RuntimeProfile
    # adapter 是注册表持有并由路由直接调用的实际协议实例。
    adapter: RuntimeAdapter
    # registered_identity 是注册表捕获且不随 adapter 改变的身份快照。
    registered_identity: AdapterIdentity

    def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """直接调用注册时注入的适配器实例."""
        ensure_registered_identity(self.adapter, self.registered_identity)
        return self.adapter.execute(run)

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """直接恢复注册时注入的适配器实例."""
        ensure_registered_identity(self.adapter, self.registered_identity)
        return self.adapter.resume(run)


@dataclass(frozen=True, slots=True)
class RuntimeRouter:
    """在 Run 创建和恢复执行前解析静态目录."""

    # registry 是启动时已验证完整的静态目录。
    registry: RuntimeRegistry

    def resolve_for_creation(self, raw_task_type: str, version: str) -> CreationRoute:
        """在 Run 构造前解析原始任务和版本为冻结选择与画像."""
        try:
            task_type = TaskType.parse(raw_task_type)
        except InvalidEnumValueError:
            _ = RuntimeSelection(RuntimeKind.SIMPLE, version)
            return UnknownTaskType(raw_task_type)
        key = RuntimeKey(task_type, version)
        entry = self.registry.entry_for_key(key)
        if entry is None:
            return UnknownRuntimeVersion(task_type, version)
        return _route(entry, entry.profile.selection)

    def resolve_for_resume(self, run: Run) -> ResumeRoute:
        """验证持久化任务、运行时族和版本后返回原选择."""
        entry = self.registry.entry_for_task(run.task_type)
        registered = entry.profile.selection
        if run.runtime != registered:
            return RuntimeSelectionMismatch(run.task_type, run.runtime, registered)
        return _route(entry, run.runtime)


def _route(entry: RegistryEntry, selection: RuntimeSelection) -> RuntimeRoute:
    """从已验证注册项构造不可变路由结果."""
    return RuntimeRoute(
        entry.key,
        selection,
        entry.profile,
        entry.adapter,
        entry.registered_identity,
    )


type CreationRoute = RuntimeRoute | UnknownRuntimeVersion | UnknownTaskType
type ResumeRoute = RuntimeRoute | RuntimeSelectionMismatch
