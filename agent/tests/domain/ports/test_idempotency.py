from datetime import UTC, datetime

import pytest

from scyg_agent.domain.ports.command_store import (
    CommandSubmission,
    InvalidCommandSubmissionError,
)
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.tool_store import (
    InvalidToolOperationError,
    ToolOperation,
    ToolOutcomeStatus,
)
from scyg_agent.domain.runs import (
    CancelRun,
    CommandId,
    EventId,
    OperationId,
    RunId,
    ToolCallId,
)


def test_request_digest_rejects_non_lowercase_sha256() -> None:
    # Given: 大写或长度错误的非规范摘要。
    invalid = ("A" * 64, "a" * 63, "not-a-digest")

    # When/Then: 边界在任何数据库访问前拒绝非法摘要。
    for raw in invalid:
        with pytest.raises(ValueError, match="lowercase SHA-256"):
            _ = RequestDigest.parse(raw)


def test_result_metadata_accepts_only_sanitized_scalar_pairs() -> None:
    # Given: 一组可安全持久化的结果元数据。
    occurred_at = datetime(2026, 7, 12, tzinfo=UTC)

    # When: 构造强类型结果和审计元数据。
    result = ResultMetadata("article_version", "7")
    audit = AuditMetadata("source", "command")

    # Then: 类型保持不可变且不包含原始正文容器。
    assert result.key == "article_version"
    assert audit.value == "command"
    assert occurred_at.utcoffset() == UTC.utcoffset(occurred_at)

    # Then: 空审计键不能越过清洗边界。
    with pytest.raises(ValueError, match="audit metadata"):
        _ = AuditMetadata("", "value")


@pytest.mark.parametrize(
    ("reference", "metadata"),
    [
        ("", ResultMetadata("key", "value")),
        ("contains space", ResultMetadata("key", "value")),
    ],
)
def test_result_reference_rejects_unsafe_values(reference: str, metadata: ResultMetadata) -> None:
    # Given/When/Then: 空值和含空格引用均不能持久化。
    assert metadata.key == "key"
    with pytest.raises(ValueError, match="sanitized reference"):
        _ = ResultReference(reference)


def test_tool_operation_rejects_inconsistent_success_and_failure_fields() -> None:
    # Given: 成功状态却缺少结果引用的非法工具终态。
    with pytest.raises(InvalidToolOperationError):
        _ = ToolOperation(
            ToolCallId("tool_t12invalid"),
            OperationId("t12:invalid"),
            RunId("run_t12invalid0"),
            "update_article",
            RequestDigest.parse("c" * 64),
            ToolOutcomeStatus.SUCCEEDED,
            None,
            ResultMetadata("code", "invalid"),
            None,
            datetime(2026, 7, 12, tzinfo=UTC),
            AuditMetadata("source", "test"),
        )


def test_command_submission_rejects_invalid_fence_and_identity_mismatch() -> None:
    # Given: 一个内部 command_id 与提交身份不同的命令。
    occurred_at = datetime(2026, 7, 12, tzinfo=UTC)
    command = CancelRun(
        CommandId("cmd_t12inner00"),
        EventId("evt_t12invalid0"),
        1,
        occurred_at,
    )

    # When/Then: 非法期望序列和身份不一致均在 I/O 前拒绝。
    with pytest.raises(InvalidCommandSubmissionError):
        _ = CommandSubmission(
            CommandId("cmd_t12outer00"),
            RunId("run_t12invalid0"),
            1,
            -1,
            "cancel",
            RequestDigest.parse("7" * 64),
            occurred_at,
            command,
            AuditMetadata("source", "test"),
        )
    with pytest.raises(InvalidCommandSubmissionError):
        _ = CommandSubmission(
            CommandId("cmd_t12outer00"),
            RunId("run_t12invalid0"),
            1,
            0,
            "cancel",
            RequestDigest.parse("7" * 64),
            occurred_at,
            command,
            AuditMetadata("source", "test"),
        )
