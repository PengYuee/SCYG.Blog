"""单个围栏 Run 的执行、续租与原子终态提交."""

from dataclasses import replace
from hashlib import sha256

import anyio

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
    TerminalCancellationRequested,
    TerminalCommitRequest,
    TerminalCommitted,
    TerminalEventConflict,
    TerminalLeaseLost,
    TerminalReplay,
    TerminalResult,
    TerminalStateConflict,
)
from scyg_agent.domain.runs import (
    ApprovalRequiredEvent,
    CommandId,
    DomainEvent,
    InteractionId,
    Run,
    RunFailed,
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
from scyg_agent.runtimes.base import AdapterIdentityDriftError
from scyg_agent.runtimes.deep.models import DeepRuntimeError
from scyg_agent.runtimes.normalization import (
    NormalizationContext,
    NormalizationError,
    normalize_runtime,
)
from scyg_agent.runtimes.outputs import RuntimeNativeOutput
from scyg_agent.runtimes.router import RuntimeRouter, RuntimeSelectionMismatch

from .config import WorkerConfig
from .contracts import WorkerDependencies
from .identity import command_id as make_command_id
from .identity import event_id
from .persistence import finish_persistence
from .terminal_events import cancel_events, completion_status


async def execute_lease(
    lease: RunLease,
    dependencies: WorkerDependencies,
    router: RuntimeRouter,
    config: WorkerConfig,
) -> None:
    """并行执行和续租, 丢租后不提交."""
    loaded = await finish_persistence(dependencies.repository.get(lease.run_id))
    if not isinstance(loaded, Run):
        return
    route = router.resolve_for_resume(loaded)
    if isinstance(route, RuntimeSelectionMismatch):
        await _commit_failure(loaded, lease, dependencies)
        return
    ownership: dict[str, bool] = {
        "active": True,
        "cancelled": lease.cancellation_requested_at is not None,
    }
    if dependencies.agent_runner is not None:
        await _execute_agent(loaded, lease, dependencies, config, ownership)
        return
    outputs: list[RuntimeNativeOutput] = []
    runtime_failed = False
    if not ownership["cancelled"]:
        with anyio.CancelScope() as execution_scope:
            async with anyio.create_task_group() as tasks:
                _ = tasks.start_soon(
                    _renew, lease, dependencies, config, ownership, execution_scope
                )
                stream = route.execute(loaded) if lease.attempt == 1 else route.resume(loaded)
                try:
                    outputs.extend([output async for output in stream])
                except (
                    AdapterIdentityDriftError,
                    DeepRuntimeError,
                    NormalizationError,
                    TimeoutError,
                ):
                    runtime_failed = True
                finally:
                    tasks.cancel_scope.cancel()
    if runtime_failed:
        if ownership["active"]:
            await _commit_failure(loaded, lease, dependencies)
        return
    if not ownership["active"]:
        return
    try:
        await _commit(
            loaded,
            lease,
            dependencies,
            outputs,
            cancelled=ownership["cancelled"],
        )
    except NormalizationError:
        await _commit_failure(loaded, lease, dependencies)


async def _commit(
    run: Run,
    lease: RunLease,
    dependencies: WorkerDependencies,
    outputs: list[RuntimeNativeOutput],
    *,
    cancelled: bool,
) -> None:
    """规范化一次并处理取消优先竞态."""
    now = dependencies.clock()
    command_id = make_command_id(run, lease.attempt)
    if cancelled:
        status = RunStatus.CANCELLED
        events = cancel_events(run, lease, command_id, now)
    else:
        normalized_run = replace(run, revision=lease.revision + 1, updated_at=now)
        events = normalize_runtime(
            normalized_run, NormalizationContext(command_id, now), tuple(outputs)
        )
        status = completion_status(events[-1])
        if status is None:
            return
    guard = LeaseGuard(run.id, lease.owner, lease.token, lease.revision, now)
    result = await finish_persistence(
        dependencies.terminal_committer.commit(
            _commit_request(run, guard, status, events, command_id)
        )
    )
    match result:  # noqa: RUF100  # noqa: MATCH_OK - TerminalCommitResult 静态闭集已完整处理。
        case TerminalCancellationRequested():
            _ = await finish_persistence(
                dependencies.terminal_committer.commit(
                    _commit_request(
                        run,
                        guard,
                        RunStatus.CANCELLED,
                        cancel_events(run, lease, command_id, now),
                        command_id,
                    )
                )
            )
        case (
            TerminalLeaseLost()
            | TerminalCommitted()
            | TerminalReplay()
            | TerminalStateConflict()
            | TerminalEventConflict()
        ):
            return


async def _execute_agent(
    run: Run,
    lease: RunLease,
    dependencies: WorkerDependencies,
    config: WorkerConfig,
    ownership: dict[str, bool],
) -> None:
    """Run the structured AgentRunner while retaining the lease fence."""
    runner = dependencies.agent_runner
    if runner is None:
        return
    outcome: AgentRunOutcome | None = None
    with anyio.CancelScope() as execution_scope:
        async with anyio.create_task_group() as tasks:
            _ = tasks.start_soon(_renew, lease, dependencies, config, ownership, execution_scope)
            try:
                if not ownership["cancelled"]:
                    outcome = await runner.execute(run, lease)
            except (RuntimeError, ValueError):
                outcome = AgentFailed(_internal_agent_failure())
            finally:
                tasks.cancel_scope.cancel()
    if not ownership["active"]:
        return
    if outcome is None:
        outcome = AgentFailed(_internal_agent_failure())
    await _commit_agent(run, lease, dependencies, outcome, cancelled=ownership["cancelled"])


def _internal_agent_failure() -> AgentFailure:
    """Return a stable failure without exposing runner exception details."""
    return AgentFailure(kind=FailureKind.INTERNAL, message="AgentRunner 执行失败")


async def _commit_agent(
    run: Run,
    lease: RunLease,
    dependencies: WorkerDependencies,
    outcome: AgentRunOutcome,
    *,
    cancelled: bool,
) -> None:
    """Map a closed AgentRunner outcome to one fenced terminal commit."""
    now = dependencies.clock()
    command_id = make_command_id(run, lease.attempt)
    result: TerminalResult | None = None
    failure: AgentFailure | None = (
        outcome.failure if not cancelled and isinstance(outcome, AgentFailed) else None
    )
    if cancelled:
        status = RunStatus.CANCELLED
        events = cancel_events(run, lease, command_id, now)
    elif isinstance(outcome, AgentSucceeded):
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
    _ = await finish_persistence(
        dependencies.terminal_committer.commit(
            _commit_request(
                run,
                guard,
                status,
                events,
                command_id,
                result=result,
                error_code=failure.kind.value if failure is not None else None,
                error_message=failure.message if failure is not None else None,
            )
        )
    )


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
                ownership["cancelled"] = True
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
    command_id: CommandId,
    *,
    result: TerminalResult | None = None,
    error_code: str | None = None,
    error_message: str | None = None,
) -> TerminalCommitRequest:
    """构造同 Run 的终态、事件与清洗审计事实."""
    material = f"{run.id}:{run.attempt}:{status.value}"
    audit = AuditFact(
        f"audit-worker-{sha256(material.encode()).hexdigest()[:20]}",
        run.id,
        run.owner_user_id,
        command_id,
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
    )


async def _commit_failure(
    run: Run,
    lease: RunLease,
    dependencies: WorkerDependencies,
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
    result = await finish_persistence(
        dependencies.terminal_committer.commit(
            _commit_request(run, guard, RunStatus.FAILED, events, command_id)
        )
    )
    if isinstance(result, TerminalCancellationRequested):
        _ = await finish_persistence(
            dependencies.terminal_committer.commit(
                _commit_request(
                    run,
                    guard,
                    RunStatus.CANCELLED,
                    cancel_events(run, lease, command_id, now),
                    command_id,
                )
            )
        )
