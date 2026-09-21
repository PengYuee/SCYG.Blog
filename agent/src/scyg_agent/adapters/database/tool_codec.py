"""工具终态记录的强类型解码。."""

from scyg_agent.domain.ports.idempotency import (
    AuditMetadata,
    RequestDigest,
    ResultMetadata,
    ResultReference,
)
from scyg_agent.domain.ports.semantic_identity import tool_semantic_digest
from scyg_agent.domain.ports.tool_store import (
    ToolDataIntegrity,
    ToolOperation,
    ToolOutcomeStatus,
)
from scyg_agent.domain.runs import OperationId, RunId, ToolCallId
from scyg_agent.domain.runs.errors import (
    InvalidIdentifierError,
    InvalidRunError,
    InvalidTimestampError,
)

from .operation_records import ToolCallRecord


def decode_tool_operation(record: ToolCallRecord) -> ToolOperation | ToolDataIntegrity:
    """将受数据库约束保护的记录恢复为强类型终态。."""
    try:
        status = ToolOutcomeStatus(record.status)
    except ValueError:
        return ToolDataIntegrity(None)
    metadata = (
        record.result_metadata if status is ToolOutcomeStatus.SUCCEEDED else record.error_metadata
    )
    if metadata is None or len(metadata) != 1:
        return ToolDataIntegrity(None)
    key, value = next(iter(metadata.items()))
    if len(record.request_audit_metadata) != 1:
        return ToolDataIntegrity(None)
    audit_key, audit_value = next(iter(record.request_audit_metadata.items()))
    try:
        operation = ToolOperation(
            ToolCallId(record.tool_call_id),
            OperationId(record.operation_id),
            RunId(record.run_id),
            record.tool_name,
            RequestDigest.parse(record.request_digest),
            status,
            ResultReference(record.result_reference) if record.result_reference else None,
            ResultMetadata(key, value),
            record.error_code,
            record.completed_at or record.started_at,
            AuditMetadata(audit_key, audit_value),
        )
        if record.semantic_digest != str(tool_semantic_digest(operation)):
            return ToolDataIntegrity(operation.operation_id)
    except (InvalidIdentifierError, InvalidRunError, InvalidTimestampError, ValueError):
        return ToolDataIntegrity(None)
    else:
        return operation
