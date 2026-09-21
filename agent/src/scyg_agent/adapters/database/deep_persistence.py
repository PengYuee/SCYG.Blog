"""Deep Runtime 的 PostgreSQL 审批与工具执行复合端口."""

from typing import final

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.ports.interaction_store import (
    InteractionResolution,
    InteractionResolveResult,
)
from scyg_agent.domain.ports.tool_store import (
    ClaimRequest,
    DecisionConflict,
    FirstClaim,
    SemanticIdentityConflict,
    ToolClaimResult,
    ToolFence,
    ToolFenceResult,
    ToolIntent,
    ToolIntentPrepared,
    ToolOperation,
)

from .interaction_store import PostgreSQLInteractionStore
from .tool_store import PostgreSQLToolOperationStore


@final
class PostgreSQLApprovalPersistence:
    """组合既有 T12 store 而不复制 SQL 或状态转换."""

    __slots__ = ("_interactions", "_tools")

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        notification_channel: str,
    ) -> None:
        """让审批与工具操作共享同一个 Agent 会话工厂."""
        self._interactions = PostgreSQLInteractionStore(sessions, notification_channel)
        self._tools = PostgreSQLToolOperationStore(sessions)

    async def resolve(self, request: InteractionResolution) -> InteractionResolveResult:
        """使用现有短事务持久化拒绝或重放决定."""
        return await self._interactions.resolve(request)

    async def resolve_and_prepare(
        self,
        request: InteractionResolution,
        intent: ToolIntent,
    ) -> (
        ToolIntentPrepared | InteractionResolveResult | SemanticIdentityConflict | DecisionConflict
    ):
        """原子解析批准并插入 pending 工具意图."""
        return await self._interactions.resolve_and_prepare(request, intent)

    async def claim(self, request: ClaimRequest) -> ToolClaimResult:
        """通过既有短事务领取或保守恢复工具围栏."""
        return await self._tools.claim(request)

    async def mark_rpc_started(self, fence: ToolFence) -> ToolFenceResult | FirstClaim:
        """在外部调用前持久化 RPC 已开始提交点."""
        return await self._tools.mark_rpc_started(fence)

    async def complete(
        self,
        fence: ToolFence,
        outcome: ToolOperation,
    ) -> ToolFenceResult:
        """使用 token/version 围栏保存唯一终态."""
        return await self._tools.complete(fence, outcome)
