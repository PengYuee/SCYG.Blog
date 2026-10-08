"""Immutable Run aggregate and validated domain values."""

from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import UTC, datetime
from enum import StrEnum
from typing import ClassVar, Final, Self, override

from .errors import (
    InvalidEnumValueError,
    InvalidIdentifierError,
    InvalidRunError,
    InvalidRuntimeVersionError,
    InvalidTimestampError,
)

_OPAQUE_ID_MAX_BYTES: Final = 128


@dataclass(frozen=True, slots=True)
class _Identifier:
    """Validate a semantically branded string identifier at construction."""

    pattern: ClassVar[re.Pattern[str]]
    value: str

    def __post_init__(self) -> None:
        """Reject values that do not satisfy the concrete identity contract."""
        if self.pattern.fullmatch(self.value) is None:
            raise InvalidIdentifierError(type(self).__name__, self.value)

    @override
    def __str__(self) -> str:
        """Expose the validated opaque identity value."""
        return self.value


@dataclass(frozen=True, slots=True)
class RunId(_Identifier):
    """Identify one durable Agent run."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"(?!(?:\.|\.\.)$)[A-Za-z0-9._~-]{1,128}")


@dataclass(frozen=True, slots=True)
class UserId(_Identifier):
    """Identify the Blog user who owns a run."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"[\s\S]+")

    def __post_init__(self) -> None:
        """按 UTF-8 字节长度验证不透明用户标识符."""
        if not 1 <= len(self.value.encode("utf-8")) <= _OPAQUE_ID_MAX_BYTES:
            raise InvalidIdentifierError(type(self).__name__, self.value)


@dataclass(frozen=True, slots=True)
class OperationId(_Identifier):
    """Identify one idempotent business operation."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"[A-Za-z0-9][A-Za-z0-9._:-]{0,127}")


@dataclass(frozen=True, slots=True)
class CommandId(_Identifier):
    """Identify one durable Run command."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"cmd_[A-Za-z0-9][A-Za-z0-9_-]{7,63}")


@dataclass(frozen=True, slots=True)
class EventId(_Identifier):
    """Identify one emitted Run domain event."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"evt_[A-Za-z0-9][A-Za-z0-9_-]{7,63}")


@dataclass(frozen=True, slots=True)
class InteractionId(_Identifier):
    """Identify one durable user interaction."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"[\s\S]+")

    def __post_init__(self) -> None:
        """按 UTF-8 字节长度验证不透明交互标识符."""
        if not 1 <= len(self.value.encode("utf-8")) <= _OPAQUE_ID_MAX_BYTES:
            raise InvalidIdentifierError(type(self).__name__, self.value)


@dataclass(frozen=True, slots=True)
class ToolCallId(_Identifier):
    """Identify one logical runtime tool invocation."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"tool_[A-Za-z0-9][A-Za-z0-9_-]{7,63}")


@dataclass(frozen=True, slots=True)
class ExecutionOwnerId(_Identifier):
    """Identify the worker currently responsible for execution."""

    pattern: ClassVar[re.Pattern[str]] = re.compile(r"worker_[A-Za-z0-9][A-Za-z0-9_-]{7,63}")


class TaskType(StrEnum):
    """Select a statically registered Agent task."""

    SUMMARY = "summary"
    QUESTION = "question"
    POLISH = "polish"
    COMPOSE = "compose"
    RESEARCH = "research"
    REVISE = "revise"

    @classmethod
    def parse(cls, raw: str) -> Self:
        """Parse an external task value into the closed task catalog."""
        try:
            return cls(raw)
        except ValueError:
            raise InvalidEnumValueError(cls.__name__, raw) from None


class RuntimeKind(StrEnum):
    """Select an application-owned runtime adapter family."""

    SIMPLE = "simple"
    DEEP = "deep"

    @classmethod
    def parse(cls, raw: str) -> Self:
        """Parse an external runtime kind into the closed adapter catalog."""
        try:
            return cls(raw)
        except ValueError:
            raise InvalidEnumValueError(cls.__name__, raw) from None


class RunStatus(StrEnum):
    """Represent the complete durable Run lifecycle."""

    PENDING = "pending"
    RUNNING = "running"
    WAITING_INPUT = "waiting_input"
    PENDING_RESUME = "pending_resume"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"

    @classmethod
    def parse(cls, raw: str) -> Self:
        """Parse an external status into the closed lifecycle."""
        try:
            return cls(raw)
        except ValueError:
            raise InvalidEnumValueError(cls.__name__, raw) from None


def validate_utc_timestamp(value: datetime, field: str) -> None:
    """Require a timezone-aware instant normalized to UTC."""
    if value.tzinfo is None or value.utcoffset() != UTC.utcoffset(value):
        raise InvalidTimestampError(field)


@dataclass(frozen=True, slots=True)
class RuntimeSelection:
    """Freeze a registered runtime family and immutable profile version."""

    kind: RuntimeKind
    version: str

    def __post_init__(self) -> None:
        """Require the transport-compatible vN version syntax."""
        if re.fullmatch(r"v[1-9][0-9]*", self.version) is None:
            raise InvalidRuntimeVersionError(self.version)


@dataclass(frozen=True, slots=True)
class Run:
    """Own the pure application lifecycle independently of runtime adapters."""

    id: RunId
    owner_user_id: UserId
    task_type: TaskType
    runtime: RuntimeSelection
    revision: int
    status: RunStatus
    created_at: datetime
    updated_at: datetime
    attempt: int
    execution_owner: ExecutionOwnerId | None
    pending_interaction_id: InteractionId | None

    def __post_init__(self) -> None:
        """Enforce revision, time, and execution ownership invariants."""
        validate_utc_timestamp(self.created_at, "created_at")
        validate_utc_timestamp(self.updated_at, "updated_at")
        self._validate_progress()
        self._validate_state()

    def _validate_progress(self) -> None:
        # Revisions, attempts, and aggregate time only move forward.
        if self.revision < 1:
            invariant = "revision must be positive"
            raise InvalidRunError(invariant)
        if self.attempt < 0:
            invariant = "attempt must be non-negative"
            raise InvalidRunError(invariant)
        if self.updated_at < self.created_at:
            invariant = "updated_at cannot precede created_at"
            raise InvalidRunError(invariant)

    def _validate_state(self) -> None:
        # Execution and pending interaction ownership follow the durable status.
        if self.status is RunStatus.RUNNING:
            if self.execution_owner is None:
                invariant = "RUNNING requires execution ownership"
                raise InvalidRunError(invariant)
            if self.pending_interaction_id is not None:
                invariant = "RUNNING cannot retain a pending interaction"
                raise InvalidRunError(invariant)
            return
        if self.status is RunStatus.WAITING_INPUT:
            if self.execution_owner is not None:
                invariant = "WAITING_INPUT cannot retain execution ownership"
                raise InvalidRunError(invariant)
            if self.pending_interaction_id is None:
                invariant = "WAITING_INPUT requires a pending interaction"
                raise InvalidRunError(invariant)
            return
        if self.execution_owner is not None:
            invariant = f"{self.status.name} cannot retain execution ownership"
            raise InvalidRunError(invariant)
        if self.pending_interaction_id is not None:
            invariant = f"{self.status.name} cannot retain a pending interaction"
            raise InvalidRunError(invariant)
