"""Deep 生产组合关闭的取消、并发和逆序保证。"""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - 生产关闭契约明确使用 shield task。
from contextlib import AsyncExitStack

import anyio
import pytest
from langgraph.checkpoint.memory import InMemorySaver

from scyg_agent.runtimes.deep import DeepRuntimeComposition

from .test_runtime import FakeClient, FakePersistence, runtime

CLEANUP_ERROR_MESSAGE = "清理失败"


class TestEngine:
    """满足组合构造所需的最小 engine 资源。"""

    async def dispose(self, *, close: bool = True) -> None:
        """该测试由显式资源栈记录关闭顺序。"""
        _ = close


async def close_step(
    name: str,
    order: list[str],
    started: anyio.Event,
    release: anyio.Event,
) -> None:
    """记录一个可控的资源关闭步骤。"""
    order.append(name)
    started.set()
    await release.wait()


@pytest.fixture
def anyio_backend() -> str:
    """共享 cleanup task 使用 asyncio 后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_cancelled_and_concurrent_close_share_one_reverse_cleanup() -> None:
    # Given: active-work、T16、T13、Agent engine 四个可控逆序步骤。
    graph = runtime(InMemorySaver(), FakePersistence(), FakeClient())
    stack = AsyncExitStack()
    order: list[str] = []
    names = ("engine", "checkpoint", "client", "active-work")
    started = {name: anyio.Event() for name in names}
    releases = {name: anyio.Event() for name in names}
    for name in names:
        _ = stack.push_async_callback(
            close_step,
            name,
            order,
            started[name],
            releases[name],
        )
    composition = DeepRuntimeComposition(graph, TestEngine(), stack.pop_all())

    # When: 第一个等待者在 active-work 清理期间取消。
    first = asyncio.create_task(composition.close())
    await started["active-work"].wait()
    _ = first.cancel()
    with pytest.raises(asyncio.CancelledError):
        await first

    # Then: 第二和第三等待者共享仍在运行的同一清理任务。
    second = asyncio.create_task(composition.close())
    third = asyncio.create_task(composition.close())
    await anyio.sleep(0.001)
    assert not second.done()
    assert not third.done()
    for name in reversed(names):
        releases[name].set()
        if name != "engine":
            await started[names[names.index(name) - 1]].wait()
    await second
    await third
    await composition.close()
    assert order == ["active-work", "client", "checkpoint", "engine"]


@pytest.mark.anyio
async def test_cleanup_exception_is_shared_without_duplicate_cleanup() -> None:
    # Given: 唯一清理步骤稳定失败并记录调用次数。
    graph = runtime(InMemorySaver(), FakePersistence(), FakeClient())
    stack = AsyncExitStack()
    calls: list[str] = []
    started = anyio.Event()
    release = anyio.Event()

    async def fail_cleanup() -> None:
        """记录并抛出可重复观察的清理失败。"""
        calls.append("cleanup")
        started.set()
        await release.wait()
        raise RuntimeError(CLEANUP_ERROR_MESSAGE)

    _ = stack.push_async_callback(fail_cleanup)
    composition = DeepRuntimeComposition(graph, TestEngine(), stack.pop_all())

    # When/Then: 并发及后续关闭均观察同一失败, 清理只执行一次。
    first = asyncio.create_task(composition.close())
    await started.wait()
    second = asyncio.create_task(composition.close())
    release.set()
    results = await asyncio.gather(first, second, return_exceptions=True)
    assert all(isinstance(result, RuntimeError) for result in results)
    with pytest.raises(RuntimeError, match=CLEANUP_ERROR_MESSAGE):
        await composition.close()
    assert calls == ["cleanup"]
