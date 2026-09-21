"""将 SIMPLE 提供方结果保持为应用自有原生输出流."""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Final, Protocol, assert_never

from scyg_agent.domain.runs import Run, RuntimeKind, RuntimeSelection
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.outputs import SimpleRuntimeOutput

from .models import CompletionRequest
from .results import CompletionFinished, ProviderDelta, ProviderFailure, ProviderResult

SIMPLE_IDENTITY: Final = AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE)
SIMPLE_SELECTION: Final = RuntimeSelection(RuntimeKind.SIMPLE, "v1")


class CompletionProvider(Protocol):
    """描述 SIMPLE runtime 所需的流式提供方能力."""

    def stream(self, request: CompletionRequest) -> AsyncIterator[ProviderResult]:
        """返回应用自有的类型化增量和终态."""
        ...


class RunRequestSource(Protocol):
    """从持久化 Run 解析业务输入并构造提供方请求."""

    async def request_for(self, run: Run) -> CompletionRequest:
        """返回已完成边界校验的请求."""
        ...


@dataclass(frozen=True, slots=True)
class SimpleRuntimeAdapter:
    """为三个 SIMPLE 任务输出同一闭集原生结果流."""

    provider: CompletionProvider
    request_source: RunRequestSource

    @property
    def identity(self) -> AdapterIdentity:
        """返回注册表可冻结校验的稳定 SIMPLE 身份."""
        return SIMPLE_IDENTITY

    @property
    def selection(self) -> RuntimeSelection:
        """返回适配器实现的固定 SIMPLE v1 选择."""
        return SIMPLE_SELECTION

    def execute(self, run: Run) -> AsyncIterator[SimpleRuntimeOutput]:
        """首次执行一个 SIMPLE Run."""
        return self._run(run)

    def resume(self, run: Run) -> AsyncIterator[SimpleRuntimeOutput]:
        """以同一确定性请求路径恢复 SIMPLE Run."""
        return self._run(run)

    async def _run(self, run: Run) -> AsyncIterator[SimpleRuntimeOutput]:
        """逐帧输出清洗结果, 并在唯一终态后停止."""
        request = await self.request_source.request_for(run)
        async for result in self.provider.stream(request):
            match result:  # noqa: RUF100  # noqa: MATCH_OK - ProviderResult 静态闭集已完整分派。
                case ProviderDelta():
                    yield SimpleRuntimeOutput(result)
                    continue
                case CompletionFinished() | ProviderFailure():
                    yield SimpleRuntimeOutput(result)
                    return
            assert_never(result)
