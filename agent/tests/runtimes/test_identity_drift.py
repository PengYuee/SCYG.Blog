"""注册后适配器 identity 漂移的快照防护测试。"""

from dataclasses import dataclass, replace
from typing import Final

import pytest

from scyg_agent.domain.runs import RuntimeKind, RuntimeSelection, TaskType
from scyg_agent.runtimes.base import AdapterIdentity, AdapterIdentityDrift, InvalidAdapterError
from scyg_agent.runtimes.registry import RuntimeRegistry, default_registry
from scyg_agent.runtimes.router import RuntimeRoute, RuntimeRouter

from .fakes import FakeRuntimeAdapter, MutableRuntimeAdapter, make_run

VERSION_V1: Final = "v1"
SIMPLE_IDENTITY: Final = AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE)
DEEP_ADAPTER: Final = FakeRuntimeAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP))


@dataclass(frozen=True, slots=True)
class AdapterIdentityImpostor(AdapterIdentity):
    """模拟注册后替换为身份子类。"""


@pytest.fixture
def anyio_backend() -> str:
    """固定项目已安装的异步后端。"""
    return "asyncio"


def _route() -> tuple[MutableRuntimeAdapter, RuntimeRouter, RuntimeRoute]:
    """构造带身份快照的 SIMPLE 路由。"""
    simple = MutableRuntimeAdapter(SIMPLE_IDENTITY)
    router = RuntimeRouter(default_registry(simple, DEEP_ADAPTER))
    route = router.resolve_for_creation(TaskType.SUMMARY, VERSION_V1)
    assert type(route) is RuntimeRoute
    return simple, router, route


def test_registry_lookup_rejects_identity_drift_before_route_creation() -> None:
    # Given: 注册完成后替换为另一合法名称和 kind。
    simple = MutableRuntimeAdapter(SIMPLE_IDENTITY)
    router = RuntimeRouter(default_registry(simple, DEEP_ADAPTER))
    simple.identity = AdapterIdentity("changed-runtime", RuntimeKind.DEEP)

    # When/Then: 查找阶段拒绝漂移且没有调用适配器。
    with pytest.raises(AdapterIdentityDrift):
        _ = router.resolve_for_creation(TaskType.SUMMARY, VERSION_V1)
    assert simple.execute_calls == 0
    assert simple.resume_calls == 0


def test_registry_rejects_registered_identity_snapshot_subclass() -> None:
    # Given: 条目携带值相同但类型不属于闭集的身份快照。
    simple = MutableRuntimeAdapter(SIMPLE_IDENTITY)
    entries = default_registry(simple, DEEP_ADAPTER).entries
    impostor = AdapterIdentityImpostor("simple-runtime", RuntimeKind.SIMPLE)

    # When/Then: 注册表在保存前拒绝快照冒充类型。
    with pytest.raises(InvalidAdapterError):
        _ = RuntimeRegistry((replace(entries[0], registered_identity=impostor), *entries[1:]))


@pytest.mark.parametrize(
    "replacement",
    [
        AdapterIdentity("changed-runtime", RuntimeKind.SIMPLE),
        AdapterIdentity("simple-runtime", RuntimeKind.DEEP),
        AdapterIdentityImpostor("simple-runtime", RuntimeKind.SIMPLE),
    ],
)
def test_existing_route_rejects_name_kind_or_type_drift(
    replacement: AdapterIdentity,
) -> None:
    # Given: 已创建路由后替换适配器身份引用。
    simple, _, route = _route()
    simple.identity = replacement
    run = make_run(TaskType.SUMMARY, route.selection)

    # When/Then: 快照不变, 分发前类型化失败且计数保持零。
    with pytest.raises(AdapterIdentityDrift):
        _ = route.execute(run)
    assert route.registered_identity is SIMPLE_IDENTITY
    assert simple.execute_calls == 0


def test_existing_route_rejects_identity_property_failure() -> None:
    # Given: 已创建路由的适配器随后无法读取 identity。
    simple, _, route = _route()
    simple.identity_available = False
    run = make_run(TaskType.SUMMARY, route.selection)

    # When/Then: 不泄漏 AttributeError, 不调用恢复实现。
    with pytest.raises(AdapterIdentityDrift):
        _ = route.resume(run)
    assert simple.resume_calls == 0


@pytest.mark.anyio
async def test_restoring_exact_identity_allows_dispatch_without_changing_snapshot() -> None:
    # Given: 路由观察到漂移后恢复为原精确身份。
    simple, _, route = _route()
    simple.identity = AdapterIdentity("changed-runtime", RuntimeKind.SIMPLE)
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, VERSION_V1))
    with pytest.raises(AdapterIdentityDrift):
        _ = route.execute(run)
    simple.identity = SIMPLE_IDENTITY

    # When: 使用未变化的注册快照再次分发。
    events = [event async for event in route.execute(run)]

    # Then: 仅恢复后产生事件和一次真实调用。
    assert route.registered_identity is SIMPLE_IDENTITY
    assert type(events[0]).__name__ == "SimpleRuntimeOutput"
    assert simple.execute_calls == 1
