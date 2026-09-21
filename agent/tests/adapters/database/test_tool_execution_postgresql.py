from datetime import datetime, timedelta
from uuid import UUID

import anyio
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.operation_records import AuditEventRecord, ToolCallRecord
from scyg_agent.adapters.database.run_records import InteractionRecord
from scyg_agent.adapters.database.tool_store import PostgreSQLToolOperationStore
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.tool_store import (
    ClaimLost,
    ClaimRequest,
    ExistingInFlight,
    ExternalOutcomeUnknown,
    FirstClaim,
    StaleLeaseRecovered,
    TerminalReplay,
    ToolOperation,
    ToolOutcomeStatus,
)
from scyg_agent.domain.runs import OperationId, ToolCallId
from scyg_agent.domain.runs.repository import LeaseToken

from .t12_postgres_support import NOW, RUN_ID, seed_run

OPERATION_ID = OperationId("t17:persistence:tool")
TOOL_CALL_ID = ToolCallId("tool_t17persist0")
LEASE_DURATION = timedelta(minutes=5)


@pytest.mark.anyio
async def test_concurrent_claims_have_one_fenced_winner(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个已提交但尚未执行的工具意图。
    _, sessions = t12_database
    await seed_run(sessions)
    await _seed_pending(sessions)
    store = PostgreSQLToolOperationStore(sessions)
    results: list[FirstClaim | ExistingInFlight] = []

    async def claim(index: int) -> None:
        """使用不同 token 并发竞争同一意图。"""
        result = await store.claim(_claim(index, NOW))
        assert isinstance(result, FirstClaim | ExistingInFlight)
        results.append(result)

    # When: 十六个独立短事务并发领取。
    async with anyio.create_task_group() as task_group:
        for index in range(16):
            _ = task_group.start_soon(claim, index)

    # Then: 恰好一个围栏获胜, 其余观察有效租约。
    assert sum(isinstance(result, FirstClaim) for result in results) == 1
    assert sum(isinstance(result, ExistingInFlight) for result in results) == 15


@pytest.mark.anyio
async def test_exact_expiry_recovers_only_before_rpc_start(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个租约截止点精确等于当前时间的未开始 RPC 意图。
    _, sessions = t12_database
    await seed_run(sessions)
    await _seed_pending(sessions)
    store = PostgreSQLToolOperationStore(sessions)
    first = await store.claim(_claim(1, NOW - LEASE_DURATION))
    assert isinstance(first, FirstClaim)

    # When: 新 token 在精确截止点领取。
    recovered = await store.claim(_claim(2, NOW))

    # Then: 版本单调增加且旧 token 丢失完成权。
    assert isinstance(recovered, StaleLeaseRecovered)
    assert recovered.fence.version == first.fence.version + 1
    lost = await store.complete(first.fence, _success())
    assert lost == ClaimLost(OPERATION_ID)


@pytest.mark.anyio
async def test_stale_post_rpc_becomes_unknown_and_never_replays(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 一个已记录 RPC 开始且随后过期的租约。
    _, sessions = t12_database
    await seed_run(sessions)
    await _seed_pending(sessions)
    store = PostgreSQLToolOperationStore(sessions)
    first = await store.claim(_claim(3, NOW - LEASE_DURATION))
    assert isinstance(first, FirstClaim)
    marked = await store.mark_rpc_started(first.fence)
    assert isinstance(marked, FirstClaim)

    # When: 新 worker 在截止点尝试恢复。
    unknown = await store.claim(_claim(4, NOW))
    replay = await store.claim(_claim(5, NOW + LEASE_DURATION))

    # Then: 首次转换为未知外部结果, 后续只读取同一关闭分类和一条审计。
    assert unknown == replay == ExternalOutcomeUnknown(OPERATION_ID)
    async with sessions() as session:
        count = (
            await session.execute(select(func.count()).select_from(AuditEventRecord))
        ).scalar_one()
        record = (await session.execute(select(ToolCallRecord))).scalar_one()
        assert count == 1
        assert record.status == ToolOutcomeStatus.EXTERNAL_OUTCOME_UNKNOWN.value


@pytest.mark.anyio
async def test_fenced_completion_is_terminal_replay_without_duplicate_audit(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 当前围栏和一个清洗成功终态。
    _, sessions = t12_database
    await seed_run(sessions)
    await _seed_pending(sessions)
    store = PostgreSQLToolOperationStore(sessions)
    claimed = await store.claim(_claim(6, NOW))
    assert isinstance(claimed, FirstClaim)

    # When: 首次完成后相同围栏再次完成。
    first = await store.complete(claimed.fence, _success())
    replay = await store.complete(claimed.fence, _success())

    # Then: 两次都返回原始终态, 但审计只追加一次。
    assert isinstance(first, TerminalReplay)
    assert replay == first
    async with sessions() as session:
        count = (
            await session.execute(select(func.count()).select_from(AuditEventRecord))
        ).scalar_one()
        assert count == 1


async def _seed_pending(sessions: async_sessionmaker[AsyncSession]) -> None:
    """插入满足新约束的调用前工具意图。"""
    async with sessions.begin() as session:
        session.add(
            InteractionRecord(
                interaction_id="int_t17persist0",
                run_id=str(RUN_ID),
                kind="approval",
                status="resolved",
                requested_at=NOW,
                request_semantic_digest="d" * 64,
                resolved_at=NOW,
                response_digest="e" * 64,
                resolution_semantic_digest="c" * 64,
                result_reference="approval:accepted",
            )
        )
        # 外键审批事实必须先落库, 再插入依赖它的工具意图。
        await session.flush()
        session.add(
            ToolCallRecord(
                tool_call_id=str(TOOL_CALL_ID),
                operation_id=str(OPERATION_ID),
                run_id=str(RUN_ID),
                tool_name="update_article",
                status="pending",
                request_digest="a" * 64,
                semantic_digest="0" * 64,
                intent_semantic_digest="b" * 64,
                approval_interaction_id="int_t17persist0",
                approval_resolution_digest="c" * 64,
                request_audit_metadata={"source": "runtime"},
                result_reference=None,
                result_metadata=None,
                error_code=None,
                error_metadata=None,
                started_at=NOW,
                completed_at=None,
                claim_token=None,
                claim_version=0,
                claim_expires_at=None,
                rpc_started_at=None,
            )
        )


def _claim(index: int, now: datetime) -> ClaimRequest:
    """构造确定性 UUID 围栏请求。"""
    return ClaimRequest(
        OPERATION_ID,
        LeaseToken(UUID(int=index + 1)),
        now,
        LEASE_DURATION,
    )


def _success() -> ToolOperation:
    """构造与意图身份一致的清洗成功终态。"""
    return ToolOperation(
        TOOL_CALL_ID,
        OPERATION_ID,
        RUN_ID,
        "update_article",
        RequestDigest.parse("a" * 64),
        ToolOutcomeStatus.SUCCEEDED,
        ResultReference("article:42:version:7"),
        ResultMetadata("article_version", "7"),
        None,
        NOW,
        AuditMetadata("source", "runtime"),
    )
