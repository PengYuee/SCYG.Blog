"""实际 RuntimeAdapter 实例的直接执行与恢复分发测试。"""

from typing import Final

import pytest

from scyg_agent.domain.runs import RuntimeKind, RuntimeSelection, TaskType
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRoute, RuntimeRouter

from .fakes import FakeRuntimeAdapter, make_run

VERSION_V1: Final = "v1"


@pytest.fixture
def anyio_backend() -> str:
    """固定项目已安装的异步后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_creation_route_preserves_and_executes_exact_adapter() -> None:
    # Given: 两个实际协议适配器实例和完整静态目录。
    simple = FakeRuntimeAdapter(AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE))
    deep = FakeRuntimeAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP))
    router = RuntimeRouter(default_registry(simple, deep))

    # When: SIMPLE 与 DEEP 路由直接执行各自适配器。
    simple_route = router.resolve_for_creation(TaskType.SUMMARY, VERSION_V1)
    deep_route = router.resolve_for_creation(TaskType.RESEARCH, VERSION_V1)
    assert type(simple_route) is RuntimeRoute
    assert type(deep_route) is RuntimeRoute
    simple_run = make_run(TaskType.SUMMARY, simple_route.selection)
    deep_run = make_run(TaskType.RESEARCH, deep_route.selection)
    simple_outputs = [output async for output in simple_route.execute(simple_run)]
    deep_outputs = [output async for output in deep_route.execute(deep_run)]

    # Then: 路由保留注入对象身份且事件直接流出。
    assert simple_route.adapter is simple
    assert deep_route.adapter is deep
    assert type(simple_outputs[0]).__name__ == "SimpleRuntimeOutput"
    assert type(deep_outputs[0]).__name__ == "SimpleRuntimeOutput"


@pytest.mark.anyio
async def test_resume_route_preserves_and_resumes_exact_adapter() -> None:
    # Given: 与持久化 SIMPLE/v1 选择一致的 Run。
    simple = FakeRuntimeAdapter(AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE))
    deep = FakeRuntimeAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP))
    run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, VERSION_V1))

    # When: 恢复路由直接调用注册时注入的实例。
    route = RuntimeRouter(default_registry(simple, deep)).resolve_for_resume(run)
    assert type(route) is RuntimeRoute
    outputs = [output async for output in route.resume(run)]

    # Then: 不存在第二次身份映射且恢复事件可见。
    assert route.adapter is simple
    assert type(outputs[0]).__name__ == "SimpleRuntimeOutput"
