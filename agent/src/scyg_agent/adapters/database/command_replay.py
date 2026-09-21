"""调用方事务内的命令原始结果重放。."""

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandApplyResult,
    CommandDataIntegrity,
    CommandRejected,
    CommandSubmission,
)
from scyg_agent.domain.ports.event_store import EventCursor, StoredEvent
from scyg_agent.domain.ports.idempotency import ResultReference
from scyg_agent.domain.runs.repository import DataIntegrityError

from .command_results import idempotency_conflict, replay_rejection
from .event_codec import deserialize_event
from .journal_records import CommandRecord, EventRecord
from .run_mapper import map_run
from .run_records import RunRecord


async def replay_command(
    session: AsyncSession, record: CommandRecord, request: CommandSubmission
) -> CommandApplyResult:
    """验证不可变请求并返回首次事务保存的结果。."""
    conflict = idempotency_conflict(record, request)
    if conflict is not None:
        return conflict
    reference_result = _reference_result(record, request)
    if isinstance(reference_result, CommandDataIntegrity | CommandRejected):
        return reference_result
    return await _replay_success(session, record, request, reference_result)


def _reference_result(
    record: CommandRecord, request: CommandSubmission
) -> ResultReference | CommandRejected | CommandDataIntegrity:
    """解析持久化结果引用并包含格式损坏。."""
    if record.result_reference is None:
        return CommandDataIntegrity(request.command_id)
    try:
        reference = ResultReference(record.result_reference)
        if record.result_status == "rejected":
            return replay_rejection(reference)
    except (ValueError, RuntimeError):
        return CommandDataIntegrity(request.command_id)
    return reference


async def _replay_success(
    session: AsyncSession,
    record: CommandRecord,
    request: CommandSubmission,
    result_reference: ResultReference,
) -> CommandApplyResult:
    """恢复成功命令的聚合及事件引用。."""
    run_record = (
        await session.execute(select(RunRecord).where(RunRecord.run_id == record.run_id))
    ).scalar_one_or_none()
    if run_record is None:
        return CommandDataIntegrity(request.command_id)
    run = map_run(run_record)
    if isinstance(run, DataIntegrityError):
        return CommandDataIntegrity(request.command_id)
    event_records = (
        await session.execute(
            select(EventRecord)
            .where(EventRecord.payload.contains({"command_id": record.command_id}))
            .order_by(EventRecord.seq)
        )
    ).scalars()
    try:
        events = tuple(
            StoredEvent(EventCursor(event_record.seq), deserialize_event(event_record))
            for event_record in event_records
        )
    except ValueError:
        return CommandDataIntegrity(request.command_id)
    return CommandApplied(run, events, result_reference, replayed=True)
