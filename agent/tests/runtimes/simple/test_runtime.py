"""SIMPLE RuntimeAdapter 与静态目录的集成测试。"""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Final

import pytest

from scyg_agent.domain.runs import (
    CommandId,
    DomainEvent,
    EventId,
    Run,
    RunFailed,
    RunSucceeded,
    RuntimeKind,
    TaskType,
)
from scyg_agent.runtimes.outputs import SimpleRuntimeOutput
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRoute, RuntimeRouter
from scyg_agent.runtimes.simple.models import CompletionRequest, Message, MessageRole
from scyg_agent.runtimes.simple.results import (
    CompletionFinished,
    FailureKind,
    ProviderDelta,
    ProviderFailure,
    ProviderResult,
)
from scyg_agent.runtimes.simple.runtime import SimpleRuntimeAdapter
from tests.runtimes.fakes import FakeRuntimeAdapter, make_run

NOW: Final = datetime(2026, 7, 12, 10, 0, tzinfo=UTC)


@dataclass(frozen=True, slots=True)
class RequestSource:
    """按任务构造可观察的类型化请求。"""

    async def request_for(self, run: Run) -> CompletionRequest:
        """将任务类型写入消息以证明三种任务均被分发。"""
        return CompletionRequest(
            model="test-model",
            messages=(Message(role=MessageRole.USER, content=run.task_type.value),),
        )


@dataclass(frozen=True, slots=True)
class EventFactory:
    """为测试构造符合既有领域联合的终态事件。"""

    def succeeded(self, run: Run) -> DomainEvent:
        """构造成功事实。"""
        return RunSucceeded(
            EventId("evt_simple_ok"), CommandId("cmd_simple_ok"), NOW, run.id, run.revision
        )

    def failed(self, run: Run, failure: ProviderFailure) -> DomainEvent:
        """构造失败事实且不携带提供方秘密。"""
        _ = failure
        return RunFailed(
            EventId("evt_simple_fail"), CommandId("cmd_simple_fail"), NOW, run.id, run.revision
        )


class RecordingProvider:
    """记录请求并产生配置的应用自有结果流。"""

    def __init__(self, results: tuple[ProviderResult, ...]) -> None:
        """初始化结果与请求记录。"""
        self.results: tuple[ProviderResult, ...] = results
        self.requests: list[CompletionRequest] = []

    async def stream(self, request: CompletionRequest) -> AsyncIterator[ProviderResult]:
        """记录请求并产生结果。"""
        self.requests.append(request)
        for result in self.results:
            yield result


@pytest.fixture
def anyio_backend() -> str:
    """固定异步后端。"""
    return "asyncio"


@pytest.mark.anyio
@pytest.mark.parametrize("task_type", [TaskType.SUMMARY, TaskType.QUESTION, TaskType.POLISH])
async def test_registry_dispatches_same_real_adapter_for_all_simple_tasks(
    task_type: TaskType,
) -> None:
    # Given: 一个真实 SIMPLE adapter 实例被静态目录共享。
    provider = RecordingProvider((ProviderDelta("内容"), CompletionFinished("stop")))
    adapter = SimpleRuntimeAdapter(provider, RequestSource())
    deep = FakeRuntimeAdapter(adapter.identity.__class__("deep-runtime", RuntimeKind.DEEP))
    route = RuntimeRouter(default_registry(adapter, deep)).resolve_for_creation(task_type, "v1")
    assert type(route) is RuntimeRoute
    run = make_run(task_type, route.selection)

    # When: 通过路由执行对应任务。
    outputs = [output async for output in route.execute(run)]

    # Then: 同一实例收到类型化请求并只产生一个领域终态。
    assert route.adapter is adapter
    assert provider.requests[-1].messages[0].content == task_type.value
    assert [type(output) for output in outputs] == [
        SimpleRuntimeOutput,
        SimpleRuntimeOutput,
    ]


@pytest.mark.anyio
async def test_provider_failure_maps_to_single_failed_domain_event() -> None:
    # Given: 提供方产生一个类型化失败终态。
    provider = RecordingProvider((ProviderFailure(FailureKind.MALFORMED),))
    adapter = SimpleRuntimeAdapter(provider, RequestSource())
    run = make_run(TaskType.SUMMARY, adapter.selection)

    # When: 执行 SIMPLE 任务。
    outputs = [output async for output in adapter.execute(run)]

    # Then: 失败只映射为一个既有领域失败事实。
    assert [type(output) for output in outputs] == [SimpleRuntimeOutput]
