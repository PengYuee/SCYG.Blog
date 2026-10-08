"""Owner-authorized control operations sharing one successful-key transaction."""

import json
from dataclasses import dataclass
from datetime import datetime, timedelta
from hashlib import sha256
from math import isfinite
from typing import TYPE_CHECKING, Final, cast, final
from uuid import UUID, uuid4

from pydantic import ValidationError
from sqlalchemy import DateTime, delete, func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.event_append import append_events_locked
from scyg_agent.adapters.database.run_records import (
    AgentRunResultRecord,
    InteractionRecord,
    RunRecord,
    SuccessfulOperationRecord,
)
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.agents.contracts import (
    INPUT_SCHEMA_VERSION,
    Capability,
    FailureKind,
    input_digest,
    input_payload,
    recipe_for_capability,
)
from scyg_agent.agents.recipes import RecipeRegistry, default_recipe_registry
from scyg_agent.domain.ports.event_store import AppendRequest
from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    InteractionId,
    OperationId,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.events import InputResolved, RunCancelled, StatusChanged
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.domain.runs.repository import CreateRunRequest

from .event_subscription import EventSubscriptionService, OpenedEventStream

if TYPE_CHECKING:
    from sqlalchemy import Result

    from scyg_agent.agents.contracts import CapabilityInput

KEY_LOCK_NAMESPACE = 0x53434147
_SURROGATE_MIN: Final = 0xD800
_SURROGATE_MAX: Final = 0xDFFF
_INVALID_INPUT: Final = ("INVALID_ARGUMENT", "请求参数无效")
_INVALID_RUN_STATE: Final = ("FAILED_PRECONDITION", "Run 当前状态不允许此操作")
_RESOLVED_INTERACTION: Final = ("FAILED_PRECONDITION", "交互已处理或不再等待响应")
_INVALID_DECISION: Final = ("INVALID_ARGUMENT", "交互决策无效")
_INVALID_PAYLOAD: Final = ("INVALID_ARGUMENT", "交互响应 payload 无效")
_MISSING_PAYLOAD: Final = ("INVALID_ARGUMENT", "交互响应缺少 payload")
_TERMINAL_RUN: Final = ("FAILED_PRECONDITION", "Run 已结束,不能取消")
_RUN_NOT_FOUND: Final = ("NOT_FOUND", "未找到 Run")
_INVALID_PERSISTED_RUN: Final = ("INTERNAL", "Run 持久化状态异常")
_INVALID_PERSISTED_INTERACTION: Final = ("INTERNAL", "Run 交互状态异常")


@dataclass(slots=True)
class ControlError(Exception):
    """Expose a sanitized application failure to the transport boundary."""

    code: str
    message: str


@dataclass(frozen=True, slots=True)
class ResumeCommand:
    """Carry one validated response while preserving optional JSON presence."""

    run_id: str
    interaction_id: str
    decision: str
    payload: bytes | None


@dataclass(frozen=True, slots=True)
class PendingSnapshot:
    """Capture the current interaction from the same Run statement snapshot."""

    interaction_id: str
    kind: str
    payload: object


@dataclass(frozen=True, slots=True)
class ControlSnapshot:
    """Capture all public Run fields from one owner-authorized SQL statement."""

    run_id: str
    status: str
    capability: str
    recipe_id: str
    recipe_version: str
    created_at: datetime
    updated_at: datetime
    failure_code: str | None
    failure_message: str | None
    result_present: bool
    result: object
    interaction: PendingSnapshot | None


