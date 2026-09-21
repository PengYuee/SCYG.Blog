"""应用所有的运行时适配器协议与冻结身份."""

import re
from collections.abc import AsyncIterator
from dataclasses import dataclass
from enum import StrEnum
from typing import Final, Protocol, override, runtime_checkable

from scyg_agent.domain.runs import Run, RuntimeKind
from scyg_agent.runtimes.outputs import RuntimeNativeOutput

ADAPTER_NAME_PATTERN: Final = re.compile(r"[a-z][a-z0-9_-]{0,63}")


class AdapterIssue(StrEnum):
    """关闭适配器启动校验失败类别."""

    IDENTITY_NAME = "身份名称必须是安全稳定短名称"
    IDENTITY_KIND = "身份 kind 必须属于 RuntimeKind"
    IDENTITY_TYPE = "identity 必须是精确 AdapterIdentity"
    PROTOCOL = "对象不满足 RuntimeAdapter 协议"
    DUPLICATE_NAME = "身份名称不能绑定不同适配器对象"
    IDENTITY_CHANGED = "同一适配器对象的身份不能变化"


@dataclass(frozen=True, slots=True)
class InvalidAdapterError(ValueError):
    """报告适配器对象或身份不满足启动契约."""

    # issue 是稳定且不包含实现细节的失败类别。
    issue: AdapterIssue

    @override
    def __str__(self) -> str:
        """返回稳定中文适配器错误."""
        return f"运行时适配器无效: {self.issue}"


class AdapterIdentityDriftError(RuntimeError):
    """报告实际适配器身份偏离注册快照."""

    @override
    def __str__(self) -> str:
        """返回不暴露 live identity 的稳定中文错误."""
        return "运行时适配器身份已漂移"


AdapterIdentityDrift: Final = AdapterIdentityDriftError


@dataclass(frozen=True, slots=True)
class AdapterIdentity:
    """标识由组合根显式提供的运行时适配器."""

    # name 是部署内稳定名称, 不承载模块导入路径.
    name: str
    # kind 绑定适配器支持的应用运行时族。
    kind: RuntimeKind

    def __post_init__(self) -> None:
        """拒绝空白、路径式名称和非闭集 kind."""
        if type(self.name) is not str or ADAPTER_NAME_PATTERN.fullmatch(self.name) is None:
            raise InvalidAdapterError(AdapterIssue.IDENTITY_NAME)
        if type(self.kind) is not RuntimeKind:
            raise InvalidAdapterError(AdapterIssue.IDENTITY_KIND)


@runtime_checkable
class RuntimeAdapter(Protocol):
    """定义框架无关的执行与恢复原生输出流."""

    @property
    def identity(self) -> AdapterIdentity:
        """返回组合根注入的冻结适配器身份."""
        ...  # pragma: no cover - 协议声明。

    def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """首次执行一个已固定运行时选择的 Run, 不构造领域事件."""
        ...  # pragma: no cover - 协议声明。

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """恢复一个已验证持久化选择的 Run, 不构造领域事件."""
        ...  # pragma: no cover - 协议声明。


class RuntimeAdapterCandidate(Protocol):
    """描述进入运行时协议校验前可读取身份的候选对象."""

    @property
    def identity(self) -> AdapterIdentity:
        """返回候选身份供启动校验."""
        ...  # pragma: no cover - 协议声明。


def validated_runtime_adapter(adapter: RuntimeAdapterCandidate) -> RuntimeAdapter:
    """验证协议形状并返回同一个实际适配器对象."""
    try:
        conforms = isinstance(adapter, RuntimeAdapter)
    except AttributeError:
        raise InvalidAdapterError(AdapterIssue.PROTOCOL) from None
    if not conforms:
        raise InvalidAdapterError(AdapterIssue.PROTOCOL)
    try:
        identity = adapter.identity
        execute = adapter.execute
        resume = adapter.resume
    except AttributeError:
        raise InvalidAdapterError(AdapterIssue.PROTOCOL) from None
    if type(identity) is not AdapterIdentity:
        raise InvalidAdapterError(AdapterIssue.IDENTITY_TYPE)
    if not callable(execute) or not callable(resume):
        raise InvalidAdapterError(AdapterIssue.PROTOCOL)
    return adapter


def validated_adapter_identity(adapter: RuntimeAdapterCandidate) -> AdapterIdentity:
    """验证实际适配器并返回精确冻结身份."""
    validated = validated_runtime_adapter(adapter)
    identity = validated.identity
    if type(identity) is not AdapterIdentity:
        raise InvalidAdapterError(AdapterIssue.IDENTITY_TYPE)
    return identity


def ensure_registered_identity(
    adapter: RuntimeAdapterCandidate,
    registered_identity: AdapterIdentity,
) -> None:
    """要求 live identity 精确等于注册时冻结快照."""
    if type(registered_identity) is not AdapterIdentity:
        raise InvalidAdapterError(AdapterIssue.IDENTITY_TYPE)
    try:
        live_identity = adapter.identity
    except AttributeError:
        raise AdapterIdentityDrift from None
    if type(live_identity) is not AdapterIdentity or live_identity != registered_identity:
        raise AdapterIdentityDrift
