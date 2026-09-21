from datetime import UTC, datetime

import pytest

from scyg_agent.adapters.database.command_failpoints import (
    NO_COMMAND_FAILPOINT,
    CommandStage,
    InjectedCommandFailureError,
)
from scyg_agent.adapters.database.command_results import (
    CorruptCommandResultError,
    Rejection,
    command_audit_fact,
    idempotency_conflict,
    rejection_reference,
    replay_rejection,
)
from scyg_agent.adapters.database.journal_records import CommandRecord
from scyg_agent.domain.ports.command_store import CommandSubmission, IdempotencyConflict
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest, ResultReference
from scyg_agent.domain.ports.semantic_identity import command_semantic_digest
from scyg_agent.domain.runs import CancelRun, CommandId, EventId, RunId

NOW = datetime(2026, 7, 12, 12, tzinfo=UTC)
RUN_ID = RunId("run_t12helper0")
COMMAND_ID = CommandId("cmd_t12helper0")


@pytest.fixture
def anyio_backend() -> str:
    """使用项目已安装的 asyncio 后端。"""
    return "asyncio"


@pytest.mark.anyio
async def test_no_failpoint_accepts_every_transaction_stage() -> None:
    # Given/When: 生产默认故障点观察全部阶段。
    for stage in CommandStage:
        await NO_COMMAND_FAILPOINT.reach(stage)

    # Then: 没有阶段产生测试异常。
    assert len(CommandStage) == 6


def test_rejection_reference_round_trips_original_result() -> None:
    # Given: 一个确定性的 stale revision 拒绝。
    rejection = Rejection("revision_mismatch", 7, 11)

    # When: 编码后再从持久化引用恢复。
    replayed = replay_rejection(rejection_reference(rejection))

    # Then: 原始代码与游标完整保留且标记为重放。
    assert replayed.code == rejection.code
    assert replayed.actual_revision == rejection.revision
    assert replayed.actual_sequence == rejection.sequence
    assert replayed.replayed


def test_rejection_replay_rejects_corrupt_reference() -> None:
    # Given/When/Then: 非拒绝引用不会被误解释为结果。
    with pytest.raises(CorruptCommandResultError):
        _ = replay_rejection(ResultReference("run:valid:revision:2"))


def test_idempotency_comparison_checks_run_kind_and_digest() -> None:
    # Given: 一条绑定到固定请求的持久化命令。
    request = _submission()
    record = _record(request)

    # When/Then: 完全相同请求不冲突。
    assert idempotency_conflict(record, request) is None

    # When/Then: 摘要变化返回类型化冲突。
    changed = _submission(RequestDigest.parse("b" * 64))
    assert idempotency_conflict(record, changed) == IdempotencyConflict(COMMAND_ID)


def test_command_audit_fact_contains_only_correlation_metadata() -> None:
    # Given: 一个已解析的强类型命令请求。
    request = _submission()

    # When: 构造不可变审计事实。
    fact = command_audit_fact(request, "user-t12", "rejected")

    # Then: 事实只携带稳定身份、动作、结果与标量元数据。
    assert fact.command_id == COMMAND_ID
    assert fact.tool_call_id is None
    assert fact.metadata == AuditMetadata("source", "test")
    assert fact.outcome == "rejected"


def test_injected_failure_has_stable_value_free_message() -> None:
    # Given: 一个阶段化测试故障。
    error = InjectedCommandFailureError(CommandStage.EVENT_APPENDED)

    # When/Then: 诊断只暴露阶段名。
    assert str(error) == "injected command failure at event_appended"


def _submission(digest: RequestDigest | None = None) -> CommandSubmission:
    """构造固定身份的取消命令提交。"""
    command = CancelRun(COMMAND_ID, EventId("evt_t12helper0"), 1, NOW)
    return CommandSubmission(
        COMMAND_ID,
        RUN_ID,
        1,
        0,
        "cancel",
        digest or RequestDigest.parse("a" * 64),
        NOW,
        command,
        AuditMetadata("source", "test"),
    )


def _record(request: CommandSubmission) -> CommandRecord:
    """构造无需数据库的持久化命令行。"""
    return CommandRecord(
        command_id=str(request.command_id),
        run_id=str(request.run_id),
        expected_revision=request.expected_revision,
        sequence=request.expected_sequence,
        kind=request.kind,
        request_digest=str(request.request_digest),
        semantic_digest=str(command_semantic_digest(request)),
        result_status="succeeded",
        result_reference="run:result:revision:2",
        created_at=NOW,
        completed_at=NOW,
    )
