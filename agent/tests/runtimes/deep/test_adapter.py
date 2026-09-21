"""T17 Deep runtime adapter tests for the T14 native output surface."""

from dataclasses import dataclass, replace

import pytest
from langgraph.checkpoint.memory import InMemorySaver

from scyg_agent.domain.runs import Run, RuntimeKind, RuntimeSelection, TaskType
from scyg_agent.runtimes.deep.adapter import DeepRuntimeAdapter
from scyg_agent.runtimes.deep.models import (
    ApprovalDecision,
    ApprovalReply,
    DeepGraphInput,
    ProposalEnvelope,
    Rejected,
)
from scyg_agent.runtimes.deep.runtime import DeepRuntime
from scyg_agent.runtimes.outputs import DeepRuntimeOutput
from tests.runtimes.deep.test_runtime import (
    FakeClient,
    FakePersistence,
    proposal,
    runtime,
)
from tests.runtimes.fakes import make_run


@dataclass(frozen=True, slots=True)
class Source:
    """Return one compiled runtime and deterministic approval inputs."""

    runtime: DeepRuntime
    proposal: ProposalEnvelope

    def runtime_for(self, run: Run) -> DeepRuntime:
        _ = run
        return self.runtime

    async def start_for(self, run: Run) -> DeepGraphInput:
        return DeepGraphInput(run.task_type, self.proposal)

    async def resume_for(self, run: Run) -> tuple[ProposalEnvelope, ApprovalReply]:
        _ = run
        return (
            self.proposal,
            ApprovalReply(self.proposal.approval_token, ApprovalDecision.REJECT),
        )


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.mark.anyio
async def test_deep_adapter_streams_start_and_resume_native_results() -> None:
    persistence = FakePersistence()
    compiled = runtime(InMemorySaver(), persistence, FakeClient())
    deep_proposal = proposal()
    adapter = DeepRuntimeAdapter(Source(compiled, deep_proposal))
    run = replace(
        make_run(TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1")),
        id=deep_proposal.intent.run_id,
    )

    started = [output async for output in adapter.execute(run)]
    resumed = [output async for output in adapter.resume(run)]

    assert adapter.identity.kind is RuntimeKind.DEEP
    assert len(started) == 1
    assert isinstance(started[0], DeepRuntimeOutput)
    assert started[0].reply is None
    assert len(resumed) == 1
    assert isinstance(resumed[0], DeepRuntimeOutput)
    assert isinstance(resumed[0].result, Rejected)
    assert persistence.rejected
