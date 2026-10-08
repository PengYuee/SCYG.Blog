"""单个围栏 Run 的执行、续租、流发布与原子终态提交。"""  # noqa: D415

from collections import deque
from collections.abc import Mapping
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from hashlib import sha256
from typing import Final

import anyio

from scyg_agent.adapters.redis import (
    RedisStreamKind,
    RedisStreamStore,
    RedisUnavailableError,
    StaleAttemptError,
    StreamEnvelope,
)
from scyg_agent.agents import AgentFailure, FailureKind
from scyg_agent.agents.runner import (
    AgentFailed,
    AgentRunOutcome,
    AgentSucceeded,
    AgentWaitingForApproval,
)
from scyg_agent.domain.ports.audit_store import AuditFact
from scyg_agent.domain.ports.event_store import AppendRequest
from scyg_agent.domain.ports.idempotency import AuditMetadata
from scyg_agent.domain.ports.terminal_commit import (
    TerminalCommitRequest,
    TerminalCommitted,
    TerminalResult,
)
from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    DomainEvent,
    InteractionId,
    Run,
    RunFailed,
    RunId,
    RunStatus,
    RunSucceeded,
)
from scyg_agent.domain.runs.repository import (
    CancellationRequested,
    CompletionRequest,
    LeaseGuard,
    LeaseLost,
    Renewed,
    RenewRequest,
    RunLease,
)

from .config import DEFAULT_STREAM_FLUSH_CHARS, DEFAULT_STREAM_FLUSH_INTERVAL, WorkerConfig
from .contracts import WorkerDependencies
from .identity import command_id as make_command_id
from .identity import event_id
from .persistence import finish_persistence

REDIS_FAILURE_CODE: Final = "redis_failure"
REDIS_FAILURE_MESSAGE: Final = "Redis 流存储不可用"


@dataclass(slots=True)
class _RunStreamPublisher:
    """Publish fenced transient events with bounded text buffering."""

    store: RedisStreamStore
    run_id: RunId
    attempt: int
    flush_chars: int = DEFAULT_STREAM_FLUSH_CHARS
    flush_interval: timedelta = DEFAULT_STREAM_FLUSH_INTERVAL
    sequence: int = 0
    _text_parts: deque[str] = field(default_factory=deque)
    _buffered_chars: int = 0
    _last_flush_at: datetime | None = None

    async def activate(self) -> None:
        """Fence this attempt before any transient output is written."""
        _ = await self.store.activate_attempt(self.run_id, self.attempt)
        self._last_flush_at = None

    async def publish(
        self, kind: RedisStreamKind, payload: Mapping[str, str], occurred_at: datetime
    ) -> None:
        """Append one bounded envelope and advance sequence only after success."""
        _ = await self.store.append(
            StreamEnvelope(
                self.run_id,
                self.attempt,
                kind,
                self.sequence,
                occurred_at,
                dict(payload),
            )
        )
        self.sequence += 1

    async def publish_text(self, content: str, occurred_at: datetime) -> None:
        """Buffer text and flush full chunks or an elapsed interval."""
        if not content:
            return
        self._text_parts.append(content)
        self._buffered_chars += len(content)
        last_flush_at = self._last_flush_at
        if last_flush_at is None:
            last_flush_at = occurred_at
            self._last_flush_at = occurred_at
        if self._buffered_chars and occurred_at - last_flush_at >= self.flush_interval:
            await self.flush(occurred_at)

    async def flush(self, occurred_at: datetime) -> None:
        """Flush all buffered text before terminal or control events."""
        await self._flush_full_chunks(occurred_at)
        if self._buffered_chars:
            chunk = self._take_text(self._buffered_chars)
            await self.publish(RedisStreamKind.TEXT_DELTA, {"text": chunk}, occurred_at)
        self._last_flush_at = occurred_at

    async def _flush_full_chunks(self, occurred_at: datetime) -> None:
        """Flush only complete chunks, retaining a bounded remainder."""
        flushed = False
        while self._buffered_chars >= self.flush_chars:
            chunk = self._take_text(self.flush_chars)
            await self.publish(RedisStreamKind.TEXT_DELTA, {"text": chunk}, occurred_at)
            flushed = True
        if flushed:
            self._last_flush_at = occurred_at

    def _take_text(self, limit: int) -> str:
        """Remove and return at most one configured text chunk."""
        remaining = limit
        parts: list[str] = []
        while remaining and self._text_parts:
            part = self._text_parts[0]
            if len(part) <= remaining:
                parts.append(self._text_parts.popleft())
                remaining -= len(part)
            else:
                parts.append(part[:remaining])
                self._text_parts[0] = part[remaining:]
                remaining = 0
        chunk = "".join(parts)
        self._buffered_chars -= len(chunk)
        return chunk


