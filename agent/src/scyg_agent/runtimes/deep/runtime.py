"""Deep Runtime 的任务画像校验和图生命周期适配器."""

from dataclasses import dataclass

import anyio
from langchain_core.runnables import RunnableConfig
from langgraph.checkpoint.base import BaseCheckpointSaver

from scyg_agent.adapters.langgraph.values import thread_id_for_run
from scyg_agent.domain.runs import RunId, RuntimeKind, TaskType
from scyg_agent.runtimes.profiles import (
    RuntimeProfile,
    deep_profile_for_task,
)

from .engine import DeepDependencies
from .framework import DeepState, GraphHandle, compile_graph
from .models import (
    GRAPH_NAME,
    GRAPH_VERSION,
    ApprovalReply,
    ApprovalRequired,
    DeepFailureKind,
    DeepGraphInput,
    DeepResult,
    DeepRuntimeError,
)


@dataclass(frozen=True, slots=True)
class DeepRuntime:
    """运行固定版本编译图并验证每次检查点恢复身份."""

    profile: RuntimeProfile
    dependencies: DeepDependencies
    graph: GraphHandle

    @classmethod
    def compile(
        cls,
        profile: RuntimeProfile,
        checkpointer: BaseCheckpointSaver[str],
        dependencies: DeepDependencies,
    ) -> "DeepRuntime":
        """仅为 T14 三个原始 DEEP 画像构建图."""
        if (
            profile
            not in (
                candidate
                for task_type in (TaskType.COMPOSE, TaskType.RESEARCH, TaskType.REVISE)
                if (candidate := deep_profile_for_task(task_type)) is not None
            )
            or profile.selection.kind is not RuntimeKind.DEEP
        ):
            raise DeepRuntimeError(DeepFailureKind.INVALID_INPUT)
        if dependencies.profile is not profile:
            raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
        return cls(profile, dependencies, compile_graph(dependencies, checkpointer, GRAPH_NAME))

    async def start(self, request: DeepGraphInput) -> DeepResult:
        """验证任务和权限后运行到持久审批中断."""
        self._validate_input(request)
        state = DeepState(
            graph_name=GRAPH_NAME,
            graph_version=GRAPH_VERSION,
            task_type=request.task_type.value,
            runtime_version=self.profile.selection.version,
            permissions=tuple(sorted(value.value for value in self.profile.tool_permissions)),
            proposal=request.proposal,
        )
        with anyio.fail_after(self.profile.bounds.timeout_seconds):
            result = await self.graph.start(state, self._config(request.proposal.intent.run_id))
        return self._result(result, interrupted=True)

    async def snapshot(self, run_id: RunId) -> tuple[DeepState, tuple[str, ...]]:
        """向生产持久化源公开已校验的只读检查点。."""
        state, next_nodes = await self.graph.snapshot(self._config(run_id))
        self._validate_state(state)
        return state, next_nodes

    async def pending(self, run_id: RunId) -> ApprovalRequired:
        """读取检查点中的待审批状态而不推进图."""
        state, next_nodes = await self.graph.snapshot(self._config(run_id))
        self._validate_state(state)
        if next_nodes != ("approval",):
            raise DeepRuntimeError(DeepFailureKind.INVALID_CHECKPOINT)
        proposal = state["proposal"]
        return ApprovalRequired(proposal.intent.run_id, proposal.approval_token)

    async def resume(self, run_id: RunId, reply: ApprovalReply) -> DeepResult:
        """以 LangGraph Command 恢复同一持久中断."""
        state, next_nodes = await self.graph.snapshot(self._config(run_id))
        self._validate_state(state)
        if next_nodes != ("approval",):
            return self._result(state, interrupted=False)
        with anyio.fail_after(self.profile.bounds.timeout_seconds):
            resumed = await self.graph.resume(reply, self._config(run_id))
        self._validate_state(resumed)
        return self._result(resumed, interrupted=False)

    async def terminal(self, run_id: RunId) -> DeepResult:
        """恢复已检查点化的终态而不重复节点或工具调用."""
        state, next_nodes = await self.graph.snapshot(self._config(run_id))
        self._validate_state(state)
        if next_nodes:
            raise DeepRuntimeError(DeepFailureKind.INVALID_CHECKPOINT)
        return self._result(state, interrupted=False)

    def _validate_input(self, request: DeepGraphInput) -> None:
        expected = deep_profile_for_task(request.task_type)
        if expected is not self.profile:
            raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
        if request.proposal.intent.tool_name not in {
            permission.value for permission in self.profile.tool_permissions
        }:
            raise DeepRuntimeError(DeepFailureKind.INVALID_INPUT)

    def _validate_state(self, state: DeepState) -> None:
        expected_permissions = tuple(
            sorted(permission.value for permission in self.profile.tool_permissions)
        )
        if (
            state["graph_name"] != GRAPH_NAME
            or state["graph_version"] != GRAPH_VERSION
            or state["runtime_version"] != self.profile.selection.version
            or state["permissions"] != expected_permissions
        ):
            raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)

    @staticmethod
    def _result(state: DeepState, *, interrupted: bool) -> DeepResult:
        proposal = state["proposal"]
        if interrupted:
            return ApprovalRequired(proposal.intent.run_id, proposal.approval_token)
        result = state.get("result")
        if result is None:
            raise DeepRuntimeError(DeepFailureKind.INVALID_CHECKPOINT)
        return result

    @staticmethod
    def _config(run_id: RunId) -> RunnableConfig:
        return {"configurable": {"thread_id": str(thread_id_for_run(run_id))}}
