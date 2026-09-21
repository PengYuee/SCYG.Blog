"""Deep 具体生产来源与三项字面量工厂测试。"""

import pytest
from langgraph.checkpoint.memory import InMemorySaver
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.blog_grpc import SearchArticles
from scyg_agent.domain.runs import RuntimeKind, RuntimeSelection, TaskType
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.runtimes.deep.models import DeepGraphInput
from scyg_agent.runtimes.deep.postgresql_source import (
    APPROVAL_ACCEPTED,
    APPROVAL_REJECTED,
    IncompleteDeepTruthError,
    PersistedProposalSource,
    PersistedResumeSource,
    ResumeStateKind,
)
from scyg_agent.runtimes.deep.production_factory import (
    create_deep_runtime_bindings,
    create_persisted_deep_execution_source,
)
from scyg_agent.runtimes.profiles import RESEARCH_DEEP_V1_PROFILE
from tests.adapters.database.scripted_session_support import tool_record
from tests.runtimes.deep.test_runtime import FakeClient, FakePersistence, runtime
from tests.runtimes.fakes import make_run


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


def test_production_binding_inventory_is_three_literal_t14_tasks() -> None:
    shared_runtime = runtime(InMemorySaver(), FakePersistence(), FakeClient())

    bindings = create_deep_runtime_bindings(InMemorySaver(), shared_runtime.dependencies)

    assert tuple(binding.task_type for binding in bindings) == (
        TaskType.COMPOSE,
        TaskType.RESEARCH,
        TaskType.REVISE,
    )
    source = create_persisted_deep_execution_source(async_sessionmaker[AsyncSession](), bindings)
    assert tuple(binding.task_type for binding in source.runtimes) == (
        TaskType.COMPOSE,
        TaskType.RESEARCH,
        TaskType.REVISE,
    )
    assert all(
        binding.runtime.dependencies.persistence is shared_runtime.dependencies.persistence
        for binding in bindings
    )
    source = create_persisted_deep_execution_source(async_sessionmaker[AsyncSession](), bindings)
    assert tuple(binding.task_type for binding in source.runtimes) == (
        TaskType.COMPOSE,
        TaskType.RESEARCH,
        TaskType.REVISE,
    )
    assert all(
        binding.runtime.dependencies.client is shared_runtime.dependencies.client
        for binding in bindings
    )
    assert tuple(binding.runtime.profile.selection for binding in bindings) == (
        RuntimeSelection(RuntimeKind.DEEP, "v1"),
        RuntimeSelection(RuntimeKind.DEEP, "v1"),
        RuntimeSelection(RuntimeKind.DEEP, "v1"),
    )


@pytest.mark.anyio
async def test_concrete_proposal_source_builds_typed_first_proposal_from_run_truth() -> None:
    compiled = runtime(InMemorySaver(), FakePersistence(), FakeClient())
    run = make_run(TaskType.RESEARCH, RESEARCH_DEEP_V1_PROFILE.selection)
    source = PersistedProposalSource(async_sessionmaker[AsyncSession](), lambda _run: compiled)

    proposal = await source.proposal_for(run, RunInput("持久化研究问题", "article-1"))

    assert proposal.intent.run_id == run.id
    assert proposal.intent.tool_name == "search_articles"
    assert proposal.resolution.command.run_id == run.id
    assert isinstance(proposal.command, SearchArticles)
    assert proposal.command.query == "持久化研究问题"


@pytest.mark.anyio
async def test_concrete_proposal_source_reuses_exact_checkpoint_envelope() -> None:
    compiled = runtime(InMemorySaver(), FakePersistence(), FakeClient())
    run = make_run(TaskType.RESEARCH, RESEARCH_DEEP_V1_PROFILE.selection)
    source = PersistedProposalSource(async_sessionmaker[AsyncSession](), lambda _run: compiled)
    first = await source.proposal_for(run, RunInput("持久化研究问题", "article-1"))
    _ = await compiled.start(DeepGraphInput(TaskType.RESEARCH, first))

    recovered = await source.proposal_for(run, RunInput("不同消息", "article-1"))

    assert recovered == first


@pytest.mark.anyio
async def test_concrete_resume_source_closes_decision_and_operation_states() -> None:
    compiled = runtime(InMemorySaver(), FakePersistence(), FakeClient())
    run = make_run(TaskType.RESEARCH, RESEARCH_DEEP_V1_PROFILE.selection)
    proposal_source = PersistedProposalSource(
        async_sessionmaker[AsyncSession](), lambda _run: compiled
    )
    proposal = await proposal_source.proposal_for(run, RunInput("持久化研究问题", "article-1"))
    resume = PersistedResumeSource(async_sessionmaker[AsyncSession](), lambda _run: compiled)

    assert resume.decision_state(proposal, APPROVAL_ACCEPTED).kind is ResumeStateKind.APPROVED
    assert resume.decision_state(proposal, APPROVAL_REJECTED).kind is ResumeStateKind.REJECTED
    with pytest.raises(IncompleteDeepTruthError):
        _ = resume.decision_state(proposal, None)

    operation = tool_record(proposal.fallback_outcome)
    assert resume.operation_state(proposal, operation).kind is ResumeStateKind.TERMINAL
    operation.status = "external_outcome_unknown"
    assert (
        resume.operation_state(proposal, operation).kind is ResumeStateKind.EXTERNAL_OUTCOME_UNKNOWN
    )


@pytest.mark.anyio
async def test_production_source_rejects_non_deep_task_without_discovery() -> None:
    shared_runtime = runtime(InMemorySaver(), FakePersistence(), FakeClient())
    bindings = create_deep_runtime_bindings(InMemorySaver(), shared_runtime.dependencies)
    source = create_persisted_deep_execution_source(async_sessionmaker[AsyncSession](), bindings)
    run = make_run(TaskType.QUESTION, RuntimeSelection(RuntimeKind.DEEP, "v1"))

    with pytest.raises(RuntimeError, match="identity_mismatch"):
        _ = await source.proposals.proposal_for(run, RunInput("不允许动态发现", "article-1"))
