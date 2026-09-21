"""T14 原生输出族与 T18 唯一分派契约测试."""

from collections.abc import AsyncIterator
from dataclasses import dataclass, replace
from datetime import UTC, datetime

import pytest

from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    CommandId,
    Run,
    RunSucceeded,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
)
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.deep.models import ApprovalRequired
from scyg_agent.runtimes.normalization import (
    NormalizationContext,
    NormalizationError,
    normalize_runtime,
)
from scyg_agent.runtimes.outputs import (
    DeepRuntimeOutput,
    RuntimeNativeOutput,
    SimpleRuntimeOutput,
)
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRoute, RuntimeRouter
from scyg_agent.runtimes.simple.results import CompletionFinished
from tests.runtimes.deep.test_runtime import proposal
from tests.runtimes.fakes import make_run

NOW = datetime(2026, 7, 12, 12, 0, tzinfo=UTC)


@dataclass(frozen=True, slots=True)
class FamilyAdapter:
    """输出注入的应用原生结果并保留直接实例分发."""

    identity: AdapterIdentity
    output: RuntimeNativeOutput

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """输出首次执行结果."""
        _ = run
        yield self.output

    async def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """输出恢复执行结果."""
        _ = run
        yield self.output


@pytest.fixture
def anyio_backend() -> str:
    """固定项目异步测试后端."""
    return "asyncio"


@pytest.mark.anyio
@pytest.mark.parametrize("task_type", tuple(TaskType))
async def test_registered_tasks_route_native_family_and_normalize_once(
    task_type: TaskType,
) -> None:
    # Given: 每个运行时族只有一个实际注册实例和一个 T18 分派入口。
    deep_proposal = proposal()
    simple_output = SimpleRuntimeOutput(CompletionFinished("stop"))
    deep_output = DeepRuntimeOutput(
        deep_proposal,
        None,
        ApprovalRequired(deep_proposal.intent.run_id, deep_proposal.approval_token),
    )
    simple = FamilyAdapter(AdapterIdentity("simple-runtime", RuntimeKind.SIMPLE), simple_output)
    deep = FamilyAdapter(AdapterIdentity("deep-runtime", RuntimeKind.DEEP), deep_output)
    route = RuntimeRouter(default_registry(simple, deep)).resolve_for_creation(task_type, "v1")
    assert type(route) is RuntimeRoute
    run = make_run(task_type, route.selection)
    if route.selection.kind is RuntimeKind.DEEP:
        run = replace(run, id=deep_proposal.intent.run_id)

    # When: Worker 形状先消费原生流, 再调用一次统一规范化。
    outputs = tuple([output async for output in route.execute(run)])
    events = normalize_runtime(run, NormalizationContext(CommandId("cmd_native001"), NOW), outputs)

    # Then: 实例直调不变, 且任务只进入其注册运行时的原生族。
    expected_adapter = simple if route.selection.kind is RuntimeKind.SIMPLE else deep
    expected_output = (
        SimpleRuntimeOutput if route.selection.kind is RuntimeKind.SIMPLE else DeepRuntimeOutput
    )
    assert route.adapter is expected_adapter
    assert type(outputs[0]) is expected_output
    expected_event = (
        RunSucceeded if expected_output is SimpleRuntimeOutput else ApprovalRequiredEvent
    )
    assert type(events[-1]) is expected_event


def test_runtime_normalizer_rejects_empty_mixed_and_multiple_deep_outputs() -> None:
    simple_run = make_run(TaskType.SUMMARY, RuntimeSelection(RuntimeKind.SIMPLE, "v1"))
    context = NormalizationContext(CommandId("cmd_native002"), NOW)
    simple = SimpleRuntimeOutput(CompletionFinished("stop"))
    deep_proposal = proposal()
    deep = DeepRuntimeOutput(
        deep_proposal,
        None,
        ApprovalRequired(deep_proposal.intent.run_id, deep_proposal.approval_token),
    )

    with pytest.raises(NormalizationError, match="不能为空"):
        _ = normalize_runtime(simple_run, context, ())
    with pytest.raises(NormalizationError, match="不能混合"):
        _ = normalize_runtime(simple_run, context, (simple, deep))
    deep_run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=deep_proposal.intent.run_id,
    )
    with pytest.raises(NormalizationError, match="一个原生结果"):
        _ = normalize_runtime(deep_run, context, (deep, deep))
