"""不可变审计事实的纯领域端口。."""

from dataclasses import dataclass
from datetime import datetime
from typing import Protocol

from scyg_agent.domain.runs import CommandId, RunId, ToolCallId, UserId

from .idempotency import AuditMetadata, require_utc


@dataclass(frozen=True, slots=True)
class AuditFact:
    """描述一个仅可追加的清洗审计事实。."""

    audit_id: str
    run_id: RunId
    user_id: UserId
    command_id: CommandId | None
    tool_call_id: ToolCallId | None
    action: str
    outcome: str
    occurred_at: datetime
    metadata: AuditMetadata

    def __post_init__(self) -> None:
        """验证事实时间及至少一个相关身份。."""
        require_utc(self.occurred_at, "occurred_at")
        if not self.audit_id or (self.command_id is None and self.tool_call_id is None):
            raise InvalidAuditFactError


@dataclass(frozen=True, slots=True)
class InvalidAuditFactError(ValueError):
    """拒绝缺少关联身份的审计事实。."""


@dataclass(frozen=True, slots=True)
class StoredAuditFact:
    """返回分配了稳定 Run 内序列的审计事实。."""

    sequence: int
    fact: AuditFact


class AuditStore(Protocol):
    """仅暴露追加能力, 刻意不提供更新或删除。."""

    async def append(self, fact: AuditFact) -> StoredAuditFact:
        """追加一个不可变审计事实。."""
        ...  # pragma: no cover
