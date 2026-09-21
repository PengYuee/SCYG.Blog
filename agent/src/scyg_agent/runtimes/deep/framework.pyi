from typing import NotRequired, TypedDict

from langchain_core.runnables import RunnableConfig
from langgraph.checkpoint.base import BaseCheckpointSaver

from .engine import DeepDependencies
from .models import ApprovalReply, DeepResult, ProposalEnvelope

class DeepState(TypedDict):
    graph_name: str
    graph_version: str
    task_type: str
    runtime_version: str
    permissions: tuple[str, ...]
    proposal: ProposalEnvelope
    decision: NotRequired[str]
    result: NotRequired[DeepResult]

class GraphHandle:
    async def start(self, state: DeepState, config: RunnableConfig) -> DeepState: ...
    async def resume(self, reply: ApprovalReply, config: RunnableConfig) -> DeepState: ...
    async def snapshot(self, config: RunnableConfig) -> tuple[DeepState, tuple[str, ...]]: ...

def compile_graph(
    dependencies: DeepDependencies, checkpointer: BaseCheckpointSaver[str], graph_name: str
) -> GraphHandle: ...
def parse_state(raw: object) -> DeepState: ...
