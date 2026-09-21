"""PostgreSQL 原子 Run 命令应用适配器。."""

from typing import final

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.ports.command_store import (
    CommandApplyResult,
    CommandDataIntegrity,
    CommandRunNotFound,
    CommandSubmission,
    UnsupportedCommand,
    external_command_kind,
)
from scyg_agent.domain.ports.semantic_identity import command_semantic_digest
from scyg_agent.domain.runs import CancelRun, CommandKind
from scyg_agent.domain.runs.commands import command_kind

from .command_application import FirstCommandApplication
from .command_failpoints import NO_COMMAND_FAILPOINT, CommandFailpoint
from .command_replay import replay_command
from .journal_records import CommandRecord
from .run_records import RunRecord


@final
class PostgreSQLCommandStore:
    """锁定 Run 并在一个事务中仲裁公共命令身份。."""

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        notification_channel: str,
        failpoint: CommandFailpoint = NO_COMMAND_FAILPOINT,
    ) -> None:
        """绑定会话及首次命令应用事务编排。."""
        self._sessions = sessions
        self._first_application = FirstCommandApplication(notification_channel, failpoint)

    async def apply(self, request: CommandSubmission) -> CommandApplyResult:
        """竞争命令身份并应用一次领域迁移。."""
        unsupported = _unsupported_external(request)
        if unsupported is not None:
            return unsupported
        async with self._sessions.begin() as session:
            return await self.apply_in_session(session, request)

    async def apply_in_session(
        self, session: AsyncSession, request: CommandSubmission
    ) -> CommandApplyResult:
        """在调用方事务中按 Run、命令身份顺序加锁并应用命令。."""
        unsupported = _unsupported_external(request)
        if unsupported is not None:
            return unsupported
        run_status = (
            await session.execute(
                select(RunRecord.status)
                .where(RunRecord.run_id == str(request.run_id))
                .with_for_update()
            )
        ).scalar_one_or_none()
        if run_status is None:
            return CommandRunNotFound(request.run_id)
        if type(request.command) is CancelRun and run_status == "running":
            return UnsupportedCommand(request.command_id, CommandKind.CANCEL)
        inserted = (
            await session.execute(
                insert(CommandRecord)
                .values(
                    command_id=str(request.command_id),
                    run_id=str(request.run_id),
                    expected_revision=request.expected_revision,
                    sequence=request.expected_sequence,
                    kind=request.kind,
                    request_digest=str(request.request_digest),
                    semantic_digest=str(command_semantic_digest(request)),
                    result_status="pending",
                    result_reference=None,
                    created_at=request.submitted_at,
                    completed_at=None,
                )
                .on_conflict_do_nothing(index_elements=[CommandRecord.command_id])
                .returning(CommandRecord.command_id)
            )
        ).scalar_one_or_none()
        record = (
            await session.execute(
                select(CommandRecord)
                .where(CommandRecord.command_id == str(request.command_id))
                .with_for_update()
            )
        ).scalar_one_or_none()
        if record is None:
            return CommandDataIntegrity(request.command_id)
        if inserted is None:
            return await replay_command(session, record, request)
        return await self._first_application.apply(session, record, request)


def _unsupported_external(request: CommandSubmission) -> UnsupportedCommand | None:
    """在任何 SQL 前拒绝 worker-only 或内部命令。."""
    kind = command_kind(request.command)
    if not isinstance(kind, CommandKind):
        return UnsupportedCommand(request.command_id, CommandKind.CANCEL)
    if external_command_kind(request.command) is None:
        return UnsupportedCommand(request.command_id, kind)
    return None
