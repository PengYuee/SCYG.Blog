"""首次命令应用的原子事务编排。."""

from typing import final

from sqlalchemy import func, select, update
from sqlalchemy.ext.asyncio import AsyncSession

from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandApplyResult,
    CommandDataIntegrity,
    CommandRejected,
    CommandSubmission,
)
from scyg_agent.domain.ports.event_store import Appended, AppendRequest
from scyg_agent.domain.ports.idempotency import ResultReference
from scyg_agent.domain.runs import (
    IllegalTransition,
    InteractionMismatch,
    RevisionMismatch,
    TimestampRegression,
    Transitioned,
    UnknownVariant,
    transition,
)
from scyg_agent.domain.runs.repository import DataIntegrityError

from .audit_append import append_audit_locked
from .command_failpoints import CommandFailpoint, CommandStage
from .command_results import Rejection, command_audit_fact, rejection_reference
from .event_append import append_events_locked
from .journal_records import CommandRecord, EventRecord
from .run_mapper import map_run
from .run_records import RunRecord


@final
class FirstCommandApplication:
    """在调用方事务中完成首次命令迁移及其原子副作用。."""

    def __init__(self, notification_channel: str, failpoint: CommandFailpoint) -> None:
        """绑定通知通道和事务阶段探针。."""
        self._notification_channel = notification_channel
        self._failpoint = failpoint

    async def apply(
        self, session: AsyncSession, record: CommandRecord, request: CommandSubmission
    ) -> CommandApplyResult:
        """执行首次命令并持久化事件、结果与审计。."""
        run_record = (
            await session.execute(
                select(RunRecord).where(RunRecord.run_id == str(request.run_id)).with_for_update()
            )
        ).scalar_one_or_none()
        if run_record is None:
            return CommandDataIntegrity(request.command_id)
        current_sequence = (
            await session.execute(
                select(func.coalesce(func.max(EventRecord.seq), 0)).where(
                    EventRecord.run_id == str(request.run_id)
                )
            )
        ).scalar_one()
        run = map_run(run_record)
        if isinstance(run, DataIntegrityError):
            return await self._reject(
                session, record, request, Rejection("corrupt_run", 0, current_sequence)
            )
        if current_sequence != request.expected_sequence:
            return await self._reject(
                session,
                record,
                request,
                Rejection("sequence_mismatch", run.revision, current_sequence),
            )
        outcome = transition(run, request.command)
        if type(outcome) is Transitioned:
            updated_run = outcome.run
            _ = await session.execute(
                update(RunRecord)
                .where(RunRecord.run_id == str(request.run_id))
                .values(
                    revision=updated_run.revision,
                    status=updated_run.status.value,
                    updated_at=updated_run.updated_at,
                    lease_owner=(
                        str(updated_run.execution_owner)
                        if updated_run.execution_owner is not None
                        else None
                    ),
                    lease_token=None,
                    lease_expires_at=None,
                    pending_interaction_id=(
                        str(updated_run.pending_interaction_id)
                        if updated_run.pending_interaction_id is not None
                        else None
                    ),
                )
            )
            await self._failpoint.reach(CommandStage.RUN_UPDATED)
            appended = await append_events_locked(
                session,
                AppendRequest(request.run_id, outcome.events),
                self._notification_channel,
            )
            if not isinstance(appended, Appended):
                raise AtomicCommandInvariantError
            await self._failpoint.reach(CommandStage.EVENT_APPENDED)
            reference = ResultReference(f"run:{request.run_id}:revision:{updated_run.revision}")
            record.result_status = "succeeded"
            record.result_reference = reference.value
            record.completed_at = request.submitted_at
            await self._failpoint.reach(CommandStage.COMMAND_COMPLETED)
            await self._audit(session, request, run_record.owner_user_id, "succeeded")
            return CommandApplied(updated_run, appended.events, reference, replayed=False)
        if type(outcome) is RevisionMismatch:
            return await self._reject(
                session,
                record,
                request,
                Rejection("revision_mismatch", outcome.actual_revision, current_sequence),
            )
        if type(outcome) in (
            IllegalTransition,
            InteractionMismatch,
            TimestampRegression,
            UnknownVariant,
        ):
            return await self._reject(
                session,
                record,
                request,
                Rejection(type(outcome).__name__, run.revision, current_sequence),
            )
        raise AtomicCommandInvariantError

    async def _reject(
        self,
        session: AsyncSession,
        record: CommandRecord,
        request: CommandSubmission,
        rejection: Rejection,
    ) -> CommandRejected:
        """持久化可重放拒绝且不修改 Run 或事件。."""
        record.result_status = "rejected"
        record.result_reference = rejection_reference(rejection).value
        record.completed_at = request.submitted_at
        await self._failpoint.reach(CommandStage.COMMAND_COMPLETED)
        owner = (
            await session.execute(
                select(RunRecord.owner_user_id).where(RunRecord.run_id == str(request.run_id))
            )
        ).scalar_one()
        await self._audit(session, request, owner, "rejected")
        return CommandRejected(
            rejection.code, rejection.revision, rejection.sequence, replayed=False
        )

    async def _audit(
        self, session: AsyncSession, request: CommandSubmission, owner: str, outcome: str
    ) -> None:
        """在同一事务追加命令审计事实。."""
        _ = await append_audit_locked(
            session,
            command_audit_fact(request, owner, outcome),
            self._failpoint,
        )


class AtomicCommandInvariantError(RuntimeError):
    """报告同一事务内部出现不可重放状态。."""
