"""生产 Deep 组合跨真实 T12/T13 PostgreSQL 与本地 gRPC 的验收。"""

from datetime import UTC, datetime, timedelta

import pytest
from pydantic import SecretStr
from sqlalchemy import text
from sqlalchemy.ext.asyncio import async_sessionmaker, create_async_engine

from scyg_agent.adapters.blog_grpc import (
    CorrelationId,
    RequestId,
    RequestIdentity,
    SearchArticles,
)
from scyg_agent.adapters.database.config import AsyncDatabaseConfig
from scyg_agent.adapters.database.run_records import InteractionRecord, RunRecord
from scyg_agent.adapters.langgraph.values import CheckpointerConfig, thread_id_for_run
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.interaction_store import InteractionResolution
from scyg_agent.domain.ports.semantic_identity import interaction_request_digest
from scyg_agent.domain.ports.tool_store import ToolIntent, ToolOperation, ToolOutcomeStatus
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    RunId,
    RuntimeKind,
    RuntimeSelection,
    SubmitInput,
    TaskType,
    ToolCallId,
)
from scyg_agent.runtimes.deep import (
    ApprovalDecision,
    ApprovalReply,
    ApprovalRequired,
    DeepCompositionConfig,
    DeepGraphInput,
    DeepRuntimeComposition,
    ProposalEnvelope,
    ToolFinished,
)
from tests.acceptance_settings import require_test_settings
from tests.adapters.blog_grpc.support import FakeBlog
from tests.adapters.langgraph.conftest import (
    windows_selector_event_loop_policy,
)

NOW = datetime(2026, 7, 12, 16, tzinfo=UTC)
RUN_ID = RunId("run_t17compose0")
INTERACTION_ID = InteractionId("int_t17compose0")
TOOL_CALL_ID = ToolCallId("tool_t17compose0")
OPERATION_ID = OperationId("t17:compose:search")
__all__ = ["windows_selector_event_loop_policy"]
DELETE_CHECKPOINT_SQL = (
    text("DELETE FROM langgraph.checkpoint_writes WHERE thread_id=:thread_id"),
    text("DELETE FROM langgraph.checkpoint_blobs WHERE thread_id=:thread_id"),
    text("DELETE FROM langgraph.checkpoints WHERE thread_id=:thread_id"),
)
TRUNCATE_SQL = """TRUNCATE agent_audit_events, agent_tool_calls, agent_interactions,
agent_commands, agent_events, agent_runs CASCADE"""


@pytest.fixture
def anyio_backend() -> str:
    """生产 PostgreSQL 和 grpc.aio 使用 asyncio 后端。"""
    return "asyncio"


def production_environment(name: str) -> str:
    """读取统一验收配置中的安全端点。"""
    settings = require_test_settings()
    if name in {"SCYG_T12_DATABASE_URL", "SCYG_T13_DATABASE_URL"}:
        return settings.normal_url
    raise ValueError(name)


def production_config(target: str) -> DeepCompositionConfig:
    """构造 Agent/T13 分离连接与 T16 本地目标。"""
    return DeepCompositionConfig(
        database=AsyncDatabaseConfig(SecretStr(production_environment("SCYG_T12_DATABASE_URL"))),
        checkpoints=CheckpointerConfig(
            dsn=SecretStr(production_environment("SCYG_T13_DATABASE_URL"))
        ),
        task_type=TaskType.RESEARCH,
        selection=RuntimeSelection(RuntimeKind.DEEP, "v1"),
        blog_target=target,
        blog_deadline_seconds=2.0,
        notification_channel="scyg_agent_events",
        lease_duration=timedelta(seconds=1),
    )


def production_proposal() -> ProposalEnvelope:
    """构造身份完全一致的审批、意图和 T16 搜索命令。"""
    command_id = CommandId("cmd_t17compose0")
    command = SubmitInput(command_id, EventId("evt_t17compose0"), 1, NOW, INTERACTION_ID)
    submission = CommandSubmission(
        command_id,
        RUN_ID,
        1,
        0,
        "submit_input",
        RequestDigest.parse("c" * 64),
        NOW,
        command,
        AuditMetadata("source", "deep-runtime"),
    )
    resolution = InteractionResolution(
        INTERACTION_ID,
        RequestDigest.parse("d" * 64),
        ResultReference("approval:accepted"),
        submission,
    )
    intent = ToolIntent(
        TOOL_CALL_ID,
        OPERATION_ID,
        RUN_ID,
        "search_articles",
        RequestDigest.parse("e" * 64),
        INTERACTION_ID,
        NOW,
        AuditMetadata("source", "deep-runtime"),
    )
    identity = RequestIdentity(
        RequestId("request-t17-compose"),
        CorrelationId("correlation-t17-compose"),
        RUN_ID,
        TOOL_CALL_ID,
    )
    fallback = ToolOperation(
        TOOL_CALL_ID,
        OPERATION_ID,
        RUN_ID,
        intent.tool_name,
        intent.request_digest,
        ToolOutcomeStatus.FAILED,
        None,
        ResultMetadata("classification", "unstarted"),
        "unstarted",
        NOW,
        intent.audit_metadata,
    )
    return ProposalEnvelope(
        "approval-token-t17-compose",
        intent,
        resolution,
        SearchArticles(identity, "integration", 10),
        fallback,
    )


