# Used StateGraph surfaces from the installed LangGraph SDK.
# Default v1 invocation remains an object boundary: the SDK does not guarantee
# that its result conforms to OutputT, and interrupted state has extra fields.

from collections.abc import Callable
from typing import Self

from langchain_core.runnables import Runnable, RunnableConfig
from langgraph._internal._typing import StateLike
from langgraph.checkpoint.base import BaseCheckpointSaver
from langgraph.runtime import Runtime
from langgraph.types import Command, StateSnapshot

type _Checkpointer = (
    None | bool | BaseCheckpointSaver[int] | BaseCheckpointSaver[float] | BaseCheckpointSaver[str]
)

class StateGraph[
    StateT: StateLike,
    ContextT: StateLike | None = None,
    InputT: StateLike = StateT,
    OutputT: StateLike = StateT,
]:
    # Native mutable schema attributes preserve invariant generic parameters.
    state_schema: type[StateT]
    context_schema: type[ContextT] | None
    input_schema: type[InputT]
    output_schema: type[OutputT]
    def __init__(
        self,
        state_schema: type[StateT],
        context_schema: type[ContextT] | None = None,
        *,
        input_schema: type[InputT] | None = None,
        output_schema: type[OutputT] | None = None,
    ) -> None: ...
    def add_node(
        self,
        node: str,
        action: (
            Callable[[StateT], object]
            | Callable[[StateT, Runtime[ContextT]], object]
            | Runnable[StateT, object]
        ),
    ) -> Self: ...
    def add_edge(self, start_key: str | list[str], end_key: str) -> Self: ...
    def compile(
        self, checkpointer: _Checkpointer = None
    ) -> CompiledStateGraph[StateT, ContextT, InputT, OutputT]: ...

class CompiledStateGraph[
    StateT: StateLike,
    ContextT: StateLike | None = None,
    InputT: StateLike = StateT,
    OutputT: StateLike = StateT,
]:
    builder: StateGraph[StateT, ContextT, InputT, OutputT]
    async def ainvoke(
        self,
        input: InputT | Command[object] | None,  # noqa: A002 - Preserve SDK keyword API.
        config: RunnableConfig | None = None,
        *,
        context: ContextT | None = None,
    ) -> object: ...
    async def aget_state(
        self, config: RunnableConfig, *, subgraphs: bool = False
    ) -> StateSnapshot: ...
