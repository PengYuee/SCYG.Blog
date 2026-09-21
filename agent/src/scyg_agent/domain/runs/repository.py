"""Pure typed port for durable Run state and fenced execution leases."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import TYPE_CHECKING, Protocol

from .cancellation import CancellationQueued, CancellationRequested, CancellationTerminal
from .errors import InvalidTimestampError
from .models import (
    ExecutionOwnerId,
    InteractionId,
    OperationId,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    validate_utc_timestamp,
)
from .repository_values import (
    InvalidRepositoryInputError,
    LeaseToken,
    RunLease,
)

if TYPE_CHECKING:
    from .input import RunInput
from .repository_values import (
    reject_repository_input as _reject,
)

__all__ = (
    "CancellationQueued",
    "CancellationRequested",
    "CancellationTerminal",
    "InvalidRepositoryInputError",
    "LeaseToken",
    "RunLease",
)


@dataclass(frozen=True, slots=True)
class ClaimRequest:
    """Describe one bounded queue claim using an injected UTC clock."""

    limit: int
    worker: ExecutionOwnerId
    runtime_kind: RuntimeKind
    now: datetime
    lease_duration: timedelta

    def __post_init__(self) -> None:
        """Reject malformed capacity and time inputs before SQL execution."""
        if self.limit <= 0:
            _reject("limit", "must be positive")
        if type(self.runtime_kind) is not RuntimeKind:
            _reject("runtime kind", "must be an exact RuntimeKind")
        try:
            validate_utc_timestamp(self.now, "now")
        except InvalidTimestampError:
            _reject("now", "must be aware UTC")
        if self.lease_duration <= timedelta(0):
            _reject("lease duration", "must be positive")


@dataclass(frozen=True, slots=True)
class CreateRunRequest:
    """Bind an initial aggregate to its idempotent operation identity."""

    run: Run
    operation_id: OperationId
    next_attempt_at: datetime
    input: RunInput

    def __post_init__(self) -> None:
        """Require a claimable initial aggregate and aware schedule."""
        if self.run.status is not RunStatus.PENDING:
            _reject("run status", "must be pending")
        try:
            validate_utc_timestamp(self.next_attempt_at, "next_attempt_at")
        except InvalidTimestampError:
            _reject("next_attempt_at", "must be aware UTC")


@dataclass(frozen=True, slots=True)
class LeaseGuard:
    """Fence a mutation by ownership, token, revision, and current time."""

    run_id: RunId
    owner: ExecutionOwnerId
    token: LeaseToken
    expected_revision: int
    now: datetime

    def __post_init__(self) -> None:
        """Validate the optimistic and temporal fence before SQL execution."""
        if self.expected_revision < 1:
            _reject("expected revision", "must be positive")
        try:
            validate_utc_timestamp(self.now, "now")
        except InvalidTimestampError:
            _reject("now", "must be aware UTC")


@dataclass(frozen=True, slots=True)
class CancellationRequest:
    """持久化一个幂等且不撤销租约的取消请求."""

    run_id: RunId
    requested_at: datetime

    def __post_init__(self) -> None:
        """要求取消时间为 UTC."""
        try:
            validate_utc_timestamp(self.requested_at, "requested_at")
        except InvalidTimestampError:
            _reject("requested_at", "must be aware UTC")


@dataclass(frozen=True, slots=True)
class RenewRequest:
    """Extend one live lease without changing its revision."""

    guard: LeaseGuard
    lease_duration: timedelta

    def __post_init__(self) -> None:
        """Require a positive renewal interval."""
        if self.lease_duration <= timedelta(0):
            _reject("lease duration", "must be positive")


@dataclass(frozen=True, slots=True)
class ReleaseRequest:
    """Return execution to the queue at a deterministic future instant."""

    guard: LeaseGuard
    next_attempt_at: datetime

    def __post_init__(self) -> None:
        """Require a non-regressing retry schedule."""
        try:
            validate_utc_timestamp(self.next_attempt_at, "next_attempt_at")
        except InvalidTimestampError:
            _reject("next_attempt_at", "must be aware UTC")
        if self.next_attempt_at < self.guard.now:
            _reject("next_attempt_at", "cannot precede now")


@dataclass(frozen=True, slots=True)
class CompletionRequest:
    """Apply one fenced lifecycle outcome and optional deterministic retry policy."""

    guard: LeaseGuard
    status: RunStatus
    pending_interaction_id: InteractionId | None = None
    retry_at: datetime | None = None
    max_attempts: int | None = None

    def __post_init__(self) -> None:
        """Validate completion semantics before issuing a guarded update."""
        allowed = {
            RunStatus.SUCCEEDED,
            RunStatus.FAILED,
            RunStatus.CANCELLED,
            RunStatus.WAITING_INPUT,
        }
        if self.status not in allowed:
            _reject("completion status", "is not supported")
        if self.status is RunStatus.WAITING_INPUT and self.pending_interaction_id is None:
            _reject("pending interaction", "is required")
        if self.retry_at is not None:
            try:
                validate_utc_timestamp(self.retry_at, "retry_at")
            except InvalidTimestampError:
                _reject("retry_at", "must be aware UTC")
            if self.retry_at < self.guard.now:
                _reject("retry_at", "cannot precede now")
            if self.max_attempts is None or self.max_attempts <= 0:
                _reject("max attempts", "must be positive")


@dataclass(frozen=True, slots=True)
class Created:
    """Confirm a newly persisted aggregate."""

    run: Run


@dataclass(frozen=True, slots=True)
class DuplicateOperation:
    """Return the original aggregate for an idempotent create replay."""

    run: Run


@dataclass(frozen=True, slots=True)
class CreateConflict:
    """报告操作身份已绑定不同创建语义。."""

    operation_id: OperationId


@dataclass(frozen=True, slots=True)
class NotFound:
    """Report an absent Run identity."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class DataIntegrityError:
    """Report a persisted row that violates the Run domain contract."""

    run_id: str
    reason: str