async def seed_agent_truth(database_url: str) -> None:
    """清理并插入一个等待审批的 DEEP Run。"""
    engine = create_async_engine(database_url)
    try:
        async with engine.begin() as connection:
            _ = await connection.execute(text(TRUNCATE_SQL))

        sessions = async_sessionmaker(engine, expire_on_commit=False)
        async with sessions.begin() as session:
            session.add(
                RunRecord(
                    run_id=str(RUN_ID),
                    owner_user_id="user-t17",
                    operation_id="t17:compose",
                    task_type="research",
                    runtime_kind="deep",
                    runtime_version="v1",
                    revision=1,
                    status="waiting_input",
                    created_at=NOW,
                    updated_at=NOW,
                    attempt=0,
                    next_attempt_at=NOW,
                    lease_owner=None,
                    lease_token=None,
                    lease_expires_at=None,
                    pending_interaction_id=str(INTERACTION_ID),
                    terminal_at=None,
                    terminal_metadata=None,
                    error_code=None,
                    error_message=None,
                    error_metadata=None,
                )
            )
            await session.flush()
            session.add(
                InteractionRecord(
                    interaction_id=str(INTERACTION_ID),
                    run_id=str(RUN_ID),
                    kind="approval",
                    status="pending",
                    requested_at=NOW,
                    request_semantic_digest=str(
                        interaction_request_digest(
                            str(INTERACTION_ID), str(RUN_ID), "approval", NOW.isoformat()
                        )
                    ),
                    resolved_at=None,
                    response_digest=None,
                    resolution_semantic_digest=None,
                    result_reference=None,
                )
            )
    finally:
        await engine.dispose()


async def delete_checkpoint(database: AsyncDatabaseConfig) -> None:
    """仅删除当前 Run 的 T13 执行状态。"""
    engine = database.create_engine()
    thread_id = str(thread_id_for_run(RUN_ID))
    try:
        async with engine.begin() as connection:
            for statement in DELETE_CHECKPOINT_SQL:
                _ = await connection.execute(statement, {"thread_id": thread_id})
    finally:
        await engine.dispose()


@pytest.mark.anyio
async def test_real_production_composition_restart_replay_and_checkpoint_loss(
    local_blog_server: tuple[str, FakeBlog],
) -> None:
    # Given: 真实 Agent/T13 数据库、生产组合和本地 T16 服务。
    target, service = local_blog_server
    agent_url = production_environment("SCYG_T12_DATABASE_URL")
    await seed_agent_truth(agent_url)
    request = DeepGraphInput(TaskType.RESEARCH, production_proposal())
    config = production_config(target)

    # When: 首次运行中断, 重建组合后批准。
    first = await DeepRuntimeComposition.open(config)
    waiting = await first.runtime.start(request)
    assert isinstance(waiting, ApprovalRequired)
    assert service.calls == []
    await first.close()
    restarted = await DeepRuntimeComposition.open(config)
    recovered = await restarted.runtime.pending(RUN_ID)
    assert recovered == waiting
    finished = await restarted.runtime.resume(
        RUN_ID, ApprovalReply(request.proposal.approval_token, ApprovalDecision.APPROVE)
    )
    assert isinstance(finished, ToolFinished)
    assert service.calls == ["search"]
    assert await restarted.runtime.terminal(RUN_ID) == finished
    await restarted.close()

    # Then: 删除 checkpoint 后同一稳定意图只重放 Agent 终态, 不重复 RPC。
    await delete_checkpoint(config.database)
    replayed = await DeepRuntimeComposition.open(config)
    _ = await replayed.runtime.start(request)
    terminal = await replayed.runtime.resume(
        RUN_ID, ApprovalReply(request.proposal.approval_token, ApprovalDecision.APPROVE)
    )
    await replayed.close()
    assert terminal == finished
    assert service.calls == ["search"]