def _stream_publisher(
    dependencies: WorkerDependencies, run: Run, lease: RunLease, config: WorkerConfig
) -> _RunStreamPublisher | None:
    """Create a publisher only when production injected the Redis stream port."""
    if dependencies.stream_store is None:
        return None
    return _RunStreamPublisher(
        dependencies.stream_store,
        run.id,
        lease.attempt,
        config.stream_flush_chars,
        config.stream_flush_interval,
    )


async def _publish_control(
    publisher: _RunStreamPublisher,
    kind: RedisStreamKind,
    payload: Mapping[str, str],
    occurred_at: datetime,
) -> None:
    """Flush pending text before an immediate control or terminal event."""
    await publisher.flush(occurred_at)
    await publisher.publish(kind, payload, occurred_at)


async def _publish_agent_outcome(
    publisher: _RunStreamPublisher | None,
    outcome: AgentRunOutcome,
    occurred_at: datetime,
) -> None:
    """Map the structured AgentRunner outcome to one safe transient frame."""
    if publisher is None:
        return
    match outcome:
        case AgentSucceeded(capability=capability) as succeeded:
            result = succeeded.terminal_result()
            await _publish_control(
                publisher,
                RedisStreamKind.TERMINAL,
                {
                    "status": "succeeded",
                    "capability": capability.value,
                    "result_digest": result.digest,
                },
                occurred_at,
            )
        case AgentWaitingForApproval(request=request):
            await _publish_control(
                publisher,
                RedisStreamKind.APPROVAL_REQUIRED,
                {
                    "interaction_id": request.interaction_id,
                    "title": request.title,
                    "outline": request.outline,
                },
                occurred_at,
            )
        case AgentFailed(failure=failure):
            await _publish_control(
                publisher,
                RedisStreamKind.TERMINAL,
                {"status": "failed", "error_code": failure.kind.value},
                occurred_at,
            )


async def execute_lease(
    lease: RunLease,
    dependencies: WorkerDependencies,
    config: WorkerConfig,
) -> None:
    """Execute only the frozen Recipe runner under an authoritative lease."""
    loaded = await finish_persistence(dependencies.repository.get(lease.run_id))
    now = dependencies.clock()
    if (
        not isinstance(loaded, Run)
        or loaded.status is not RunStatus.RUNNING
        or loaded.id != lease.run_id
        or loaded.execution_owner != lease.owner
        or loaded.revision != lease.revision
        or loaded.attempt != lease.attempt
        or lease.expires_at <= now
        or lease.cancellation_requested_at is not None
    ):
        return
    admitted = await finish_persistence(
        dependencies.repository.renew(
            RenewRequest(
                LeaseGuard(lease.run_id, lease.owner, lease.token, lease.revision, now),
                config.lease_duration,
            )
        )
    )
    if not isinstance(admitted, Renewed):
        return
    lease = admitted.lease
    ownership = {"active": True}
    publisher = _stream_publisher(dependencies, loaded, lease, config)
    try:
        if publisher is not None:
            await publisher.activate()
    except StaleAttemptError:
        return
    except RedisUnavailableError:
        if ownership["active"]:
            await _commit_failure(
                loaded,
                lease,
                dependencies,
                error_code=REDIS_FAILURE_CODE,
                error_message=REDIS_FAILURE_MESSAGE,
            )
        return
    await _execute_agent(loaded, lease, dependencies, config, ownership, publisher)


async def _execute_agent(  # noqa: PLR0913
    run: Run,
    lease: RunLease,
    dependencies: WorkerDependencies,
    config: WorkerConfig,
    ownership: dict[str, bool],
    publisher: _RunStreamPublisher | None,
) -> None:
    """Run the structured AgentRunner while retaining the lease fence."""
    runner = dependencies.agent_runner
    outcome: AgentRunOutcome | None = None
    with anyio.CancelScope() as execution_scope:
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(_renew, lease, dependencies, config, ownership, execution_scope)
            try:
                outcome = await runner.execute(run, lease)
            except (RuntimeError, ValueError):
                outcome = AgentFailed(_internal_agent_failure())
            finally:
                tasks.cancel_scope.cancel()
    if not ownership["active"]:
        return
    if outcome is None:
        outcome = AgentFailed(_internal_agent_failure())
    accepted = await _commit_agent(run, lease, dependencies, outcome)
    if not accepted:
        return
    try:
        await _publish_agent_outcome(publisher, outcome, dependencies.clock())
    except (StaleAttemptError, RedisUnavailableError):
        # PostgreSQL already owns the accepted outcome; transient failure cannot rewrite it.
        return


def _internal_agent_failure() -> AgentFailure:
    """Return a stable failure without exposing runner exception details."""
    return AgentFailure(kind=FailureKind.INTERNAL, message="AgentRunner 执行失败")


