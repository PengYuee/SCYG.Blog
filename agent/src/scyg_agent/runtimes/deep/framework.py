# pyright: reportMissingTypeStubs=false, reportUnknownMemberType=false, reportAny=false
"""唯一接触 LangGraph 弱类型值的受审计框架边界."""

from dataclasses import dataclass
from typing import ClassVar, NotRequired, Protocol, TypedDict, cast

from langchain_core.runnables import RunnableConfig
from langgraph.checkpoint.base import BaseCheckpointSaver
from langgraph.graph import END, START, StateGraph
from langgraph.types import Command, interrupt
from pydantic import BaseModel, ConfigDict, ValidationError
from typing_extensions import TypeIs

from .engine import DeepDependencies, approve_and_execute
from .models import (
    ApprovalDecision,
    ApprovalReply,
    DeepFailureKind,
    DeepResult,
    DeepRuntimeError,
    ProposalEnvelope,
    Rejected,
)


class DeepState(TypedDict):
    """定义检查点中的完整确定性图状态."""

    graph_name: str
    graph_version: str
    task_type: str
    runtime_version: str
    permissions: tuple[str, ...]
    proposal: ProposalEnvelope
    decision: NotRequired[str]
    result: NotRequired[DeepResult]


class _GraphProtocol(Protocol):
    async def ainvoke(
        self,
        value: object,  # noqa: RUF100  # noqa: OBJECT_OK
        config: RunnableConfig,  # noqa: RUF100  # noqa: OBJECT_OK
    ) -> object: ...  # noqa: RUF100  # noqa: OBJECT_OK
    async def aget_state(self, config: RunnableConfig) -> object: ...  # noqa: RUF100  # noqa: OBJECT_OK


@dataclass(frozen=True, slots=True)
class _SnapshotProtocol(Protocol):
    @property
    def values(self) -> object: ...  # noqa: RUF100  # noqa: OBJECT_OK
    @property
    def next(self) -> tuple[str, ...]: ...


class _StateBoundary(BaseModel):
    """解析 LangGraph 字典并隐藏失败输入."""

    model_config: ClassVar[ConfigDict] = ConfigDict(
        frozen=True, arbitrary_types_allowed=True, hide_input_in_errors=True
    )
    graph_name: str
    graph_version: str
    task_type: str
    runtime_version: str
    permissions: tuple[str, ...]
    proposal: ProposalEnvelope
    decision: str | None = None
    result: DeepResult | None = None


@dataclass(frozen=True, slots=True)
class GraphHandle:
    """隐藏第三方图对象并只返回已解析状态."""

    _graph: _GraphProtocol

    async def start(self, state: DeepState, config: RunnableConfig) -> DeepState:
        """执行首次输入并解析框架输出."""
        raw: object = await self._graph.ainvoke(state, config)  # noqa: RUF100  # noqa: OBJECT_OK
        return parse_state(raw)

    async def resume(self, reply: ApprovalReply, config: RunnableConfig) -> DeepState:
        """以 Command 恢复并解析框架输出."""
        raw: object = await self._graph.ainvoke(Command(resume=reply), config)  # noqa: RUF100  # noqa: OBJECT_OK
        return parse_state(raw)

    async def snapshot(self, config: RunnableConfig) -> tuple[DeepState, tuple[str, ...]]:
        """读取并解析当前检查点快照."""
        raw: object = await self._graph.aget_state(config)  # noqa: RUF100  # noqa: OBJECT_OK
        if not _is_snapshot(raw):
            raise DeepRuntimeError(DeepFailureKind.INVALID_CHECKPOINT)
        return parse_state(raw.values), raw.next


def compile_graph(
    dependencies: DeepDependencies,
    checkpointer: BaseCheckpointSaver[str],
    graph_name: str,
) -> GraphHandle:
    """编译固定审批、执行和终态节点图."""

    async def approval(state: DeepState) -> DeepState:
        raw: object = interrupt(  # noqa: RUF100  # noqa: OBJECT_OK
            {"kind": "tool_approval", "token": state["proposal"].approval_token}
        )
        reply = _parse_reply(raw)
        if reply.token != state["proposal"].approval_token:
            raise DeepRuntimeError(DeepFailureKind.APPROVAL_CONFLICT)
        return {**state, "decision": reply.decision.value}

    async def execute(state: DeepState) -> DeepState:
        decision_value = state.get("decision")
        if decision_value is None:
            raise DeepRuntimeError(DeepFailureKind.INVALID_CHECKPOINT)
        decision = ApprovalDecision(decision_value)
        match decision:  # noqa: RUF100  # noqa: MATCH_OK - ApprovalDecision 两个分支已完整映射。
            case ApprovalDecision.APPROVE:
                result = await approve_and_execute(
                    state["proposal"], dependencies.profile, dependencies
                )
            case ApprovalDecision.REJECT:
                persisted = await dependencies.persistence.resolve(state["proposal"].resolution)
                if not dependencies.is_rejection_persisted(persisted):
                    raise DeepRuntimeError(DeepFailureKind.APPROVAL_CONFLICT)
                result = Rejected(state["proposal"].intent.run_id)
        return {**state, "result": result}

    builder = StateGraph(DeepState)
    _ = builder.add_node("approval", approval)
    _ = builder.add_node("execute", execute)
    _ = builder.add_edge(START, "approval")
    _ = builder.add_edge("approval", "execute")
    _ = builder.add_edge("execute", END)
    raw_graph: object = builder.compile(checkpointer=checkpointer, name=graph_name)  # noqa: RUF100  # noqa: OBJECT_OK
    if not _is_graph(raw_graph):
        raise DeepRuntimeError(DeepFailureKind.INVALID_INPUT)
    return GraphHandle(cast("_GraphProtocol", cast("object", raw_graph)))  # noqa: RUF100  # noqa: OBJECT_OK


def parse_state(raw: object) -> DeepState:  # noqa: RUF100  # noqa: OBJECT_OK
    """立即把框架字典解析为精确状态."""
    try:
        parsed = _StateBoundary.model_validate(raw)
    except ValidationError:
        raise DeepRuntimeError(DeepFailureKind.INVALID_CHECKPOINT) from None
    state: DeepState = {
        "graph_name": parsed.graph_name,
        "graph_version": parsed.graph_version,
        "task_type": parsed.task_type,
        "runtime_version": parsed.runtime_version,
        "permissions": parsed.permissions,
        "proposal": parsed.proposal,
    }
    if parsed.decision is not None:
        state["decision"] = parsed.decision
    if parsed.result is not None:
        state["result"] = parsed.result
    return state


def _parse_reply(raw: object) -> ApprovalReply:  # noqa: RUF100  # noqa: OBJECT_OK
    if type(raw) is not ApprovalReply:
        raise DeepRuntimeError(DeepFailureKind.INVALID_INPUT)
    return raw


def _is_graph(raw: object) -> TypeIs[_GraphProtocol]:  # noqa: RUF100  # noqa: OBJECT_OK
    return callable(getattr(raw, "ainvoke", None)) and callable(getattr(raw, "aget_state", None))


def _is_snapshot(raw: object) -> TypeIs[_SnapshotProtocol]:  # noqa: RUF100  # noqa: OBJECT_OK
    return hasattr(raw, "values") and type(getattr(raw, "next", None)) is tuple
