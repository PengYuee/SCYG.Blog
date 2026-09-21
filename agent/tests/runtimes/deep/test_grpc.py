"""Deep Runtime 通过真实本地 T16 gRPC 的批准路径。"""

from collections.abc import AsyncIterator

import anyio
import pytest
from grpc import aio
from langgraph.checkpoint.memory import InMemorySaver

from scyg_agent.adapters.blog_grpc import BlogFailure, BlogGrpcClient, FailureKind
from scyg_agent.adapters.blog_grpc.contracts import BlogCommand, BlogResult
from scyg_agent.domain.runs import TaskType
from scyg_agent.runtimes.deep import (
    ApprovalDecision,
    ApprovalReply,
    DeepGraphInput,
    ToolFinished,
)
from scyg_agent.runtimes.profiles import RESEARCH_DEEP_V1_PROFILE
from tests.adapters.blog_grpc.support import FakeBlog, add_blog_servicer

from .test_runtime import FakePersistence, proposal, runtime


@pytest.fixture
def anyio_backend() -> str:
    """grpc.aio 与 LangGraph 使用 asyncio 后端。"""
    return "asyncio"


@pytest.fixture
async def local_blog_server() -> AsyncIterator[tuple[str, FakeBlog]]:
    """启动并确定停止本地真实 Blog gRPC 服务。"""
    service = FakeBlog()
    server = aio.server()
    add_blog_servicer(service, server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}", service
    finally:
        await server.stop(None)


@pytest.mark.anyio
async def test_approved_path_uses_real_local_blog_grpc(
    local_blog_server: tuple[str, FakeBlog],
) -> None:
    # Given: 真实本地 grpc.aio 服务和编译图。
    target, service = local_blog_server
    saver = InMemorySaver()
    persistence = FakePersistence()
    request = DeepGraphInput(TaskType.RESEARCH, proposal())
    async with BlogGrpcClient(target, RESEARCH_DEEP_V1_PROFILE, 1.0) as client:
        graph = runtime(saver, persistence, client)
        _ = await graph.start(request)

        # When: 人工批准恢复中断。
        result = await graph.resume(
            request.proposal.intent.run_id,
            ApprovalReply("approval-token-1", ApprovalDecision.APPROVE),
        )

    # Then: T16 真实传输只执行一次且终态持久化。
    assert isinstance(result, ToolFinished)
    assert service.calls == ["search"]
    assert persistence.terminal is not None


class BlockingClient:
    """暴露可观察清理的阻塞 T16 调用。"""

    def __init__(self) -> None:
        self.started: anyio.Event = anyio.Event()
        self.drained: anyio.Event = anyio.Event()

    async def invoke(self, tool_name: str, version: str, command: BlogCommand) -> BlogResult:
        """阻塞到取消并在 finally 中确认任务排空。"""
        _ = (tool_name, version, command)
        self.started.set()
        try:
            await anyio.sleep_forever()
        finally:
            self.drained.set()
        return BlogFailure(FailureKind.INTERNAL, retryable=False)


@pytest.mark.anyio
async def test_caller_timeout_cancels_and_drains_running_tool_call() -> None:
    saver = InMemorySaver()
    persistence = FakePersistence()
    client = BlockingClient()
    request = DeepGraphInput(TaskType.RESEARCH, proposal())
    graph = runtime(saver, persistence, client)
    _ = await graph.start(request)

    # When: 调用方施加更短墙钟超时。
    with pytest.raises(TimeoutError):
        with anyio.fail_after(2.0):
            _ = await graph.resume(
                request.proposal.intent.run_id,
                ApprovalReply("approval-token-1", ApprovalDecision.APPROVE),
            )

    # Then: 取消传播为 TimeoutError 且运行中的工具任务完成清理。
    assert client.started.is_set()
    assert client.drained.is_set()
