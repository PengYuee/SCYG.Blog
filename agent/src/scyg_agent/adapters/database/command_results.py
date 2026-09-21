"""命令幂等比较与结果引用编解码。."""

from dataclasses import dataclass
from hashlib import sha256

from scyg_agent.domain.ports.audit_store import AuditFact
from scyg_agent.domain.ports.command_store import (
    CommandRejected,
    CommandSubmission,
    IdempotencyConflict,
)
from scyg_agent.domain.ports.idempotency import ResultReference
from scyg_agent.domain.ports.semantic_identity import command_semantic_digest
from scyg_agent.domain.runs import UserId

from .journal_records import CommandRecord

REJECTION_PART_COUNT = 4


@dataclass(frozen=True, slots=True)
class Rejection:
    """组合一个可持久化并确定重放的拒绝结果。."""

    code: str
    revision: int
    sequence: int


def idempotency_conflict(
    record: CommandRecord, request: CommandSubmission
) -> IdempotencyConflict | None:
    """比较命令身份绑定的全部不可变请求字段。."""
    if (
        record.run_id != str(request.run_id)
        or record.kind != request.kind
        or record.request_digest != str(request.request_digest)
        or record.expected_revision != request.expected_revision
        or record.sequence != request.expected_sequence
        or record.semantic_digest != str(command_semantic_digest(request))
    ):
        return IdempotencyConflict(request.command_id)
    return None


def rejection_reference(rejection: Rejection) -> ResultReference:
    """编码不含请求正文的稳定拒绝引用。."""
    return ResultReference(f"reject:{rejection.code}:{rejection.revision}:{rejection.sequence}")


def replay_rejection(reference: ResultReference) -> CommandRejected:
    """从持久化引用恢复原始拒绝结果。."""
    parts = reference.value.split(":")
    if len(parts) != REJECTION_PART_COUNT or parts[0] != "reject":
        raise CorruptCommandResultError
    return CommandRejected(parts[1], int(parts[2]), int(parts[3]), replayed=True)


class CorruptCommandResultError(RuntimeError):
    """报告无法恢复的命令结果引用。."""


def command_audit_fact(request: CommandSubmission, owner: str, outcome: str) -> AuditFact:
    """构造只含允许列表关联字段的命令审计事实。."""
    return AuditFact(
        _audit_id("command", str(request.command_id)),
        request.run_id,
        UserId(owner),
        request.command_id,
        None,
        request.kind,
        outcome,
        request.submitted_at,
        request.audit_metadata,
    )


def _audit_id(namespace: str, identity: str) -> str:
    """构造跨命名空间且长度受限的稳定审计身份。."""
    digest = sha256(f"{namespace}:{identity}".encode()).hexdigest()
    return f"audit_{namespace[0]}_{digest[:56]}"
