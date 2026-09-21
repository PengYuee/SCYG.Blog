from datetime import timedelta
from uuid import UUID

import pytest
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.domain.ports.command_store import (
    CommandApplied,
    CommandRejected,
    CommandSubmission,
    UnsupportedCommand,
)
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest
from scyg_agent.domain.runs import (
    CancelRun,
    ClaimRun,
    CommandId,
    CommandKind,
    EventId,
    ExecutionOwnerId,
    FailRun,
    InteractionId,
    ReleaseExecution,
    RequestInput,
    RunCommand,
    RunStatus,
    SubmitInput,
    SucceedRun,
)
from scyg_agent.domain.runs.commands import command_kind

from .t12_postgres_support import NOW, RUN_ID, RowCounts, row_counts

WORKER_KINDS = {
    CommandKind.CLAIM,
    CommandKind.REQUEST_INPUT,
    CommandKind.SUCCEED,
    CommandKind.FAIL,
    CommandKind.RELEASE_EXECUTION,
}


@pytest.mark.anyio
@pytest.mark.parametrize("status", list(RunStatus))
@pytest.mark.parametrize("variant", list(CommandKind))
async def test_every_t08_variant_status_pair_is_typed_and_constraint_safe(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
    status: RunStatus,
    variant: CommandKind,
) -> None:
    # Given: 一个满足数据库状态约束的 Run 和指定 T08 命令。
    _, sessions = t12_database
    async with sessions.begin() as session:
        session.add(_run(status))
    request = _submission(variant)
    store = PostgreSQLCommandStore(sessions, "scyg_t12_events")

    # When: 命令通过无租约公共入口提交。
    result = await store.apply(request)

    # Then: worker 变体及 RUNNING cancel 在写入前拒绝, 其余均为领域结果。
    if variant in WORKER_KINDS or (variant is CommandKind.CANCEL and status is RunStatus.RUNNING):
        assert result == UnsupportedCommand(request.command_id, variant)
        async with sessions() as session:
            assert await row_counts(session) == RowCounts(1, 0, 0, 0, 0, 0)
        return
    assert isinstance(result, CommandApplied | CommandRejected)
    async with sessions() as session:
        counts = await row_counts(session)
    assert counts.commands == 1
    assert counts.audits == 1
    assert counts.events in (0, 1)


def _submission(variant: CommandKind) -> CommandSubmission:
    """构造指定 T08 变体的稳定提交。"""
    suffix = f"t12{list(CommandKind).index(variant):07d}"
    command_id = CommandId(f"cmd_{suffix}")
    event_id = EventId(f"evt_{suffix}")
    common = (command_id, event_id, 1, NOW)
    constructors = {
        CommandKind.CLAIM: lambda: ClaimRun(*common, ExecutionOwnerId("worker_t12catalog")),
        CommandKind.REQUEST_INPUT: lambda: RequestInput(*common, InteractionId("int_t12catalog")),
        CommandKind.SUBMIT_INPUT: lambda: SubmitInput(*common, InteractionId("int_t12catalog")),
        CommandKind.SUCCEED: lambda: SucceedRun(*common),
        CommandKind.FAIL: lambda: FailRun(*common),
        CommandKind.CANCEL: lambda: CancelRun(*common),
        CommandKind.RELEASE_EXECUTION: lambda: ReleaseExecution(*common),
    }
    command: RunCommand = constructors[variant]()
    actual = command_kind(command)
    assert isinstance(actual, CommandKind)
    return CommandSubmission(
        command_id,
        RUN_ID,
        1,
        0,
        actual.value,
        RequestDigest.parse("7" * 64),
        NOW,
        command,
        AuditMetadata("source", "catalog"),
    )


def _run(status: RunStatus) -> RunRecord:
    """构造满足 lease/interaction 数据库约束的状态行。"""
    running = status is RunStatus.RUNNING
    waiting = status is RunStatus.WAITING_INPUT
    return RunRecord(
        run_id=str(RUN_ID),
        owner_user_id="user-t12",
        operation_id="t12:catalog",
        task_type="summary",
        runtime_kind="simple",
        runtime_version="v1",
        revision=1,
        status=status.value,
        created_at=NOW,
        updated_at=NOW,
        attempt=0,
        next_attempt_at=NOW,
        lease_owner="worker_t12catalog" if running else None,
        lease_token=UUID("12345678-1234-5678-9234-567812345678") if running else None,
        lease_expires_at=NOW + timedelta(minutes=5) if running else None,
        pending_interaction_id="int_t12catalog" if waiting else None,
        terminal_at=NOW
        if status in (RunStatus.SUCCEEDED, RunStatus.FAILED, RunStatus.CANCELLED)
        else None,
        terminal_metadata=None,
        error_code=None,
        error_message=None,
        error_metadata=None,
    )
