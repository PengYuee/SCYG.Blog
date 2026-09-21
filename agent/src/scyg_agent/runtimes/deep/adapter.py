"""把现有 Deep 图运行时接入 T14 的 Run 原生输出协议."""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Protocol

from scyg_agent.domain.runs import Run, RuntimeKind
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.outputs import DeepRuntimeOutput

from .models import ApprovalReply, DeepGraphInput, ProposalEnvelope
from .runtime import DeepRuntime


class DeepExecutionSource(Protocol):
    """为一个已路由 Run 提供任务画像对应的图输入和恢复决定."""

    def runtime_for(self, run: Run) -> DeepRuntime:
        """返回已按 T14 画像组合的实际图运行时."""
        ...

    async def start_for(self, run: Run) -> DeepGraphInput:
        """返回首次执行所需的清洗图输入."""
        ...

    async def resume_for(self, run: Run) -> tuple[ProposalEnvelope, ApprovalReply]:
        """返回恢复时已验证的提案和审批决定."""
        ...


@dataclass(frozen=True, slots=True)
class DeepRuntimeAdapter:
    """为三个 DEEP 任务共享同一直接注册适配器实例."""

    source: DeepExecutionSource

    @property
    def identity(self) -> AdapterIdentity:
        """返回稳定 DEEP 适配器身份."""
        return AdapterIdentity("deep-runtime", RuntimeKind.DEEP)

    async def execute(self, run: Run) -> AsyncIterator[DeepRuntimeOutput]:
        """首次运行到审批中断并输出一个原生结果."""
        request = await self.source.start_for(run)
        result = await self.source.runtime_for(run).start(request)
        yield DeepRuntimeOutput(request.proposal, None, result)

    async def resume(self, run: Run) -> AsyncIterator[DeepRuntimeOutput]:
        """恢复持久审批并输出一个原生结果."""
        proposal, reply = await self.source.resume_for(run)
        result = await self.source.runtime_for(run).resume(run.id, reply)
        yield DeepRuntimeOutput(proposal, reply, result)
