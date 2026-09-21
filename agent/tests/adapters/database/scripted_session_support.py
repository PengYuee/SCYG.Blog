"""无数据库事务辅助测试的类型化脚本会话。"""

from dataclasses import dataclass
from datetime import UTC, datetime

import pytest
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.sql.base import Executable

from scyg_agent.adapters.database.journal_records import CommandRecord, EventRecord
from scyg_agent.adapters.database.operation_records import AuditEventRecord, ToolCallRecord
from scyg_agent.adapters.database.run_records import (
    AgentRunResultRecord,
    InteractionRecord,
    RunRecord,
)
from scyg_agent.domain.ports.audit_store import AuditFact
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.interaction_store import InteractionResolution
from scyg_agent.domain.ports.semantic_identity import (
    command_semantic_digest,
    interaction_request_digest,
    interaction_resolution_digest,
    tool_semantic_digest,
)
from scyg_agent.domain.ports.tool_store import ToolOperation, ToolOutcomeStatus
from scyg_agent.domain.runs import (
    CancelRun,
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    RunId,
    SubmitInput,
    ToolCallId,
    UserId,
)

NOW = datetime(2026, 7, 12, 13, tzinfo=UTC)
RUN_ID = RunId("run_t12script0")
COMMAND_ID = CommandId("cmd_t12script0")
INTERACTION_ID = InteractionId("int_t12script0")
type Scalar = (
    str
    | int
    | RunRecord
    | CommandRecord
    | InteractionRecord
    | ToolCallRecord
    | AgentRunResultRecord
    | None
)
type AddedRecord = EventRecord | AuditEventRecord | AgentRunResultRecord


@dataclass(frozen=True, slots=True)
class FakeResult:
    """提供事务辅助函数实际消费的窄结果表面。"""

    scalar: Scalar = None
    rows: tuple[EventRecord, ...] = ()

    def scalar_one_or_none(self) -> Scalar:
        """返回可空标量。"""
        return self.scalar

    def scalar_one(self) -> Scalar:
        """返回脚本配置的必有标量。"""
        if self.scalar is None:
            raise MissingScriptedScalarError
        return self.scalar

    def scalars(self) -> tuple[EventRecord, ...]:
        """返回脚本配置的 ORM 行。"""
        return self.rows


class ScriptedSession:
    """按顺序提供 SQL 结果并记录新增行和 flush 次数。"""

    def __init__(self, results: list[FakeResult]) -> None:
        """保存按执行顺序排列的结果。"""
        self.results: list[FakeResult] = results
        self.added: list[AddedRecord] = []
        self.flushes: int = 0

    async def execute(self, statement: Executable) -> FakeResult:
        """消费一个预先声明的 SQL 结果。"""
        _ = statement
        if not self.results:
            raise MissingScriptedResultError
        return self.results.pop(0)

    def add(self, record: AddedRecord) -> None:
        """记录 ORM 新增行。"""
        self.added.append(record)

    async def flush(self) -> None:
        """记录调用方要求的事务 flush。"""
        self.flushes += 1


class MissingScriptedResultError(RuntimeError):
    """报告测试脚本缺少 SQL 结果。"""


class MissingScriptedScalarError(RuntimeError):
    """报告测试脚本把必有标量配置为空。"""


def session(monkeypatch: pytest.MonkeyPatch, script: ScriptedSession) -> AsyncSession:
    """把真实 AsyncSession 的窄 I/O 方法替换为类型化脚本。"""
    value = AsyncSession()
    monkeypatch.setattr(value, "execute", script.execute)
    monkeypatch.setattr(value, "add", script.add)
    monkeypatch.setattr(value, "flush", script.flush)
    return value


def submission(digest: RequestDigest | None = None) -> CommandSubmission:
    """构造固定取消命令。"""
    command = CancelRun(COMMAND_ID, EventId("evt_t12script0"), 1, NOW)
    return CommandSubmission(
        COMMAND_ID,
        RUN_ID,
        1,
        0,
        "cancel",
        digest or RequestDigest.parse("d" * 64),
        NOW,
        command,
        AuditMetadata("source", "test"),
    )


def resolution() -> InteractionResolution:
    """构造固定交互解析请求。"""
    command = SubmitInput(COMMAND_ID, EventId("evt_t12script0"), 1, NOW, INTERACTION_ID)
    request = CommandSubmission(
        COMMAND_ID,
        RUN_ID,
        1,
        0,
        "submit_input",
        RequestDigest.parse("e" * 64),
        NOW,
        command,
        AuditMetadata("source", "test"),
    )
    return InteractionResolution(
        INTERACTION_ID,
        RequestDigest.parse("f" * 64),
        ResultReference("approval:accepted"),
        request,
    )


