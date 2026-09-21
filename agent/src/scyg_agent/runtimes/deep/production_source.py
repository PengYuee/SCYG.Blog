"""从 Agent 持久化真相构造共享 Deep 执行源。."""

from dataclasses import dataclass
from typing import Protocol, override

from scyg_agent.domain.runs import Run, RuntimeKind, TaskType
from scyg_agent.domain.runs.input import MissingRunInput, RunInput, RunInputSource
from scyg_agent.runtimes.profiles import deep_profile_for_task

from .adapter import DeepRuntimeAdapter
from .models import (
    ApprovalReply,
    DeepFailureKind,
    DeepGraphInput,
    DeepRuntimeError,
    ProposalEnvelope,
)
from .runtime import DeepRuntime


class ProposalSource(Protocol):
    """从 Run 输入、交互与操作事实恢复精确提案。."""

    async def proposal_for(self, run: Run, run_input: RunInput) -> ProposalEnvelope:
        """返回已严格解析的提案封套。."""
        ...


class ResumeSource(Protocol):
    """按操作优先于检查点的顺序恢复审批决定。."""

    async def reply_for(self, run: Run, proposal: ProposalEnvelope) -> ApprovalReply:
        """先咨询操作事实,再读取交互或检查点决定。."""
        ...


@dataclass(frozen=True, slots=True)
class DeepRuntimeBinding:
    """绑定一个任务及其固定生产运行时。."""

    task_type: TaskType
    runtime: DeepRuntime


@dataclass(frozen=True, slots=True)
class MissingDeepRunInputError(RuntimeError):
    """阻止执行缺少创建输入的历史 Deep Run。."""

    @override
    def __str__(self) -> str:
        """返回不包含 Run 或输入值的稳定中文错误。."""
        return "Deep Run 缺少可执行的创建输入"


@dataclass(frozen=True, slots=True)
class PersistedDeepExecutionSource:
    """组合既有输入、提案、恢复和运行时真相而不缓存影子状态。."""

    inputs: RunInputSource
    proposals: ProposalSource
    resumes: ResumeSource
    runtimes: tuple[DeepRuntimeBinding, ...]

    def __post_init__(self) -> None:
        """要求三个 DEEP 任务恰好各绑定一个匹配画像。."""
        expected = {TaskType.COMPOSE, TaskType.RESEARCH, TaskType.REVISE}
        actual = {binding.task_type for binding in self.runtimes}
        if actual != expected or len(self.runtimes) != len(expected):
            raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
        for binding in self.runtimes:
            profile = deep_profile_for_task(binding.task_type)
            if profile is None or binding.runtime.profile is not profile:
                raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)

    def runtime_for(self, run: Run) -> DeepRuntime:
        """按持久化任务和完整选择返回精确运行时。."""
        profile = deep_profile_for_task(run.task_type)
        if (
            profile is None
            or run.runtime.kind is not RuntimeKind.DEEP
            or run.runtime != profile.selection
        ):
            raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
        for binding in self.runtimes:
            if binding.task_type is run.task_type:
                return binding.runtime
        raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)

    async def start_for(self, run: Run) -> DeepGraphInput:
        """读取持久化输入并构造任务画像对应的精确图输入。."""
        proposal = await self._proposal_for(run)
        return DeepGraphInput(run.task_type, proposal)

    async def resume_for(self, run: Run) -> tuple[ProposalEnvelope, ApprovalReply]:
        """重建提案并委托操作优先的持久恢复源。."""
        proposal = await self._proposal_for(run)
        return proposal, await self.resumes.reply_for(run, proposal)

    async def _proposal_for(self, run: Run) -> ProposalEnvelope:
        """把历史缺失输入收敛为 Deep 专属类型化失败。."""
        result = await self.inputs.get_input(run.id)
        if isinstance(result, MissingRunInput):
            raise MissingDeepRunInputError
        proposal = await self.proposals.proposal_for(run, result)
        if proposal.intent.run_id != run.id:
            raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
        return proposal


def create_deep_runtime_adapter(source: PersistedDeepExecutionSource) -> DeepRuntimeAdapter:
    """创建供 T14 三个 DEEP 注册项共享的唯一稳定适配器。."""
    return DeepRuntimeAdapter(source)
