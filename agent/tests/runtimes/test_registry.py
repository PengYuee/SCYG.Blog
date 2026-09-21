"""静态运行时目录、能力与恢复选择测试。"""

from dataclasses import FrozenInstanceError, dataclass, replace
from datetime import UTC, datetime
from typing import Final

import pytest

from scyg_agent.domain.runs import (
    InvalidRuntimeVersionError,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.profiles import SIMPLE_V1_PROFILE, RuntimeProfile
from scyg_agent.runtimes.registry import (
    DuplicateRuntimeKeyError,
    DuplicateTaskRegistrationError,
    InvalidRegistryEntriesError,
    InvalidRegistryEntryError,
    InvalidRegistryEntryTypeError,
    InvalidRuntimeKeyError,
    MissingTaskCoverageError,
    RegistryEntry,
    RuntimeKey,
    RuntimeRegistry,
    default_registry,
)
from scyg_agent.runtimes.router import (
    RuntimeRoute,
    RuntimeRouter,
    RuntimeSelectionMismatch,
    UnknownRuntimeVersion,
    UnknownTaskType,
)

from .fakes import FakeRuntimeAdapter

VERSION_V1: Final = "v1"
SIMPLE_ADAPTER: Final = FakeRuntimeAdapter(AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE))
DEEP_ADAPTER: Final = FakeRuntimeAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP))
NOW: Final = datetime(2026, 7, 12, 8, 0, tzinfo=UTC)

EXPECTED_ROUTES: Final = (
    (TaskType.SUMMARY, RuntimeKind.SIMPLE, SIMPLE_ADAPTER),
    (TaskType.QUESTION, RuntimeKind.SIMPLE, SIMPLE_ADAPTER),
    (TaskType.POLISH, RuntimeKind.SIMPLE, SIMPLE_ADAPTER),
    (TaskType.COMPOSE, RuntimeKind.DEEP, DEEP_ADAPTER),
    (TaskType.RESEARCH, RuntimeKind.DEEP, DEEP_ADAPTER),
    (TaskType.REVISE, RuntimeKind.DEEP, DEEP_ADAPTER),
)


def _registry() -> RuntimeRegistry:
    """构造无全局状态的确定性默认目录。"""
    return default_registry(SIMPLE_ADAPTER, DEEP_ADAPTER)


def _persisted_run(runtime: RuntimeSelection) -> Run:
    """构造仅用于恢复路由验证的冻结 Run。"""
    return Run(
        id=RunId("run_12345678"),
        owner_user_id=UserId("user-1"),
        task_type=TaskType.SUMMARY,
        runtime=runtime,
        revision=1,
        status=RunStatus.PENDING,
        created_at=NOW,
        updated_at=NOW,
        attempt=0,
        execution_owner=None,
        pending_interaction_id=None,
    )


@pytest.mark.parametrize(("task_type", "kind", "adapter"), EXPECTED_ROUTES)
def test_default_catalog_resolves_all_approved_tasks_once(
    task_type: TaskType,
    kind: RuntimeKind,
    adapter: FakeRuntimeAdapter,
) -> None:
    # Given: 由显式适配器身份构造的静态目录。
    router = RuntimeRouter(_registry())

    # When: 创建 Run 前解析批准的任务和版本。
    route = router.resolve_for_creation(task_type, VERSION_V1)

    # Then: 选择、画像和适配器身份保持一致。
    assert type(route) is RuntimeRoute
    assert route.key == RuntimeKey(task_type, VERSION_V1)
    assert route.selection == RuntimeSelection(kind, VERSION_V1)
    assert route.profile.selection == route.selection
    assert route.adapter.identity.kind is kind
    assert route.adapter is adapter


def test_default_catalog_is_deterministic_and_immutable() -> None:
    # Given/When: 两次构造相同的显式静态目录。
    first = _registry()
    second = _registry()

    # Then: 条目有序相等, 且冻结值不能被修改.
    assert first.entries == second.entries
    assert len(first.entries) == len(TaskType)
    with pytest.raises(FrozenInstanceError):
        first.__setattr__("entries", ())
    with pytest.raises(FrozenInstanceError):
        SIMPLE_ADAPTER.identity.__setattr__("name", "changed")


