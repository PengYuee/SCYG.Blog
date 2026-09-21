"""生产 Deep 组合在 RPC 启动后取消时的未知结果验收。"""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - 生产 grpc.aio 取消场景固定 asyncio 后端。
from datetime import datetime, timedelta

import pytest
from pydantic import SecretStr

from scyg_agent.adapters.database.config import AsyncDatabaseConfig
from scyg_agent.domain.runs import TaskType
from scyg_agent.runtimes.deep import (
    ApprovalDecision,
    ApprovalReply,
    DeepGraphInput,
    DeepRuntimeComposition,
    ExecutionInFlight,
    ExternalOutcomeUnknownResult,
)
from tests.adapters.blog_grpc.support import FakeBlog

from .test_production_postgresql import (
    NOW,
    RUN_ID,
    delete_checkpoint,
    production_config,
    production_environment,
    production_proposal,
    seed_agent_truth,
)


@pytest.fixture
def anyio_backend() -> str:
    """生产 PostgreSQL 和 grpc.aio 使用 asyncio 后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_real_production_composition_rpc_started_restart_is_unknown(
    local_blog_server: tuple[str, FakeBlog],
) -> None:
    # Given: 真实组合已经持久化审批中断, 本地 RPC 在响应前阻塞。
    target, service = local_blog_server
    agent_url = production_environment("SCYG_T12_DATABASE_URL")
    await seed_agent_truth(agent_url)
    request = DeepGraphInput(TaskType.RESEARCH, production_proposal())
    config = production_config(target)
    service.block = True
    current_time = [NOW]

    def clock() -> datetime:
        """返回测试控制的确定性领取时间。"""
        return current_time[0]

    composition = await DeepRuntimeComposition.open(config, clock=clock)
    _ = await composition.runtime.start(request)

    # When: RPC 已开始后取消调用方并删除不确定的执行检查点。
    resume = asyncio.create_task(
        composition.runtime.resume(
            RUN_ID,
            ApprovalReply(request.proposal.approval_token, ApprovalDecision.APPROVE),
        )
    )
    await service.started.wait()
    _ = resume.cancel()
    with pytest.raises(asyncio.CancelledError):
        await resume
    await service.drained.wait()
    await composition.close()
    await delete_checkpoint(config.database)

    # Then: 租约存活时只报告执行中, 且不重复发送 RPC。
    live = await DeepRuntimeComposition.open(config, clock=clock)
    _ = await live.runtime.start(request)
    in_flight = await live.runtime.resume(
        RUN_ID, ApprovalReply(request.proposal.approval_token, ApprovalDecision.APPROVE)
    )
    await live.close()
    assert isinstance(in_flight, ExecutionInFlight)
    assert service.calls == ["search"]

    # When: 确定性推进到租约过期后, 再删除执行中 checkpoint。
    current_time[0] = NOW + config.lease_duration + timedelta(microseconds=1)
    await delete_checkpoint(config.database)

    # Then: Agent 持久事实报告外部结果未知, 仍不重复发送 RPC。
    expired = await DeepRuntimeComposition.open(config, clock=clock)
    _ = await expired.runtime.start(request)
    unknown = await expired.runtime.resume(
        RUN_ID, ApprovalReply(request.proposal.approval_token, ApprovalDecision.APPROVE)
    )
    await expired.close()
    assert isinstance(unknown, ExternalOutcomeUnknownResult)
    assert service.calls == ["search"]


@pytest.mark.anyio
async def test_checkpoint_cleanup_database_uses_asyncpg_dialect() -> None:
    # Given: 不包含凭据的已验证 Agent SQLAlchemy 配置。
    database = AsyncDatabaseConfig(SecretStr("postgresql+asyncpg://localhost/test"))

    # When: checkpoint 清理取得与生产组合相同的 engine。
    engine = database.create_engine()
    try:
        # Then: SQLAlchemy 明确选择 asyncpg, 而不是 T13 psycopg DSN。
        assert engine.dialect.driver == "asyncpg"
    finally:
        await engine.dispose()