@final
class ControlApplication:
    """Apply public control commands atomically with successful-key records."""

    def __init__(
        self,
        sessions: async_sessionmaker[AsyncSession],
        notification_channel: str,
        events: EventSubscriptionService,
        recipes: RecipeRegistry | None = None,
    ) -> None:
        """Bind the shared sessions, journal and immutable Recipe registry."""
        self._sessions = sessions
        self._channel = notification_channel
        self._events = events
        self._recipes = recipes or default_recipe_registry()
        self._runs = PostgreSQLRunRepository(sessions)

    async def get(self, user_id: str, run_id: str) -> ControlSnapshot:
        """Read an owner-authorized consistent public snapshot."""
        async with self._sessions() as session:
            return await self._snapshot(session, user_id, run_id)

    async def open_events(self, user_id: str, run_id: str, cursor: str | None) -> OpenedEventStream:
        """Authorize before opening and validating a durable subscription."""
        _ = await self.get(user_id, run_id)
        return await self._events.open_events(RunId(run_id), cursor)

    async def create(
        self, user_id: str, key: str, capability: Capability, raw_json: bytes
    ) -> ControlSnapshot:
        """Replay a successful key before parsing new business input."""
        async with self._sessions.begin() as session:
            replay = await self._reserve(session, user_id, key)
            if replay is not None:
                return replay
            recipe_id = recipe_for_capability(capability)
            recipe = self._recipes.resolve(recipe_id, "v1")
            if (
                recipe is None
                or not recipe.recipe_id.value
                or not recipe.version
                or recipe.capability is not capability
            ):
                raise ControlError(*_INVALID_INPUT)
            try:
                value = cast("CapabilityInput", recipe.input_schema.model_validate_json(raw_json))
                value_payload = input_payload(value)
                _validate_json_storage(value_payload)
                digest = input_digest(value)
            except (ValidationError, ValueError, UnicodeError, RecursionError):
                raise ControlError(*_INVALID_INPUT) from None
            now = await self._now(session)
            run_id = RunId(f"run_{uuid4().hex}")
            task, runtime, field = {
                Capability.SEARCH: (TaskType.RESEARCH, RuntimeKind.DEEP, "query"),
                Capability.WRITE: (TaskType.COMPOSE, RuntimeKind.DEEP, "topic"),
                Capability.POLISH: (TaskType.POLISH, RuntimeKind.SIMPLE, "content"),
                Capability.CHAT: (TaskType.QUESTION, RuntimeKind.SIMPLE, "message"),
            }[capability]
            run = Run(
                run_id,
                UserId(user_id),
                task,
                RuntimeSelection(runtime, "v1"),
                1,
                RunStatus.PENDING,
                now,
                now,
                0,
                None,
                None,
            )
            _ = await self._runs.create_in_session(
                session,
                CreateRunRequest(
                    run,
                    OperationId(str(uuid4())),
                    now,
                    RunInput(
                        str(value_payload[field]),
                        "",
                        capability.value,
                        recipe_id.value,
                        recipe.version,
                        INPUT_SCHEMA_VERSION,
                        value_payload,
                        digest,
                        "und",
                        str(run_id),
                        recipe.model_tier,
                        "v1",
                    ),
                ),
            )
            return await self._success(session, user_id, key, str(run_id))

    async def resume(self, user_id: str, key: str, command: ResumeCommand) -> ControlSnapshot:
        """Resolve one pending interaction without overwriting its history."""
        run_id, interaction_id = command.run_id, command.interaction_id
        decision, payload = command.decision, command.payload
        async with self._sessions.begin() as session:
            replay = await self._reserve(session, user_id, key)
            if replay is not None:
                return replay
            run = await self._owned_locked(session, user_id, run_id)
            if run.status != "waiting_input" or run.pending_interaction_id != interaction_id:
                raise ControlError(*_INVALID_RUN_STATE)
            interaction = (
                await session.execute(
                    select(InteractionRecord)
                    .where(
                        InteractionRecord.run_id == run_id,
                        InteractionRecord.interaction_id == interaction_id,
                    )
                    .with_for_update()
                )
            ).scalar_one_or_none()
            if interaction is None or interaction.status != "pending":
                raise ControlError(*_RESOLVED_INTERACTION)
            kind = normalize_interaction_kind(interaction.kind)
            allowed = {
                "confirmation": {"approve", "reject"},
                "selection": {"select"},
                "text_input": {"submit"},
            }[kind]
            if decision not in allowed:
                raise ControlError(*_INVALID_DECISION)
            try:
                parsed = cast("object", json.loads(payload)) if payload is not None else None
                _validate_json_storage(parsed)
            except (ValueError, UnicodeError, RecursionError):
                raise ControlError(*_INVALID_PAYLOAD) from None
            if kind in {"selection", "text_input"} and payload is None:
                raise ControlError(*_MISSING_PAYLOAD)
            try:
                digest = sha256(
                    json.dumps(
                        {"decision": decision, "present": payload is not None, "payload": parsed},
                        sort_keys=True,
                        ensure_ascii=False,
                        allow_nan=False,
                    ).encode()
                ).hexdigest()
            except (ValueError, UnicodeError, RecursionError):
                raise ControlError(*_INVALID_PAYLOAD) from None
            now = await self._now(session)
            interaction.status = "resolved"
            interaction.resolved_at = now
            interaction.response_digest = digest
            interaction.resolution_semantic_digest = digest
            interaction.decision = decision
            interaction.response_payload = parsed
            interaction.payload_present = payload is not None
            interaction.result_reference = (
                "approval:rejected" if decision == "reject" else "approval:accepted"
            )
            run.status = "pending_resume"
            run.pending_interaction_id = None
            run.updated_at = now
            run.revision += 1
            run.next_attempt_at = now
            command_id = CommandId(f"cmd_{uuid4().hex}")
            common = (EventId(f"evt_{uuid4().hex}"), command_id, now, RunId(run_id), run.revision)
            _ = await append_events_locked(
                session,
                AppendRequest(
                    RunId(run_id), (InputResolved(*common, InteractionId(interaction_id)),)
                ),
                self._channel,
            )
            return await self._success(session, user_id, key, run_id)

    async def cancel(self, user_id: str, key: str, run_id: str) -> ControlSnapshot:
        """Set cancellation and revoke worker admission under the Run lock."""
        async with self._sessions.begin() as session:
            replay = await self._reserve(session, user_id, key)
            if replay is not None:
                return replay
            run = await self._owned_locked(session, user_id, run_id)
            if run.status in {"succeeded", "failed"}:
                raise ControlError(*_TERMINAL_RUN)
            if run.status != "cancelled":
                now = await self._now(session)
                previous = RunStatus(run.status)
                run.status = "cancelled"
                run.revision += 1
                run.updated_at = now
                run.terminal_at = now
                run.cancellation_requested_at = now
                run.pending_interaction_id = None
                run.lease_owner = None
                run.lease_token = None
                run.lease_expires_at = None
                run.next_attempt_at = None
                command_id = CommandId(f"cmd_{uuid4().hex}")
                _ = await append_events_locked(
                    session,
                    AppendRequest(
                        RunId(run_id),
                        (
                            StatusChanged(
                                EventId(f"evt_{uuid4().hex}"),
                                command_id,
                                now,
                                RunId(run_id),
                                run.revision,
                                previous,
                                RunStatus.CANCELLED,
                            ),
                            RunCancelled(
                                EventId(f"evt_{uuid4().hex}"),
                                command_id,
                                now,
                                RunId(run_id),
                                run.revision,
                            ),
                        ),
                    ),
                    self._channel,
                )
            return await self._success(session, user_id, key, run_id)

    async def _reserve(
        self, session: AsyncSession, user_id: str, key: str
    ) -> ControlSnapshot | None:
        identity = UUID(key)
        hash_value = int.from_bytes(
            sha256(f"{user_id}:{identity}".encode()).digest()[:4], "big", signed=True
        )
        _ = await session.execute(
            select(func.pg_advisory_xact_lock(KEY_LOCK_NAMESPACE, hash_value))
        )
        record = (
            await session.execute(
                select(SuccessfulOperationRecord).where(
                    SuccessfulOperationRecord.user_id == user_id,
                    SuccessfulOperationRecord.idempotency_key == identity,
                    SuccessfulOperationRecord.expires_at > func.clock_timestamp(),
                )
            )
        ).scalar_one_or_none()
        if record is not None:
            return await self._snapshot(session, user_id, record.run_id)
        _ = await session.execute(
            delete(SuccessfulOperationRecord).where(
                SuccessfulOperationRecord.user_id == user_id,
                SuccessfulOperationRecord.idempotency_key == identity,
                SuccessfulOperationRecord.expires_at <= func.clock_timestamp(),
            )
        )
        return None

    async def _success(
        self, session: AsyncSession, user_id: str, key: str, run_id: str
    ) -> ControlSnapshot:
        await session.flush()
        now = await self._now(session)
        session.add(
            SuccessfulOperationRecord(
                user_id=user_id,
                idempotency_key=UUID(key),
                run_id=run_id,
                succeeded_at=now,
                expires_at=now + timedelta(hours=24),
            )
        )
        await session.flush()
        return await self._snapshot(session, user_id, run_id)

    async def _owned_locked(self, session: AsyncSession, user_id: str, run_id: str) -> RunRecord:
        run = (
            await session.execute(
                select(RunRecord)
                .where(RunRecord.run_id == run_id, RunRecord.owner_user_id == user_id)
                .with_for_update()
            )
        ).scalar_one_or_none()
        if run is None:
            raise ControlError(*_RUN_NOT_FOUND)
        return run

    async def _snapshot(self, session: AsyncSession, user_id: str, run_id: str) -> ControlSnapshot:
        await session.flush()
        result_rows = cast(
            "Result[tuple[RunRecord, AgentRunResultRecord | None, InteractionRecord | None]]",
            (
                await session.execute(
                    select(RunRecord, AgentRunResultRecord, InteractionRecord)
                    .outerjoin(
                        AgentRunResultRecord,
                        AgentRunResultRecord.run_id == RunRecord.run_id,
                    )
                    .outerjoin(
                        InteractionRecord,
                        (InteractionRecord.interaction_id == RunRecord.pending_interaction_id)
                        & (InteractionRecord.run_id == RunRecord.run_id),
                    )
                    .where(
                        RunRecord.run_id == run_id,
                        RunRecord.owner_user_id == user_id,
                    )
                    .execution_options(populate_existing=True)
                )
            ),
        )
        row = result_rows.tuples().one_or_none()
        if row is None:
            raise ControlError(*_RUN_NOT_FOUND)
        run, result, interaction = row
        if not run.capability or not run.recipe_id or not run.recipe_version:
            raise ControlError(*_INVALID_PERSISTED_RUN)
        pending = (
            None
            if interaction is None
            else PendingSnapshot(
                interaction.interaction_id,
                normalize_interaction_kind(interaction.kind),
                interaction.request_payload,
            )
        )
        failure_code = None
        if run.error_code is not None:
            failure_code = (
                run.error_code
                if run.error_code in {kind.value for kind in FailureKind}
                else "internal_failure"
            )
        return ControlSnapshot(
            run.run_id,
            run.status,
            run.capability,
            run.recipe_id,
            run.recipe_version,
            run.created_at,
            run.updated_at,
            failure_code,
            "执行失败" if run.error_code is not None else None,
            result is not None,
            result.result_payload if result is not None else None,
            pending,
        )

    @staticmethod
    async def _now(session: AsyncSession) -> datetime:
        return (
            await session.execute(select(func.clock_timestamp(type_=DateTime(timezone=True))))
        ).scalar_one()


def normalize_interaction_kind(kind: str) -> str:
    """Reject persisted interaction kinds outside the public contract."""
    if kind in {"confirmation", "selection", "text_input"}:
        return kind
    raise ControlError(*_INVALID_PERSISTED_INTERACTION)


def _validate_json_storage(value: object) -> None:
    """Reject only JSON values PostgreSQL JSONB/UTF-8 cannot represent."""
    if isinstance(value, str):
        if "\x00" in value or any(
            _SURROGATE_MIN <= ord(character) <= _SURROGATE_MAX for character in value
        ):
            raise ValueError
    elif isinstance(value, float):
        if not isfinite(value):
            raise ValueError
    elif isinstance(value, dict):
        for key, item in cast("dict[str, object]", value).items():
            _validate_json_storage(key)
            _validate_json_storage(item)
    elif isinstance(value, list):
        for item in cast("list[object]", value):
            _validate_json_storage(item)