def test_registry_rejects_duplicate_key_before_startup() -> None:
    # Given: 默认目录重复附加完全相同的键。
    entries = _registry().entries

    # When/Then: 构造阶段返回精确重复键错误。
    with pytest.raises(DuplicateRuntimeKeyError):
        _ = RuntimeRegistry((*entries, entries[0]))


def test_registry_rejects_mutable_entries_container() -> None:
    # Given: 调用方提供可在构造后变更的列表容器.
    entries = list(_registry().entries)

    # When/Then: 注册表拒绝容器而不是保留可变别名.
    with pytest.raises(InvalidRegistryEntriesError):
        _ = RuntimeRegistry(entries)


def test_registry_rejects_same_task_with_second_version() -> None:
    # Given: 一个任务同时声明 v1 和 v2, 造成批准版本歧义.
    entries = _registry().entries
    v2_profile = replace(
        SIMPLE_V1_PROFILE,
        selection=RuntimeSelection(RuntimeKind.SIMPLE, "v2"),
    )
    ambiguous = replace(
        entries[0],
        key=RuntimeKey(TaskType.SUMMARY, "v2"),
        profile=v2_profile,
    )

    # When/Then: 启动目录拒绝同任务多版本歧义。
    with pytest.raises(DuplicateTaskRegistrationError):
        _ = RuntimeRegistry((*entries, ambiguous))


def test_registry_rejects_missing_task_coverage() -> None:
    # Given: 静态目录遗漏最后一个闭集任务。
    entries = _registry().entries[:-1]

    # When/Then: 构造阶段报告缺失覆盖而非延迟到执行。
    with pytest.raises(MissingTaskCoverageError) as caught:
        _ = RuntimeRegistry(entries)
    assert caught.value.missing == frozenset({TaskType.REVISE})


def test_registry_rejects_adapter_kind_or_profile_version_mismatch() -> None:
    # Given: 条目分别伪装错误适配器族和错误画像版本。
    entries = _registry().entries
    wrong_kind = replace(
        entries[0],
        adapter=DEEP_ADAPTER,
        registered_identity=DEEP_ADAPTER.identity,
    )
    wrong_version = replace(
        entries[0],
        profile=replace(
            SIMPLE_V1_PROFILE,
            selection=RuntimeSelection(RuntimeKind.SIMPLE, "v2"),
        ),
    )

    # When/Then: 两个不一致条目都无法形成可用目录。
    with pytest.raises(InvalidRegistryEntryError):
        _ = RuntimeRegistry((wrong_kind, *entries[1:]))
    with pytest.raises(InvalidRegistryEntryError):
        _ = RuntimeRegistry((wrong_version, *entries[1:]))


@dataclass(frozen=True, slots=True)
class RuntimeKeyImpostor(RuntimeKey):
    """模拟继承合法字段但不属于闭集的运行时键。"""


@dataclass(frozen=True, slots=True)
class RegistryEntryImpostor(RegistryEntry):
    """模拟继承合法字段但不属于闭集的注册条目."""


@dataclass(frozen=True, slots=True)
class RuntimeProfileImpostor(RuntimeProfile):
    """模拟字段合法但不属于闭集的画像子类。"""


def test_registry_rejects_runtime_key_subclass_impostor() -> None:
    # Given: 首个条目携带 RuntimeKey 子类冒充值。
    entries = _registry().entries
    impostor = RuntimeKeyImpostor(TaskType.SUMMARY, VERSION_V1)

    # When/Then: 精确具体类型检查在启动期拒绝冒充键。
    with pytest.raises(InvalidRuntimeKeyError):
        _ = RuntimeRegistry((replace(entries[0], key=impostor), *entries[1:]))


