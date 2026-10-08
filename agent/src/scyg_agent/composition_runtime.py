"""Production Recipe AgentRunner and its borrowed application resources."""

from contextlib import AsyncExitStack
from datetime import UTC, datetime
from typing import final

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.blog_grpc import BlogGrpcClient
from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.agents.production import LangChainAgentRunner, create_langchain_agent_runner
from scyg_agent.config import ApplicationSettings
from scyg_agent.lifecycle import ComponentDiagnostic

NOTIFICATION_CHANNEL = "agent_events"


def utc_now() -> datetime:
    """Return the Worker's UTC clock."""
    return datetime.now(UTC)


def listener_dsn(settings: ApplicationSettings) -> str:
    """Convert the SQLAlchemy URL to the dedicated listener DSN."""
    return settings.database_url.get_secret_value().replace(
        "postgresql+asyncpg://", "postgresql://", 1
    )


@final
class AgentRunnerResource:
    """Own the Blog client and the real server-selected Recipe AgentRunner."""

    def __init__(
        self,
        settings: ApplicationSettings,
        sessions: async_sessionmaker[AsyncSession],
        checkpoints: CheckpointStore,
    ) -> None:
        """Borrow application resources for the production Recipe runner lifecycle."""
        self._settings, self._sessions, self._checkpoints = settings, sessions, checkpoints
        self._stack = AsyncExitStack()
        self.agent_runner: LangChainAgentRunner | None = None

    @property
    def name(self) -> str:
        """Return a credential-free lifecycle component name."""
        return "agent_runner"

    async def start(self) -> None:
        """Open the Blog client and compile only production Recipe graphs."""
        stack = AsyncExitStack()
        try:
            client = await stack.enter_async_context(
                BlogGrpcClient(
                    self._settings.blog_grpc_target.get_secret_value(),
                    float(self._settings.blog_deadline_seconds),
                )
            )
            runner = await create_langchain_agent_runner(
                self._settings, self._sessions, self._checkpoints, client
            )
        except BaseException:  # noqa: RUF100  # noqa: BROAD_EXCEPT_OK - Cancellation and startup failure must close acquired resources.
            await stack.aclose()
            raise
        self._stack = stack
        self.agent_runner = runner

    async def stop(self) -> None:
        """Close model clients before their Blog client dependency."""
        runner = self.agent_runner
        self.agent_runner = None
        try:
            if runner is not None:
                await runner.close()
        finally:
            await self._stack.aclose()

    async def probe(self) -> ComponentDiagnostic:
        """Report whether the compiled Recipe runner has been published."""
        ready = self.agent_runner is not None
        return ComponentDiagnostic(self.name, ready, "已就绪" if ready else "未就绪")

    def require_agent_runner(self) -> LangChainAgentRunner:
        """Return the started production runner, never a fallback producer."""
        if self.agent_runner is None:
            message = "AgentRunner 尚未启动"
            raise RuntimeError(message)
        return self.agent_runner
