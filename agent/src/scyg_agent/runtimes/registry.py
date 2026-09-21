"""确定性静态任务目录及其启动期完整性检查."""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Final, override

from scyg_agent.domain.runs import RuntimeKind, RuntimeSelection, TaskType

from .base import (
    AdapterIdentity,
    AdapterIssue,
    InvalidAdapterError,
    RuntimeAdapter,
    ensure_registered_identity,
    validated_adapter_identity,
    validated_runtime_adapter,
)
from .profiles import (
    COMPOSE_DEEP_V1_PROFILE,
    RESEARCH_DEEP_V1_PROFILE,
    REVISE_DEEP_V1_PROFILE,
    RUNTIME_VERSION_V1,
    SIMPLE_V1_PROFILE,
    RuntimeProfile,
)

if TYPE_CHECKING:
    from collections.abc import Sequence

VERSION_MISMATCH_RULE: Final = "键版本与画像版本不一致"
KIND_MISMATCH_RULE: Final = "适配器族与画像选择不一致"
PROFILE_TYPE_RULE: Final = "profile 必须是精确 RuntimeProfile"


@dataclass(frozen=True, slots=True)
class InvalidRuntimeKeyError(ValueError):
    """报告不属于精确 RuntimeKey 类型的冒充值."""

    # concrete_type 仅记录具体类型名称。
    concrete_type: str

    @override
    def __str__(self) -> str:
        """返回稳定的中文键错误."""
        return f"运行时键类型无效: {self.concrete_type}"


@dataclass(frozen=True, slots=True)
class DuplicateRuntimeKeyError(ValueError):
    """报告完全重复的任务和版本键."""

    # key 是发生冲突的冻结键。
    key: RuntimeKey

    @override
    def __str__(self) -> str:
        """返回稳定的中文重复键错误."""
        return f"运行时键重复: {self.key.task_type.value}/{self.key.version}"


@dataclass(frozen=True, slots=True)
class DuplicateTaskRegistrationError(ValueError):
    """报告一个任务同时批准多个注册项."""

    # task_type 是存在版本歧义的闭集任务。
    task_type: TaskType

    @override
    def __str__(self) -> str:
        """返回稳定的中文任务歧义错误."""
        return f"任务注册重复: {self.task_type.value}"


@dataclass(frozen=True, slots=True)
class MissingTaskCoverageError(ValueError):
    """报告静态目录遗漏的闭集任务."""

    # missing 是启动时缺失的不可变任务集合。
    missing: frozenset[TaskType]

    @override
    def __str__(self) -> str:
        """返回按名称排序的稳定中文覆盖错误."""
        names = ",".join(sorted(task.value for task in self.missing))
        return f"运行时目录缺少任务: {names}"


@dataclass(frozen=True, slots=True)
class InvalidRegistryEntryError(ValueError):
    """报告键、画像与适配器身份之间的不一致."""

    # task_type 标识无效条目所属任务。
    task_type: TaskType
    # rule 描述稳定的不变量名称。
    rule: str

    @override
    def __str__(self) -> str:
        """返回稳定的中文条目错误."""
        return f"运行时目录条目无效: {self.task_type.value} {self.rule}"


@dataclass(frozen=True, slots=True)
class InvalidRegistryEntriesError(ValueError):
    """报告可变或伪装的注册项容器."""

    # concrete_type 仅记录容器具体类型名称.
    concrete_type: str

    @override
    def __str__(self) -> str:
        """返回稳定的中文容器错误."""
        return f"运行时目录容器无效: {self.concrete_type}"


@dataclass(frozen=True, slots=True)
class InvalidRegistryEntryTypeError(ValueError):
    """报告不属于精确 RegistryEntry 类型的冒充值."""

    # concrete_type 仅记录条目具体类型名称.
    concrete_type: str

    @override
    def __str__(self) -> str:
        """返回稳定的中文条目类型错误."""
        return f"运行时目录条目类型无效: {self.concrete_type}"


@dataclass(frozen=True, slots=True)
class RuntimeKey:
    """以任务和通过 T08 校验的版本唯一标识注册项."""

    # task_type 是 T08 定义的闭集任务。
    task_type: TaskType
    # version 复用 RuntimeSelection 的 vN 校验规则。
    version: str

    def __post_init__(self) -> None:
        """复用 T08 RuntimeSelection 的版本解析契约."""
        if type(self.task_type) is not TaskType:
            raise InvalidRuntimeKeyError(type(self.task_type).__name__)
        _ = RuntimeSelection(RuntimeKind.SIMPLE, self.version)


@dataclass(frozen=True, slots=True)
class RegistryEntry:
    """绑定一个任务键、冻结画像和显式适配器身份."""

    # key 是静态任务和版本键。
    key: RuntimeKey
    # profile 是运行时资源与能力画像。
    profile: RuntimeProfile
    # adapter 是组合根提供并由路由直接调用的实际协议实例。
    adapter: RuntimeAdapter
    # registered_identity 是注册时捕获的精确冻结身份快照。
    registered_identity: AdapterIdentity


