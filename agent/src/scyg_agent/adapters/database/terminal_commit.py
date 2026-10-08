"""Atomic fenced commit of Run terminal state, events, and audit."""

from enum import StrEnum
from hashlib import sha256
from typing import Protocol, final

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.ports.event_store import (
    Appended,
    EventIdentityConflict,
    EventRunNotFound,
    PartialEventBatchConflict,
)
from scyg_agent.domain.ports.terminal_commit import (
    TerminalCancellationRequested,
    TerminalCommitRequest,
    TerminalCommitResult,
    TerminalCommitted,
    TerminalEventConflict,
    TerminalLeaseLost,
    TerminalReplay,
    TerminalStateConflict,
)
from scyg_agent.domain.runs import Run, RunStatus
from scyg_agent.domain.runs.repository import DataIntegrityError

from .audit_append import append_audit_locked
from .event_append import append_events_locked
from .event_codec import record_matches_event
from .journal_records import EventRecord
from .run_fencing import TERMINAL_STATUSES
from .run_mapper import map_run
from .run_records import AgentRunResultRecord, InteractionRecord, RunRecord


class TerminalStage(StrEnum):
    """事务内可观测的三个写入阶段."""

    EVENTS_APPENDED = "events_appended"
    AUDIT_APPENDED = "audit_appended"
    RUN_UPDATED = "run_updated"


class TerminalFailpoint(Protocol):
    """测试可以在任一写入阶段抛出异常."""

    async def reach(self, stage: TerminalStage) -> None:
        """到达指定阶段."""
        ...


@final
class NoTerminalFailpoint:
    """生产环境的空阶段探针."""

    __slots__ = ()

    async def reach(self, stage: TerminalStage) -> None:
        """接受阶段且不注入行为."""
        _ = stage


NO_TERMINAL_FAILPOINT = NoTerminalFailpoint()


@final
class PostgreSQLTerminalCommitter:
    """在一个短事务内提交完整终态事实."""

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        notification_channel: str,
        failpoint: TerminalFailpoint = NO_TERMINAL_FAILPOINT,
    ) -> None:
        """绑定共享 session 工厂和既有 T11 通知通道."""
        self._sessions = sessions
        self._notification_channel = notification_channel
        self._failpoint = failpoint

    async def commit(self, request: TerminalCommitRequest) -> TerminalCommitResult:
        """创建唯一事务并提交或完整回滚."""
        async with self._sessions.begin() as session:
            return await self.commit_in_session(session, request)

    async def commit_in_session(
        self, session: AsyncSession, request: TerminalCommitRequest
    ) -> TerminalCommitResult:
        """复用调用方 session 且不自行提交."""
        record = await _lock_run(session, request)
        if record is None:
            return TerminalLeaseLost(request.completion.guard.run_id)
        mapped = map_run(record)
        if isinstance(mapped, DataIntegrityError):
            return TerminalStateConflict(request.completion.guard.run_id)
        if mapped.status in TERMINAL_STATUSES:
            return await _terminal_replay(session, mapped, record, request)
        conflict = _active_conflict(record, request)
        if conflict is not None:
            return conflict
        appended = await append_events_locked(session, request.events, self._notification_channel)
        event_conflict = _event_conflict(request, appended)
        if event_conflict is not None:
            return event_conflict
        if not isinstance(appended, Appended):
            reason = "事件追加结果未被完整收窄"
            raise TerminalCommitInvariantError(reason)
        await self._failpoint.reach(TerminalStage.EVENTS_APPENDED)
        audit = await append_audit_locked(session, request.audit)
        await self._failpoint.reach(TerminalStage.AUDIT_APPENDED)
        await _append_result_locked(session, request)
        _apply_terminal(record, request)
        await _ensure_pending_interaction(session, record, request)
        await session.flush()
        await self._failpoint.reach(TerminalStage.RUN_UPDATED)
        committed = map_run(record)
        if not isinstance(committed, Run):
            reason = "终态 Run 映射失败"
            raise TerminalCommitInvariantError(reason)
        return TerminalCommitted(committed, appended.events, audit)


async def _ensure_pending_interaction(
    session: AsyncSession, record: RunRecord, request: TerminalCommitRequest
) -> None:
    """Persist approval facts while the caller still owns the Run row lock."""
    if request.completion.status is not RunStatus.WAITING_INPUT:
        return
    interaction_id = request.completion.pending_interaction_id
    if interaction_id is None:
        reason = "待处理交互缺少身份"
        raise TerminalCommitInvariantError(reason)
    interaction = (
        await session.execute(
            select(InteractionRecord)
            .where(InteractionRecord.interaction_id == str(interaction_id))
            .with_for_update()
        )
    ).scalar_one_or_none()
    if interaction is None:
        session.add(
            InteractionRecord(
                interaction_id=str(interaction_id),
                run_id=record.run_id,
                kind=request.interaction_kind,
                status="pending",
                requested_at=request.completion.guard.now,
                request_semantic_digest=sha256(str(interaction_id).encode()).hexdigest(),
                request_payload=request.interaction_payload,
            )
        )
    elif (
        interaction.run_id != record.run_id
        or interaction.status != "pending"
        or interaction.kind != request.interaction_kind
    ):
        reason = "待处理交互事实冲突"
        raise TerminalCommitInvariantError(reason)
    elif request.interaction_payload is not None:
        if (
            interaction.request_payload is not None
            and interaction.request_payload != request.interaction_payload
        ):
            reason = "交互请求内容冲突"
            raise TerminalCommitInvariantError(reason)
        interaction.request_payload = request.interaction_payload


