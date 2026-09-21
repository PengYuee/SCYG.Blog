"""RuntimeAdapter 身份、协议和唯一绑定校验测试。"""

from dataclasses import dataclass, replace

import pytest

from scyg_agent.domain.runs import RuntimeKind
from scyg_agent.runtimes.base import (
    AdapterIdentity,
    InvalidAdapterError,
    validated_runtime_adapter,
)
from scyg_agent.runtimes.registry import RuntimeRegistry, default_registry

from .fakes import FakeRuntimeAdapter, MissingExecuteAdapter, MissingIdentityAdapter


@pytest.mark.parametrize("name", ["", "   ", "pkg.runtime", "pkg/runtime", "x" * 65])
def test_adapter_identity_rejects_unsafe_or_path_like_name(name: str) -> None:
    # Given/When/Then: 非稳定短名称不能成为适配器身份。
    with pytest.raises(InvalidAdapterError):
        _ = AdapterIdentity(name, RuntimeKind.SIMPLE)


@dataclass(frozen=True, slots=True)
class AdapterIdentityImpostor(AdapterIdentity):
    """模拟字段合法但不属于闭集的身份子类。"""


def test_registry_rejects_identity_and_protocol_impostors() -> None:
    # Given: 身份子类适配器和 identity 读取失败的协议冒充对象。
    identity_impostor = FakeRuntimeAdapter(
        AdapterIdentityImpostor("simple-runtime", RuntimeKind.SIMPLE)
    )
    deep = FakeRuntimeAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP))

    # When/Then: 两类无效适配器都返回启动期类型化错误。
    with pytest.raises(InvalidAdapterError):
        _ = default_registry(identity_impostor, deep)
    with pytest.raises(InvalidAdapterError):
        _ = default_registry(MissingIdentityAdapter(), deep)
    with pytest.raises(InvalidAdapterError):
        _ = validated_runtime_adapter(MissingExecuteAdapter())


def test_registry_rejects_duplicate_identity_name_on_different_objects() -> None:
    # Given: SIMPLE 与 DEEP 不同对象错误复用同一稳定名称。
    simple = FakeRuntimeAdapter(AdapterIdentity("shared-runtime", RuntimeKind.SIMPLE))
    deep = FakeRuntimeAdapter(AdapterIdentity("shared-runtime", RuntimeKind.DEEP))

    # When/Then: 名称不能形成歧义的对象或 kind 绑定。
    with pytest.raises(InvalidAdapterError):
        _ = default_registry(simple, deep)


def test_registry_allows_same_instance_for_each_kind_task_group() -> None:
    # Given: 默认目录为每个 kind 注入一个共享实际实例。
    simple = FakeRuntimeAdapter(AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE))
    deep = FakeRuntimeAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP))

    # When: 六个条目由两个实例构造。
    registry = default_registry(simple, deep)

    # Then: 三个 SIMPLE 和三个 DEEP 条目分别共享同一对象。
    assert all(entry.adapter is simple for entry in registry.entries[:3])
    assert all(entry.adapter is deep for entry in registry.entries[3:])


def test_registry_rejects_duplicate_name_on_same_kind_different_objects() -> None:
    # Given: 第二个 SIMPLE 对象冒用首个实例名称。
    first = FakeRuntimeAdapter(AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE))
    second = FakeRuntimeAdapter(AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE))
    deep = FakeRuntimeAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP))
    entries = default_registry(first, deep).entries
    duplicate = replace(entries[1], adapter=second)

    # When/Then: 即使 kind 相同, 名称也只能绑定一个对象。
    with pytest.raises(InvalidAdapterError):
        _ = RuntimeRegistry((entries[0], duplicate, *entries[2:]))