def _parse_entries(entries: Sequence[RegistryEntry]) -> tuple[RegistryEntry, ...]:
    """将有限输入收窄为精确不可变 tuple."""
    if not isinstance(entries, tuple):
        raise InvalidRegistryEntriesError(type(entries).__name__)
    if type(entries) is not tuple:
        raise InvalidRegistryEntriesError(type(entries).__name__)
    return entries


def _validated_entry_identity(entry: RegistryEntry) -> AdapterIdentity:
    """验证条目、键、画像和实际适配器的精确类型."""
    if type(entry) is not RegistryEntry:
        raise InvalidRegistryEntryTypeError(type(entry).__name__)
    if type(entry.key) is not RuntimeKey:
        raise InvalidRuntimeKeyError(type(entry.key).__name__)
    if type(entry.profile) is not RuntimeProfile:
        raise InvalidRegistryEntryError(entry.key.task_type, PROFILE_TYPE_RULE)
    if type(entry.registered_identity) is not AdapterIdentity:
        raise InvalidAdapterError(AdapterIssue.IDENTITY_TYPE)
    ensure_registered_identity(entry.adapter, entry.registered_identity)
    return entry.registered_identity


@dataclass(frozen=True, slots=True)
class RuntimeRegistry:
    """保存完整、无歧义且不带缓存的静态注册项."""

    # entries 是调用方显式提供的有限不可变条目序列。
    entries: Sequence[RegistryEntry]

    def __post_init__(self) -> None:
        """在服务启动时验证键、任务覆盖和画像一致性."""
        seen_keys: set[RuntimeKey] = set()
        seen_tasks: set[TaskType] = set()
        seen_adapters: list[tuple[AdapterIdentity, RuntimeAdapter]] = []
        for entry in _parse_entries(self.entries):
            identity = _validated_entry_identity(entry)
            for known_identity, known_adapter in seen_adapters:
                if entry.adapter is known_adapter and identity != known_identity:
                    raise InvalidAdapterError(AdapterIssue.IDENTITY_CHANGED)
                if identity.name == known_identity.name and entry.adapter is not known_adapter:
                    raise InvalidAdapterError(AdapterIssue.DUPLICATE_NAME)
            if entry.key in seen_keys:
                raise DuplicateRuntimeKeyError(entry.key)
            if entry.key.task_type in seen_tasks:
                raise DuplicateTaskRegistrationError(entry.key.task_type)
            if entry.key.version != entry.profile.selection.version:
                raise InvalidRegistryEntryError(entry.key.task_type, VERSION_MISMATCH_RULE)
            if identity.kind is not entry.profile.selection.kind:
                raise InvalidRegistryEntryError(entry.key.task_type, KIND_MISMATCH_RULE)
            seen_keys.add(entry.key)
            seen_tasks.add(entry.key.task_type)
            seen_adapters.append((identity, entry.adapter))

        missing = frozenset(TaskType) - seen_tasks
        if missing:
            raise MissingTaskCoverageError(missing)

    def entry_for_key(self, key: RuntimeKey) -> RegistryEntry | None:
        """按精确键查找条目, 不使用动态导入或缓存."""
        if type(key) is not RuntimeKey:
            raise InvalidRuntimeKeyError(type(key).__name__)
        for entry in self.entries:
            if entry.key == key:
                _ = _validated_entry_identity(entry)
                return entry
        return None

    def entry_for_task(self, task_type: TaskType) -> RegistryEntry:
        """返回启动期已证明存在的唯一任务条目."""
        for entry in self.entries:
            if entry.key.task_type is task_type:
                _ = _validated_entry_identity(entry)
                return entry
        raise MissingTaskCoverageError(frozenset({task_type}))


def default_registry(
    simple_adapter: RuntimeAdapter,
    deep_adapter: RuntimeAdapter,
) -> RuntimeRegistry:
    """用六个批准的字面量条目构造默认静态目录."""
    simple = validated_runtime_adapter(simple_adapter)
    deep = validated_runtime_adapter(deep_adapter)
    simple_identity = validated_adapter_identity(simple)
    deep_identity = validated_adapter_identity(deep)
    return RuntimeRegistry(
        (
            RegistryEntry(
                RuntimeKey(TaskType.SUMMARY, RUNTIME_VERSION_V1),
                SIMPLE_V1_PROFILE,
                simple,
                simple_identity,
            ),
            RegistryEntry(
                RuntimeKey(TaskType.QUESTION, RUNTIME_VERSION_V1),
                SIMPLE_V1_PROFILE,
                simple,
                simple_identity,
            ),
            RegistryEntry(
                RuntimeKey(TaskType.POLISH, RUNTIME_VERSION_V1),
                SIMPLE_V1_PROFILE,
                simple,
                simple_identity,
            ),
            RegistryEntry(
                RuntimeKey(TaskType.COMPOSE, RUNTIME_VERSION_V1),
                COMPOSE_DEEP_V1_PROFILE,
                deep,
                deep_identity,
            ),
            RegistryEntry(
                RuntimeKey(TaskType.RESEARCH, RUNTIME_VERSION_V1),
                RESEARCH_DEEP_V1_PROFILE,
                deep,
                deep_identity,
            ),
            RegistryEntry(
                RuntimeKey(TaskType.REVISE, RUNTIME_VERSION_V1),
                REVISE_DEEP_V1_PROFILE,
                deep,
                deep_identity,
            ),
        )
    )