async def _append_result_locked(session: AsyncSession, request: TerminalCommitRequest) -> None:
    """Insert one validated result while the Run row remains locked."""
    result = request.result
    if result is None:
        return
    run_id = str(request.completion.guard.run_id)
    existing = (
        await session.execute(
            select(AgentRunResultRecord)
            .where(AgentRunResultRecord.run_id == run_id)
            .with_for_update()
        )
    ).scalar_one_or_none()
    if existing is not None:
        if (
            existing.schema_version != result.schema_version
            or existing.capability != result.capability
            or existing.result_payload != result.payload
            or existing.result_digest != result.digest
        ):
            reason = "终态结果不可变内容冲突"
            raise TerminalCommitInvariantError(reason)
        return
    session.add(
        AgentRunResultRecord(
            run_id=run_id,
            schema_version=result.schema_version,
            capability=result.capability,
            result_payload=result.payload,
            result_digest=result.digest,
        )
    )


async def _lock_run(session: AsyncSession, request: TerminalCommitRequest) -> RunRecord | None:
    """按 T11 顺序先锁事件身份, 再锁 Run 行."""
    for event_id in sorted(str(event.event_id) for event in request.events.events):
        _ = await session.execute(
            select(func.pg_advisory_xact_lock(func.hashtextextended(event_id, 0)))
        )
    return (
        await session.execute(
            select(RunRecord)
            .where(RunRecord.run_id == str(request.completion.guard.run_id))
            .with_for_update()
        )
    ).scalar_one_or_none()


def _active_conflict(
    record: RunRecord, request: TerminalCommitRequest
) -> TerminalCommitResult | None:
    """验证活动租约、revision、取消优先级和终态合法性."""
    guard = request.completion.guard
    owns = (
        record.status == RunStatus.RUNNING.value
        and record.lease_owner == str(guard.owner)
        and record.lease_token == guard.token.value
        and record.lease_expires_at is not None
        and record.lease_expires_at > guard.now
    )
    if not owns:
        return TerminalLeaseLost(guard.run_id)
    if record.revision != guard.expected_revision:
        return TerminalStateConflict(guard.run_id)
    if (
        record.cancellation_requested_at is not None
        and request.completion.status is not RunStatus.CANCELLED
    ):
        return TerminalCancellationRequested(guard.run_id)
    if request.completion.status not in (*TERMINAL_STATUSES, RunStatus.WAITING_INPUT):
        return TerminalStateConflict(guard.run_id)
    return None


def _event_conflict(
    request: TerminalCommitRequest,
    result: Appended | EventIdentityConflict | PartialEventBatchConflict | EventRunNotFound,
) -> TerminalEventConflict | None:
    """把 T11 不可变冲突收窄为终态提交结果."""
    match result:  # noqa: RUF100  # noqa: MATCH_OK - T11 AppendResult 全部分支已收窄。
        case Appended():
            return None
        case EventIdentityConflict(event_id=event_id):
            return TerminalEventConflict(request.events.run_id, event_id)
        case PartialEventBatchConflict():
            return TerminalEventConflict(request.events.run_id, None)
        case EventRunNotFound():
            return TerminalEventConflict(request.events.run_id, None)


def _apply_terminal(record: RunRecord, request: TerminalCommitRequest) -> None:
    """在已验证且锁定的 ORM 行上应用唯一终态变化."""
    guard = request.completion.guard
    record.status = request.completion.status.value
    record.revision += 1
    record.updated_at = guard.now
    record.terminal_at = guard.now if request.completion.status in TERMINAL_STATUSES else None
    record.lease_owner = None
    record.lease_token = None
    record.lease_expires_at = None
    record.cancellation_requested_at = None
    record.pending_interaction_id = (
        str(request.completion.pending_interaction_id)
        if request.completion.pending_interaction_id is not None
        else None
    )
    record.error_code = request.error_code
    record.error_message = request.error_message
    record.error_metadata = None


async def _terminal_replay(
    session: AsyncSession, run: Run, record: RunRecord, request: TerminalCommitRequest
) -> TerminalCommitResult:
    """仅当全部事件和终态错误字段不可变内容相同时返回幂等重放."""
    if run.status is not request.completion.status:
        return TerminalStateConflict(run.id)
    if record.error_code != request.error_code or record.error_message != request.error_message:
        return TerminalStateConflict(run.id)
    records = tuple(
        (
            await session.execute(
                select(EventRecord).where(
                    EventRecord.event_id.in_(
                        tuple(str(event.event_id) for event in request.events.events)
                    )
                )
            )
        ).scalars()
    )
    if len(records) != len(request.events.events):
        return TerminalStateConflict(run.id)
    by_id = {event_record.event_id: event_record for event_record in records}
    for event in request.events.events:
        event_record = by_id[str(event.event_id)]
        if not record_matches_event(event_record, event):
            return TerminalEventConflict(run.id, event.event_id)
    return TerminalReplay(run)


class TerminalCommitInvariantError(RuntimeError):
    """报告事务内部不可能的映射失败."""
