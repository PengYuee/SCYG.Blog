"""冻结运行时画像、能力和最小工具权限测试。"""

from dataclasses import FrozenInstanceError, dataclass, replace
from typing import Final

import pytest

from scyg_agent.domain.runs import RuntimeKind, RuntimeSelection
from scyg_agent.runtimes.profiles import (
    COMPOSE_DEEP_V1_PROFILE,
    DEEP_V1_PROFILE,
    RESEARCH_DEEP_V1_PROFILE,
    REVISE_DEEP_V1_PROFILE,
    SIMPLE_V1_PROFILE,
    InvalidRuntimeProfileError,
    RuntimeBounds,
    RuntimeCapabilities,
    RuntimeProfile,
    ToolPermission,
)

VERSION_V1: Final = "v1"


def test_profiles_declare_closed_capability_matrix_and_least_privilege() -> None:
    # Given/When: 读取 SIMPLE v1 与三个任务级 DEEP v1 画像。
    simple = SIMPLE_V1_PROFILE
    deep = DEEP_V1_PROFILE

    # Then: 能力有界, 工具权限按任务保持最小集合。
    assert simple.capabilities == RuntimeCapabilities(
        streaming=True,
        tools=False,
        interrupts=False,
        checkpoints=False,
        subagents=False,
    )
    assert simple.tool_permissions == frozenset()
    assert deep.capabilities == RuntimeCapabilities(
        streaming=True,
        tools=True,
        interrupts=True,
        checkpoints=True,
        subagents=True,
    )
    assert deep.tool_permissions == frozenset(
        {
            ToolPermission.GET_PUBLISHED_ARTICLE,
            ToolPermission.SEARCH_ARTICLES,
            ToolPermission.CREATE_ARTICLE_DRAFT,
        }
    )
    assert RESEARCH_DEEP_V1_PROFILE.tool_permissions == frozenset(
        {ToolPermission.GET_PUBLISHED_ARTICLE, ToolPermission.SEARCH_ARTICLES}
    )
    assert REVISE_DEEP_V1_PROFILE.tool_permissions == frozenset(
        {
            ToolPermission.GET_PUBLISHED_ARTICLE,
            ToolPermission.SEARCH_ARTICLES,
            ToolPermission.UPDATE_ARTICLE_DRAFT,
            ToolPermission.ADD_ARTICLE_TAGS,
        }
    )
    assert simple.bounds.recursion_limit < deep.bounds.recursion_limit
    assert simple.bounds.context_tokens < deep.bounds.context_tokens
    assert simple.bounds.timeout_seconds < deep.bounds.timeout_seconds


def test_profile_rejects_unbounded_or_inconsistent_capabilities() -> None:
    # Given/When/Then: 无递归上限的边界在构造时稳定失败。
    with pytest.raises(InvalidRuntimeProfileError):
        _ = RuntimeBounds(recursion_limit=0, context_tokens=4096, timeout_seconds=60)

    # Given/When/Then: 没有工具能力却携带权限的画像稳定失败。
    with pytest.raises(InvalidRuntimeProfileError):
        _ = RuntimeProfile(
            selection=RuntimeSelection(RuntimeKind.SIMPLE, VERSION_V1),
            capabilities=RuntimeCapabilities(
                streaming=True,
                tools=False,
                interrupts=False,
                checkpoints=False,
                subagents=False,
            ),
            bounds=RuntimeBounds(4, 4096, 60),
            tool_permissions=frozenset({ToolPermission.SEARCH_ARTICLES}),
        )


class PermissionSetImpostor(frozenset[ToolPermission]):
    """模拟静态兼容但不属于闭集容器的权限集合。"""


@dataclass(frozen=True, slots=True)
class CapabilitiesImpostor(RuntimeCapabilities):
    """模拟字段合法但不属于闭集的能力子类。"""


@dataclass(frozen=True, slots=True)
class BoundsImpostor(RuntimeBounds):
    """模拟字段合法但不属于闭集的边界子类。"""


def test_profile_is_deeply_frozen_and_rejects_permission_container_impostor() -> None:
    # Given: 冻结画像、能力和一个 frozenset 子类冒充值。
    permissions = PermissionSetImpostor({ToolPermission.SEARCH_ARTICLES})

    # When/Then: 字段不可变且画像拒绝非精确不可变容器。
    with pytest.raises(FrozenInstanceError):
        SIMPLE_V1_PROFILE.__setattr__("tool_permissions", frozenset(ToolPermission))
    with pytest.raises(FrozenInstanceError):
        SIMPLE_V1_PROFILE.capabilities.__setattr__("tools", True)
    with pytest.raises(InvalidRuntimeProfileError):
        _ = replace(COMPOSE_DEEP_V1_PROFILE, tool_permissions=permissions)
    with pytest.raises(InvalidRuntimeProfileError):
        _ = replace(
            COMPOSE_DEEP_V1_PROFILE,
            capabilities=CapabilitiesImpostor(
                streaming=True,
                tools=True,
                interrupts=True,
                checkpoints=True,
                subagents=True,
            ),
        )
    with pytest.raises(InvalidRuntimeProfileError):
        _ = replace(COMPOSE_DEEP_V1_PROFILE, bounds=BoundsImpostor(64, 131_072, 900))