def test_registry_rejects_registry_entry_subclass_impostor() -> None:
    # Given: 首个条目使用字段完全合法的 RegistryEntry 子类.
    entries = _registry().entries
    impostor = RegistryEntryImpostor(
        entries[0].key,
        entries[0].profile,
        entries[0].adapter,
        entries[0].registered_identity,
    )

    # When/Then: 构造阶段返回精确条目类型错误.
    with pytest.raises(InvalidRegistryEntryTypeError):
        _ = RuntimeRegistry((impostor, *entries[1:]))


def test_registry_rejects_runtime_profile_subclass_impostor() -> None:
    # Given: 首个条目携带字段合法的 RuntimeProfile 子类。
    entries = _registry().entries
    source = entries[0].profile
    impostor = RuntimeProfileImpostor(
        source.selection,
        source.capabilities,
        source.bounds,
        source.tool_permissions,
    )

    # When/Then: 注册表在读取画像字段前拒绝冒充类型。
    with pytest.raises(InvalidRegistryEntryError):
        _ = RuntimeRegistry((replace(entries[0], profile=impostor), *entries[1:]))


def test_runtime_key_reuses_t08_version_validation() -> None:
    # Given/When/Then: 非 vN 版本沿用 T08 的类型化版本错误。
    with pytest.raises(InvalidRuntimeVersionError):
        _ = RuntimeKey(TaskType.SUMMARY, "latest")


def test_creation_router_returns_typed_unknown_task() -> None:
    # Given: 创建入口收到 T08 闭集之外的原始任务.
    router = RuntimeRouter(_registry())

    # When/Then: T08 解析失败被收敛为冻结未知目录结果.
    result = router.resolve_for_creation("unknown", VERSION_V1)
    assert result == UnknownTaskType("unknown")


def test_creation_router_returns_typed_unknown_version() -> None:
    # Given: 目录仅批准 summary/v1。
    router = RuntimeRouter(_registry())

    # When/Then: 合法形状但未注册的 v2 不泄漏 KeyError。
    result = router.resolve_for_creation(TaskType.SUMMARY, "v2")
    assert result == UnknownRuntimeVersion(TaskType.SUMMARY, "v2")


def test_creation_router_rejects_malformed_version_with_t08_error() -> None:
    # Given: 一个已知任务和不符合 vN 语法的版本。
    router = RuntimeRouter(_registry())

    # When/Then: 输入错误不被误报为未知任务或未知注册版本。
    with pytest.raises(InvalidRuntimeVersionError):
        _ = router.resolve_for_creation(TaskType.SUMMARY, "latest")


@pytest.mark.parametrize(
    "persisted",
    [
        RuntimeSelection(RuntimeKind.DEEP, VERSION_V1),
        RuntimeSelection(RuntimeKind.SIMPLE, "v2"),
    ],
)
def test_resume_rejects_persisted_runtime_selection_drift(
    persisted: RuntimeSelection,
) -> None:
    # Given: Run 持久化了与当前 summary 路由不同的族或版本。
    run = _persisted_run(persisted)
    router = RuntimeRouter(_registry())

    # When/Then: 恢复返回选择不匹配错误, 绝不静默迁移.
    result = router.resolve_for_resume(run)
    assert result == RuntimeSelectionMismatch(
        TaskType.SUMMARY,
        persisted,
        RuntimeSelection(RuntimeKind.SIMPLE, VERSION_V1),
    )


def test_resume_returns_exact_persisted_selection_when_registry_matches() -> None:
    # Given: Run 持久化了当前批准的 summary/simple/v1 选择。
    persisted = RuntimeSelection(RuntimeKind.SIMPLE, VERSION_V1)
    run = _persisted_run(persisted)

    # When: 恢复路由验证持久化选择。
    route = RuntimeRouter(_registry()).resolve_for_resume(run)

    # Then: 返回值保持原选择, 不创建迁移后的替代选择.
    assert type(route) is RuntimeRoute
    assert route.selection is persisted
    assert route.profile is SIMPLE_V1_PROFILE
