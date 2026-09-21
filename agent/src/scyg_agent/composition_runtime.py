"""生产运行时、认证与共享应用门面组件."""

from contextlib import AsyncExitStack
from datetime import UTC, datetime, timedelta
from typing import final
from uuid import uuid4

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.auth import JwtVerifier, create_jwt_verifier
from scyg_agent.adapters.blog_grpc import BlogGrpcClient
from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.deep_persistence import PostgreSQLApprovalPersistence
from scyg_agent.adapters.database.event_store import NotificationChannel, PostgreSQLEventStore
from scyg_agent.adapters.database.event_subscription import open_listener_connection
from scyg_agent.adapters.database.interaction_store import PostgreSQLInteractionStore
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.adapters.database.run_request_source import (
    PersistedRunRequestSource,
    PostgreSQLRunInputSource,
)
from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.agents.production import LangChainAgentRunner, create_langchain_agent_runner
from scyg_agent.application import ApplicationFacade
from scyg_agent.config import ApplicationSettings
from scyg_agent.domain.ports.command_store import CommandApplied
from scyg_agent.domain.ports.interaction_store import AlreadyResolved, InteractionResolveResult
from scyg_agent.domain.runs import TaskType
from scyg_agent.domain.runs.repository import LeaseToken
from scyg_agent.lifecycle import ComponentDiagnostic
from scyg_agent.runtimes.deep import (
    BlogOutcomeFactory,
    DeepDependencies,
    create_deep_runtime_adapter,
    create_deep_runtime_bindings,
    create_persisted_deep_execution_source,
)
from scyg_agent.runtimes.profiles import deep_profile_for_task
from scyg_agent.runtimes.registry import default_registry
from scyg_agent.runtimes.router import RuntimeRouter
from scyg_agent.runtimes.simple.factory import open_simple_provider
from scyg_agent.runtimes.simple.runtime import SimpleRuntimeAdapter

NOTIFICATION_CHANNEL = "agent_events"


class RuntimeProfileMissingError(RuntimeError):
    """表示固定的 Deep 运行时画像不可用."""


def utc_now() -> datetime:
    """返回 Worker 与 Deep 共享的 UTC 时间."""
    return datetime.now(UTC)


def listener_dsn(settings: ApplicationSettings) -> str:
    """把 SQLAlchemy asyncpg URL 转换为专用 listener DSN."""
    return settings.database_url.get_secret_value().replace(
        "postgresql+asyncpg://", "postgresql://", 1
    )


def _rejection_persisted(result: InteractionResolveResult) -> bool:
    return isinstance(result, (CommandApplied, AlreadyResolved))


@final
class RuntimeFacadeResource:
    """拥有 provider、Blog client、注册表、认证和共享门面."""

    def __init__(
        self,
        settings: ApplicationSettings,
        sessions: async_sessionmaker[AsyncSession],
        checkpoints: CheckpointStore,
    ) -> None:
        """保存已验证配置及借用数据库/检查点资源."""
        self._settings, self.sessions, self._checkpoints = settings, sessions, checkpoints
        self._stack = AsyncExitStack()
        self.agent_runner: LangChainAgentRunner | None = None
        self.facade: ApplicationFacade | None = None
        self.router: RuntimeRouter | None = None
        self.verifier: JwtVerifier | None = None

    @property
    def name(self) -> str:
        """返回脱敏组件名."""
        return "runtime"

    async def start(self) -> None:
        """复用生产 factory 构造三 Deep、Simple、认证与门面."""
        stack = AsyncExitStack()
        runner: LangChainAgentRunner | None = None
        profile = deep_profile_for_task(TaskType.RESEARCH)
        if profile is None:
            raise RuntimeProfileMissingError
        try:
            verifier = create_jwt_verifier(self._settings)
            provider = await stack.enter_async_context(open_simple_provider(self._settings))
            client = await stack.enter_async_context(
                BlogGrpcClient(
                    self._settings.blog_grpc_target.get_secret_value(),
                    profile,
                    float(self._settings.blog_deadline_seconds),
                )
            )
            shared = DeepDependencies(
                profile,
                PostgreSQLApprovalPersistence(self.sessions, NOTIFICATION_CHANNEL),
                client,
                BlogOutcomeFactory(),
                lambda: LeaseToken(uuid4()),
                utc_now,
                timedelta(seconds=self._settings.lease_seconds),
                _rejection_persisted,
            )
            async with self._checkpoints.saver() as saver:
                bindings = create_deep_runtime_bindings(saver, shared)
            deep = create_deep_runtime_adapter(
                create_persisted_deep_execution_source(self.sessions, bindings)
            )
            simple = SimpleRuntimeAdapter(
                provider,
                PersistedRunRequestSource(
                    PostgreSQLRunInputSource(self.sessions), self._settings.provider_model
                ),
            )
            runner = await create_langchain_agent_runner(
                self._settings, self.sessions, self._checkpoints
            )
            router = RuntimeRouter(default_registry(simple, deep))
            runs = PostgreSQLRunRepository(self.sessions)
            events = PostgreSQLEventStore(
                self.sessions,
                listener_dsn(self._settings),
                NotificationChannel.parse(NOTIFICATION_CHANNEL),
                open_listener_connection,
            )
            facade = ApplicationFacade(
                runs,
                events,
                PostgreSQLCommandStore(self.sessions, NOTIFICATION_CHANNEL),
                PostgreSQLInteractionStore(self.sessions, NOTIFICATION_CHANNEL),
            )
            self._stack = stack
            self.agent_runner = runner
            self.router = router
            self.facade = facade
            self.verifier = verifier
            runner = None
        except BaseException:
            if runner is not None:
                await runner.close()
            await stack.aclose()
            self.agent_runner = None
            self.facade = self.router = self.verifier = None
            raise

    async def stop(self) -> None:
        """关闭 AgentRunner, provider 与 Blog client in dependency order."""
        runner = self.agent_runner
        self.agent_runner = None
        try:
            if runner is not None:
                await runner.close()
        finally:
            await self._stack.aclose()
            self.facade = self.router = self.verifier = None

    async def probe(self) -> ComponentDiagnostic:
        """报告完整运行时图是否已发布."""
        ready = (
            self.agent_runner is not None
            and self.facade is not None
            and self.router is not None
            and self.verifier is not None
        )
        return ComponentDiagnostic(self.name, ready, "已就绪" if ready else "未就绪")

    def require_facade(self) -> ApplicationFacade:
        """返回启动后的共享门面."""
        if self.facade is None:
            message = "应用门面尚未启动"
            raise RuntimeError(message)
        return self.facade

    def require_router(self) -> RuntimeRouter:
        """返回启动后的静态路由."""
        if self.router is None:
            message = "运行时路由尚未启动"
            raise RuntimeError(message)
        return self.router

    def require_agent_runner(self) -> LangChainAgentRunner:
        """返回启动后的生产 AgentRunner."""
        if self.agent_runner is None:
            message = "AgentRunner 尚未启动"
            raise RuntimeError(message)
        return self.agent_runner

    def require_verifier(self) -> JwtVerifier:
        """返回启动后的 JWT 验证器."""
        if self.verifier is None:
            message = "认证验证器尚未启动"
            raise RuntimeError(message)
        return self.verifier