def command_record(request: CommandSubmission, *, completed: bool = False) -> CommandRecord:
    """构造命令 ORM 行。"""
    return CommandRecord(
        command_id=str(request.command_id),
        run_id=str(request.run_id),
        expected_revision=1,
        sequence=0,
        kind=request.kind,
        request_digest=str(request.request_digest),
        semantic_digest=str(command_semantic_digest(request)),
        result_status="succeeded" if completed else "pending",
        result_reference="run:run_t12script0:revision:2" if completed else None,
        created_at=NOW,
        completed_at=NOW if completed else None,
    )


def run(status: str = "pending", interaction: InteractionId | None = None) -> RunRecord:
    """构造最小 Run ORM 行。"""
    return RunRecord(
        run_id=str(RUN_ID),
        owner_user_id="user-t12",
        operation_id="t12:script",
        task_type="summary",
        runtime_kind="simple",
        runtime_version="v1",
        revision=1,
        status=status,
        created_at=NOW,
        updated_at=NOW,
        attempt=0,
        next_attempt_at=NOW,
        lease_owner=None,
        lease_token=None,
        lease_expires_at=None,
        pending_interaction_id=str(interaction) if interaction is not None else None,
        terminal_at=None,
        terminal_metadata=None,
        error_code=None,
        error_message=None,
        error_metadata=None,
    )


def interaction(status: str, reference: str | None) -> InteractionRecord:
    """构造交互 ORM 行。"""
    resolved_request = resolution()
    resolved_digest = interaction_resolution_digest(
        resolved_request.response_digest,
        resolved_request.result_reference.value,
        command_semantic_digest(resolved_request.command),
    )
    return InteractionRecord(
        interaction_id=str(INTERACTION_ID),
        run_id=str(RUN_ID),
        kind="approval",
        status=status,
        requested_at=NOW,
        request_semantic_digest=str(
            interaction_request_digest(
                str(INTERACTION_ID), str(RUN_ID), "approval", NOW.isoformat()
            )
        ),
        resolved_at=NOW if status == "resolved" else None,
        response_digest="f" * 64 if status == "resolved" else None,
        resolution_semantic_digest=str(resolved_digest) if status == "resolved" else None,
        result_reference=reference,
    )


def audit_fact() -> AuditFact:
    """构造固定命令审计事实。"""
    return AuditFact(
        "audit_t12script0",
        RUN_ID,
        UserId("user-t12"),
        COMMAND_ID,
        None,
        "cancel",
        "succeeded",
        NOW,
        AuditMetadata("source", "test"),
    )


def tool_operation(status: ToolOutcomeStatus = ToolOutcomeStatus.SUCCEEDED) -> ToolOperation:
    """构造固定工具成功或失败终态。"""
    succeeded = status is ToolOutcomeStatus.SUCCEEDED
    return ToolOperation(
        ToolCallId("tool_t12script0"),
        OperationId("t12:script:tool"),
        RUN_ID,
        "update_article",
        RequestDigest.parse("9" * 64),
        status,
        ResultReference("article:42:version:7") if succeeded else None,
        ResultMetadata("article_version", "7"),
        None if succeeded else "tool_timeout",
        NOW,
        AuditMetadata("source", "runtime"),
    )


def tool_record(operation: ToolOperation) -> ToolCallRecord:
    """构造与工具终态一致的 ORM 行。"""
    succeeded = operation.status is ToolOutcomeStatus.SUCCEEDED
    return ToolCallRecord(
        tool_call_id=str(operation.tool_call_id),
        operation_id=str(operation.operation_id),
        run_id=str(operation.run_id),
        tool_name=operation.tool_name,
        status=operation.status.value,
        request_digest=str(operation.request_digest),
        semantic_digest=str(tool_semantic_digest(operation)),
        request_audit_metadata={operation.audit_metadata.key: operation.audit_metadata.value},
        result_reference=operation.result_reference.value if operation.result_reference else None,
        result_metadata={operation.metadata.key: operation.metadata.value} if succeeded else None,
        error_code=operation.error_code,
        error_metadata={operation.metadata.key: operation.metadata.value}
        if not succeeded
        else None,
        started_at=NOW,
        completed_at=NOW,
    )