async def _commit_agent(
    run: Run,
    lease: RunLease,
    dependencies: WorkerDependencies,
    outcome: AgentRunOutcome,
) -> bool:
    """Commit the fenced outcome before allowing any transient terminal publication."""
    now = dependencies.clock()
    command_id = make_command_id(run, lease.attempt)
    result: TerminalResult | None = None
    failure = outcome.failure if isinstance(outcome, AgentFailed) else None
    if isinstance(outcome, AgentSucceeded):
        status = RunStatus.SUCCEEDED
        events = (
            RunSucceeded(
                event_id(run, "agent_succeeded"), command_id, now, run.id, lease.revision + 1
            ),
        )
        result = outcome.terminal_result()
    elif isinstance(outcome, AgentWaitingForApproval):
        status = RunStatus.WAITING_INPUT
        interaction_id = InteractionId(outcome.request.interaction_id)
        events = (
            ApprovalRequiredEvent(
                event_id(run, "approval_required"),
                command_id,
                now,
                run.id,
                lease.revision + 1,
                interaction_id,
            ),
        )
    else:
        status = RunStatus.FAILED
        events = (
            RunFailed(event_id(run, "agent_failed"), command_id, now, run.id, lease.revision + 1),
        )
    guard = LeaseGuard(run.id, lease.owner, lease.token, lease.revision, now)
    committed = await finish_persistence(
        dependencies.terminal_committer.commit(
            _commit_request(
                run,
                guard,
                status,
                events,
                result=result,
                error_code=failure.kind.value if failure is not None else None,
                error_message=failure.message if failure is not None else None,
                interaction_payload=(
                    outcome.request.model_dump(mode="json")
                    if isinstance(outcome, AgentWaitingForApproval)
                    else None
                ),
                interaction_kind=(
                    outcome.request.kind
                    if isinstance(outcome, AgentWaitingForApproval)
                    else "confirmation"
                ),
            )
        )
    )
    return isinstance(committed, TerminalCommitted)


async def _renew(
    lease: RunLease,
    dependencies: WorkerDependencies,
    config: WorkerConfig,
    ownership: dict[str, bool],
    execution_scope: anyio.CancelScope,
) -> None:
    """按安全比例续租并区分取消与丢租."""
    interval = config.lease_duration.total_seconds() * config.renewal_fraction
    current = lease
    while True:
        await anyio.sleep(interval)
        now = dependencies.clock()
        guard = LeaseGuard(current.run_id, current.owner, current.token, current.revision, now)
        result = await finish_persistence(
            dependencies.repository.renew(RenewRequest(guard, config.lease_duration))
        )
        match result:  # noqa: RUF100  # noqa: MATCH_OK - RenewResult 静态闭集已完整处理。
            case Renewed(lease=renewed):
                current = renewed
            case CancellationRequested():
                ownership["active"] = False
                execution_scope.cancel()
                return
            case LeaseLost():
                ownership["active"] = False
                execution_scope.cancel()
                return


def _commit_request(  # noqa: PLR0913 - terminal request fields mirror the atomic contract.
    run: Run,
    guard: LeaseGuard,
    status: RunStatus,
    events: tuple[DomainEvent, ...],
    *,
    result: TerminalResult | None = None,
    error_code: str | None = None,
    error_message: str | None = None,
    interaction_payload: dict[str, object] | None = None,
    interaction_kind: str = "confirmation",
) -> TerminalCommitRequest:
    """构造同 Run 的终态、事件与清洗审计事实."""
    material = f"{run.id}:{run.attempt}:{status.value}"
    audit = AuditFact(
        f"audit-worker-{sha256(material.encode()).hexdigest()[:20]}",
        run.id,
        run.owner_user_id,
        # Worker event correlation is synthetic, not a persisted user command FK.
        None,
        None,
        "worker_terminal",
        status.value,
        guard.now,
        AuditMetadata("source", "worker"),
    )
    terminal = events[-1]
    pending = terminal.interaction_id if isinstance(terminal, ApprovalRequiredEvent) else None
    return TerminalCommitRequest(
        CompletionRequest(guard, status, pending_interaction_id=pending),
        AppendRequest(run.id, events),
        audit,
        result,
        error_code,
        error_message,
        interaction_payload,
        interaction_kind,
    )


async def _commit_failure(
    run: Run,
    lease: RunLease,
    dependencies: WorkerDependencies,
    *,
    error_code: str | None = None,
    error_message: str | None = None,
) -> None:
    """把已清洗运行时失败收敛为唯一围栏终态."""
    now = dependencies.clock()
    command_id = make_command_id(run, lease.attempt)
    events: tuple[DomainEvent, ...] = (
        RunFailed(
            event_id(run, "runtime_failed"),
            command_id,
            now,
            run.id,
            lease.revision + 1,
        ),
    )
    guard = LeaseGuard(run.id, lease.owner, lease.token, lease.revision, now)
    _ = await finish_persistence(
        dependencies.terminal_committer.commit(
            _commit_request(
                run,
                guard,
                RunStatus.FAILED,
                events,
                error_code=error_code,
                error_message=error_message,
            )
        )
    )
