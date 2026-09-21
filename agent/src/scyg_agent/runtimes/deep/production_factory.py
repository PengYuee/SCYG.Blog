"""三个 T14 Deep 画像的字面量生产运行时工厂。."""

from dataclasses import replace

from langgraph.checkpoint.base import BaseCheckpointSaver
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.run_request_source import PostgreSQLRunInputSource
from scyg_agent.domain.runs import Run, TaskType
from scyg_agent.runtimes.profiles import deep_profile_for_task

from .engine import DeepDependencies
from .models import DeepFailureKind, DeepRuntimeError
from .postgresql_source import PersistedProposalSource, PersistedResumeSource
from .production_source import (
    DeepRuntimeBinding,
    PersistedDeepExecutionSource,
)
from .runtime import DeepRuntime


def create_deep_runtime_bindings(
    checkpointer: BaseCheckpointSaver[str],
    shared: DeepDependencies,
) -> tuple[DeepRuntimeBinding, DeepRuntimeBinding, DeepRuntimeBinding]:
    """共享持久化/client/时钟资源并编译三个任务专属图。."""
    compose = deep_profile_for_task(TaskType.COMPOSE)
    research = deep_profile_for_task(TaskType.RESEARCH)
    revise = deep_profile_for_task(TaskType.REVISE)
    if compose is None or research is None or revise is None:
        raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
    return (
        DeepRuntimeBinding(
            TaskType.COMPOSE,
            DeepRuntime.compile(compose, checkpointer, replace(shared, profile=compose)),
        ),
        DeepRuntimeBinding(
            TaskType.RESEARCH,
            DeepRuntime.compile(research, checkpointer, replace(shared, profile=research)),
        ),
        DeepRuntimeBinding(
            TaskType.REVISE,
            DeepRuntime.compile(revise, checkpointer, replace(shared, profile=revise)),
        ),
    )


def create_persisted_deep_execution_source(
    sessions: async_sessionmaker[AsyncSession],
    bindings: tuple[DeepRuntimeBinding, DeepRuntimeBinding, DeepRuntimeBinding],
) -> PersistedDeepExecutionSource:
    """用具体 PostgreSQL/checkpoint 来源创建 T21 可直接注入的执行源。."""

    def runtime_for(run: Run) -> DeepRuntime:
        for binding in bindings:
            if binding.task_type is run.task_type:
                return binding.runtime
        raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)

    proposals = PersistedProposalSource(sessions, runtime_for)
    resumes = PersistedResumeSource(sessions, runtime_for)
    return PersistedDeepExecutionSource(
        PostgreSQLRunInputSource(sessions),
        proposals,
        resumes,
        bindings,
    )