@dataclass(frozen=True, slots=True)
class LeaseLost:
    """Report a zero-row guarded lease mutation."""

    run_id: RunId


@dataclass(frozen=True, slots=True)
class RevisionConflict:
    """Report a live lease whose aggregate revision differs from the expected fence."""

    run_id: RunId
    expected_revision: int
    actual_revision: int


@dataclass(frozen=True, slots=True)
class Renewed:
    """Return the extended immutable lease."""

    lease: RunLease


@dataclass(frozen=True, slots=True)
class RetryScheduled:
    """Confirm release to pending with its next eligible instant."""

    run: Run
    next_attempt_at: datetime


@dataclass(frozen=True, slots=True)
class Completed:
    """Confirm a terminal or waiting-input lifecycle update."""

    run: Run


@dataclass(frozen=True, slots=True)
class TerminalFailure:
    """Confirm retry exhaustion and terminal failure."""

    run: Run


type CreateResult = Created | DuplicateOperation | CreateConflict | DataIntegrityError
type GetResult = Run | NotFound | DataIntegrityError
type RenewResult = Renewed | CancellationRequested | LeaseLost
type ReleaseResult = RetryScheduled | LeaseLost
type CancellationResult = (
    CancellationRequested
    | CancellationQueued
    | CancellationTerminal
    | NotFound
    | DataIntegrityError
)
type CompleteResult = Completed | RetryScheduled | TerminalFailure | LeaseLost | RevisionConflict


class RunRepository(Protocol):
    """Persist Run aggregates through bounded self-owned transactions."""

    async def create(self, request: CreateRunRequest) -> CreateResult:
        """Create or return the aggregate bound to an existing operation."""
        ...  # pragma: no cover - protocol declaration.

    async def get(self, run_id: RunId) -> GetResult:
        """Load one aggregate through the domain invariant mapper."""
        ...  # pragma: no cover - protocol declaration.

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """Claim up to the requested free capacity without waiting on locked rows."""
        ...  # pragma: no cover - protocol declaration.

    async def request_cancellation(self, request: CancellationRequest) -> CancellationResult:
        """记录取消意图而不清除当前租约."""
        ...  # pragma: no cover - protocol declaration.

    async def renew(self, request: RenewRequest) -> RenewResult:
        """Extend one unexpired fenced lease."""
        ...  # pragma: no cover - protocol declaration.

    async def release(self, request: ReleaseRequest) -> ReleaseResult:
        """Return one fenced execution to the pending queue."""
        ...  # pragma: no cover - protocol declaration.

    async def complete(self, request: CompletionRequest) -> CompleteResult:
        """Apply a fenced terminal, retry, or waiting-input outcome."""
        ...  # pragma: no cover - protocol declaration.
