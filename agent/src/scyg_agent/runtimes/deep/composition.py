"""生产 Deep Runtime 的数据库、检查点和 Blog 客户端组合."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - LangGraph/grpc.aio 生产组合固定 asyncio 后端。
from collections.abc import AsyncIterator, Callable
from contextlib import AsyncExitStack, asynccontextmanager
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from enum import StrEnum
from types import TracebackType
from typing import Protocol, Self
from uuid import uuid4

from sqlalchemy.ext.asyncio import async_sessionmaker

from scyg_agent.adapters.blog_grpc import BlogGrpcClient
from scyg_agent.adapters.database.config import AsyncDatabaseConfig
from scyg_agent.adapters.database.deep_persistence import PostgreSQLApprovalPersistence
from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.adapters.langgraph.values import CheckpointerConfig
from scyg_agent.domain.ports.command_store import CommandApplied
from scyg_agent.domain.ports.interaction_store import AlreadyResolved, InteractionResolveResult
from scyg_agent.domain.runs import RuntimeSelection, TaskType
from scyg_agent.domain.runs.repository import LeaseToken
from scyg_agent.runtimes.profiles import RuntimeProfile, deep_profile_for_task

from .engine import DeepDependencies
from .models import DeepFailureKind, DeepRuntimeError
from .outcomes import BlogOutcomeFactory
from .runtime import DeepRuntime


class EngineResource(Protocol):
    """描述组合关闭所需的最小 Agent engine 表面."""

    async def dispose(self, *, close: bool = True) -> None:
        """释放连接池资源."""
        ...


class CompositionState(StrEnum):
    """限定生产组合资源关闭状态."""

    OPEN = "open"
    CLOSING = "closing"
    CLOSED = "closed"


@dataclass(frozen=True, slots=True)
class DeepCompositionConfig:
    """保存生产组合所需的严格配置和持久选择."""

    database: AsyncDatabaseConfig
    checkpoints: CheckpointerConfig
    task_type: TaskType
    selection: RuntimeSelection
    blog_target: str
    blog_deadline_seconds: float
    notification_channel: str
    lease_duration: timedelta


class DeepRuntimeComposition:
    """拥有可直接使用的运行时及其全部外部资源."""

    __slots__: tuple[str, ...] = (
        "_cleanup_task",
        "_close_lock",
        "_engine",
        "_resources",
        "_state",
        "runtime",
    )

    runtime: DeepRuntime
    _cleanup_task: asyncio.Task[None] | None
    _close_lock: asyncio.Lock
    _engine: EngineResource
    _resources: AsyncExitStack
    _state: CompositionState

    def __init__(
        self,
        runtime: DeepRuntime,
        engine: EngineResource,
        resources: AsyncExitStack,
    ) -> None:
        """接管已经成功打开的资源栈."""
        self.runtime = runtime
        self._engine = engine
        self._cleanup_task = None
        self._close_lock = asyncio.Lock()
        self._resources = resources
        self._state = CompositionState.OPEN

    @classmethod
    async def open(
        cls,
        config: DeepCompositionConfig,
        *,
        token_factory: Callable[[], LeaseToken] = lambda: LeaseToken(uuid4()),
        clock: Callable[[], datetime] = lambda: datetime.now(UTC),
    ) -> Self:
        """先校验静态身份, 再构造隔离 Agent/T13/T16 资源."""
        profile = _validated_profile(config)
        engine = config.database.create_engine()
        async with AsyncExitStack() as stack:
            _ = stack.push_async_callback(engine.dispose)
            checkpoints = await stack.enter_async_context(CheckpointStore(config.checkpoints))
            client = await stack.enter_async_context(
                BlogGrpcClient(config.blog_target, profile, config.blog_deadline_seconds)
            )
            sessions = async_sessionmaker(engine, expire_on_commit=False)
            persistence = PostgreSQLApprovalPersistence(
                sessions,
                config.notification_channel,
            )
            dependencies = DeepDependencies(
                profile,
                persistence,
                client,
                BlogOutcomeFactory(),
                token_factory,
                clock,
                config.lease_duration,
                _rejection_persisted,
            )
            async with checkpoints.saver() as saver:
                runtime = DeepRuntime.compile(profile, saver, dependencies)
            resources = stack.pop_all()
        return cls(runtime, engine, resources)

    async def close(self) -> None:
        """共享并屏蔽唯一清理任务, 成功后才报告 CLOSED."""
        async with self._close_lock:
            if self._state is CompositionState.CLOSED:
                return
            cleanup = self._cleanup_task
            if cleanup is None:
                self._state = CompositionState.CLOSING
                cleanup = asyncio.create_task(self._run_cleanup())
                self._cleanup_task = cleanup
        await asyncio.shield(cleanup)

    async def _run_cleanup(self) -> None:
        """执行唯一资源清理并仅在成功后发布 CLOSED."""
        await self._resources.aclose()
        async with self._close_lock:
            self._state = CompositionState.CLOSED

    async def __aenter__(self) -> Self:
        """返回已经打开的生产组合."""
        return self

    async def __aexit__(
        self,
        _exception_type: type[BaseException] | None,
        _exception: BaseException | None,
        _traceback: TracebackType | None,
    ) -> None:
        """离开作用域时排空并关闭全部资源."""
        await self.close()


def _validated_profile(config: DeepCompositionConfig) -> RuntimeProfile:
    """在任何数据库或 gRPC I/O 前拒绝 T14 选择漂移."""
    profile = deep_profile_for_task(config.task_type)
    if profile is None or profile.selection != config.selection:
        raise DeepRuntimeError(DeepFailureKind.IDENTITY_MISMATCH)
    return profile


def _rejection_persisted(result: InteractionResolveResult) -> bool:
    """只接受 T12 已提交或同语义重放的拒绝结果."""
    return isinstance(result, (CommandApplied, AlreadyResolved))


@asynccontextmanager
async def open_deep_runtime(
    config: DeepCompositionConfig,
) -> AsyncIterator[DeepRuntime]:
    """打开生产组合并只向调用方借出可用运行时."""
    composition = await DeepRuntimeComposition.open(config)
    try:
        yield composition.runtime
    finally:
        await composition.close()
