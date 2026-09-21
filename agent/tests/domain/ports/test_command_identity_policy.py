from dataclasses import replace
from datetime import UTC, datetime, timedelta

import pytest
from sqlalchemy.ext.asyncio import async_sessionmaker

from scyg_agent.adapters.database.command_results import idempotency_conflict
from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.journal_records import CommandRecord
from scyg_agent.domain.ports.command_store import (
    CommandSubmission,
    ExternalCommandKind,
    UnsupportedCommand,
    external_command_kind,
)
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest
from scyg_agent.domain.ports.semantic_identity import command_semantic_digest
from scyg_agent.domain.runs import (
    CancelRun,
    ClaimRun,
    CommandId,
    CommandKind,
    EventId,
    ExecutionOwnerId,
    FailRun,
    InteractionId,
    ReleaseExecution,
    RequestInput,
    RunCommand,
    RunId,
    SubmitInput,
    SucceedRun,
)

NOW = datetime(2026, 7, 12, 15, tzinfo=UTC)
RUN_ID = RunId("run_t12policy0")
COMMAND_ID = CommandId("cmd_t12policy0")
EVENT_ID = EventId("evt_t12policy0")


@pytest.fixture
def anyio_backend() -> str:
    """使用 asyncio 后端。"""
    return "asyncio"


def test_external_command_catalog_is_explicit_and_closed() -> None:
    # Given: T08 的全部七种命令变体。
    variants = (
        ClaimRun(COMMAND_ID, EVENT_ID, 1, NOW, ExecutionOwnerId("worker_t12policy")),
        RequestInput(COMMAND_ID, EVENT_ID, 1, NOW, InteractionId("int_t12policy0")),
        SubmitInput(COMMAND_ID, EVENT_ID, 1, NOW, InteractionId("int_t12policy0")),
        SucceedRun(COMMAND_ID, EVENT_ID, 1, NOW),
        FailRun(COMMAND_ID, EVENT_ID, 1, NOW),
        CancelRun(COMMAND_ID, EVENT_ID, 1, NOW),
        ReleaseExecution(COMMAND_ID, EVENT_ID, 1, NOW),
    )

    # When/Then: 仅 approval SubmitInput 与受状态限制 Cancel 属于外部目录。
    assert tuple(external_command_kind(value) for value in variants) == (
        None,
        None,
        ExternalCommandKind.SUBMIT_INPUT,
        None,
        None,
        ExternalCommandKind.CANCEL,
        None,
    )


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("command", "kind"),
    [
        (
            ClaimRun(COMMAND_ID, EVENT_ID, 1, NOW, ExecutionOwnerId("worker_t12policy")),
            CommandKind.CLAIM,
        ),
        (
            RequestInput(COMMAND_ID, EVENT_ID, 1, NOW, InteractionId("int_t12policy0")),
            CommandKind.REQUEST_INPUT,
        ),
        (SucceedRun(COMMAND_ID, EVENT_ID, 1, NOW), CommandKind.SUCCEED),
        (FailRun(COMMAND_ID, EVENT_ID, 1, NOW), CommandKind.FAIL),
        (ReleaseExecution(COMMAND_ID, EVENT_ID, 1, NOW), CommandKind.RELEASE_EXECUTION),
    ],
)
async def test_worker_variants_are_rejected_before_session_acquisition(
    command: RunCommand, kind: CommandKind
) -> None:
    # Given: 一个没有绑定数据库的公共命令存储。
    request = _submission(command)
    store = PostgreSQLCommandStore(async_sessionmaker(), "scyg_t12_test")

    # When: worker/internal 命令进入公共入口。
    result = await store.apply(request)

    # Then: 返回冻结 UnsupportedCommand, 未尝试获取连接。
    assert result == UnsupportedCommand(COMMAND_ID, kind)


def test_canonical_identity_changes_for_event_and_time_with_same_caller_digest() -> None:
    # Given: caller digest、kind、run 和 command_id 均相同。
    original = _submission(CancelRun(COMMAND_ID, EVENT_ID, 1, NOW))
    changed_event = replace(
        original,
        command=CancelRun(COMMAND_ID, EventId("evt_t12policy1"), 1, NOW),
    )
    changed_time = replace(
        original,
        submitted_at=NOW + timedelta(seconds=1),
        command=CancelRun(COMMAND_ID, EVENT_ID, 1, NOW + timedelta(seconds=1)),
    )
    record = _record(original)

    # When/Then: 规范摘要变化且持久化比较返回冲突。
    assert command_semantic_digest(original) != command_semantic_digest(changed_event)
    assert idempotency_conflict(record, changed_event) is not None
    assert idempotency_conflict(record, changed_time) is not None


def test_canonical_identity_includes_claim_owner() -> None:
    """验证 worker 命令即使被公共入口拒绝也具有完整规范身份。"""
    # Given: 除执行所有者外完全相同的两个领取命令。
    first = _submission(
        ClaimRun(COMMAND_ID, EVENT_ID, 1, NOW, ExecutionOwnerId("worker_t12policy_a"))
    )
    second = _submission(
        ClaimRun(COMMAND_ID, EVENT_ID, 1, NOW, ExecutionOwnerId("worker_t12policy_b"))
    )

    # When/Then: 所有者参与不可变规范摘要。
    assert command_semantic_digest(first) != command_semantic_digest(second)


def _submission(command: RunCommand) -> CommandSubmission:
    """构造固定公共命令提交。"""
    return CommandSubmission(
        COMMAND_ID,
        RUN_ID,
        1,
        0,
        CommandKind.CANCEL.value,
        RequestDigest.parse("a" * 64),
        NOW,
        command,
        AuditMetadata("source", "test"),
    )


def _record(request: CommandSubmission) -> CommandRecord:
    """构造带规范摘要的命令记录。"""
    return CommandRecord(
        command_id=str(COMMAND_ID),
        run_id=str(RUN_ID),
        expected_revision=1,
        sequence=0,
        kind=request.kind,
        request_digest=str(request.request_digest),
        semantic_digest=str(command_semantic_digest(request)),
        result_status="succeeded",
        result_reference="run:result:revision:2",
        created_at=NOW,
        completed_at=NOW,
    )
