"""双 Worker PostgreSQL 验收的无密钥超时诊断."""

from dataclasses import dataclass

import anyio
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.journal_records import EventRecord
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.worker import Worker, WorkerState

from .runtime_support import RuntimeProbe


@dataclass(frozen=True, slots=True)
class WorkerQueueTimeoutError(AssertionError):
    """携带不含连接配置的持久化队列快照."""

    statuses: tuple[tuple[str, int], ...]
    event_count: int
    adapter_visits: int
    first_state: WorkerState
    second_state: WorkerState


async def wait_for_completion(
    sessions: async_sessionmaker[AsyncSession],
    probe: RuntimeProbe,
    first: Worker,
    second: Worker,
) -> None:
    """等待合法暂停或终态, 超时时返回可复现的状态计数."""
    for _attempt in range(500):
        if await active_count(sessions) == 0:
            return
        await anyio.sleep(0.02)
    async with sessions() as session:
        statuses = tuple(
            (
                await session.execute(
                    select(RunRecord.status, func.count())
                    .group_by(RunRecord.status)
                    .order_by(RunRecord.status)
                )
            ).tuples()
        )
        event_count = (
            await session.execute(select(func.count()).select_from(EventRecord))
        ).scalar_one()
    raise WorkerQueueTimeoutError(
        statuses,
        event_count,
        len(probe.visits),
        first.state,
        second.state,
    )


async def active_count(sessions: async_sessionmaker[AsyncSession]) -> int:
    """统计仍需 Worker 推进的 Run."""
    async with sessions() as session:
        return (
            await session.execute(
                select(func.count())
                .select_from(RunRecord)
                .where(RunRecord.status.in_(("pending", "running")))
            )
        ).scalar_one()
