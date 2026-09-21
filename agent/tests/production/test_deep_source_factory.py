"""持久化 DeepExecutionSource 与共享工厂契约。"""

from dataclasses import dataclass, replace

import pytest
from langgraph.checkpoint.memory import InMemorySaver

from scyg_agent.domain.runs import Run, RunId, RuntimeKind, RuntimeSelection, TaskType
from scyg_agent.domain.runs.input import MissingRunInput, RunInput
from scyg_agent.runtimes.deep.models import ApprovalDecision, ApprovalReply, ProposalEnvelope
from scyg_agent.runtimes.deep.production_source import (
    DeepRuntimeBinding,
    MissingDeepRunInputError,
    PersistedDeepExecutionSource,
    create_deep_runtime_adapter,
)
from scyg_agent.runtimes.profiles import (
    COMPOSE_DEEP_V1_PROFILE,
    RESEARCH_DEEP_V1_PROFILE,
    REVISE_DEEP_V1_PROFILE,
)
from tests.runtimes.deep.test_runtime import FakeClient, FakePersistence, proposal, runtime
from tests.runtimes.fakes import make_run


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@dataclass(frozen=True, slots=True)
class InputSource:
    missing: bool = False

    async def get_input(self, run_id: RunId) -> RunInput | MissingRunInput:
        if self.missing:
            return MissingRunInput(run_id)
        return RunInput("持久化消息", "article-1")


@dataclass(frozen=True, slots=True)
class ProposalSource:
    value: ProposalEnvelope

    async def proposal_for(self, run: Run, run_input: RunInput) -> ProposalEnvelope:
        assert run_input.initial_message == "持久化消息"
        return replace(self.value, intent=replace(self.value.intent, run_id=run.id))


@dataclass(frozen=True, slots=True)
class ResumeSource:
    async def reply_for(self, run: Run, proposal: ProposalEnvelope) -> ApprovalReply:
        assert proposal.intent.run_id == run.id
        return ApprovalReply(proposal.approval_token, ApprovalDecision.REJECT)


def _source(*, missing: bool = False) -> PersistedDeepExecutionSource:
    research = runtime(InMemorySaver(), FakePersistence(), FakeClient())
    return PersistedDeepExecutionSource(
        InputSource(missing),
        ProposalSource(proposal()),
        ResumeSource(),
        (
            DeepRuntimeBinding(
                TaskType.COMPOSE, replace(research, profile=COMPOSE_DEEP_V1_PROFILE)
            ),
            DeepRuntimeBinding(TaskType.RESEARCH, research),
            DeepRuntimeBinding(TaskType.REVISE, replace(research, profile=REVISE_DEEP_V1_PROFILE)),
        ),
    )


@pytest.mark.anyio
@pytest.mark.parametrize("task_type", [TaskType.COMPOSE, TaskType.RESEARCH, TaskType.REVISE])
async def test_shared_deep_factory_preserves_each_persisted_task_profile(
    task_type: TaskType,
) -> None:
    source = _source()
    adapter = create_deep_runtime_adapter(source)
    run = make_run(task_type, RuntimeSelection(RuntimeKind.DEEP, "v1"))

    request = await source.start_for(run)
    value, reply = await source.resume_for(run)

    assert adapter.source is source
    assert request.task_type is task_type
    assert source.runtime_for(run).profile.selection == run.runtime
    assert value.intent.run_id == run.id
    assert reply.decision is ApprovalDecision.REJECT


@pytest.mark.anyio
async def test_deep_source_historical_missing_input_is_typed_failure() -> None:
    run = make_run(TaskType.RESEARCH, RESEARCH_DEEP_V1_PROFILE.selection)

    with pytest.raises(MissingDeepRunInputError) as captured:
        _ = await _source(missing=True).start_for(run)
    assert str(captured.value) == "Deep Run 缺少可执行的创建输入"


def test_deep_source_rejects_persisted_selection_drift_before_delegation() -> None:
    run = make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v2"))

    with pytest.raises(RuntimeError, match="identity_mismatch"):
        _ = _source().runtime_for(run)
