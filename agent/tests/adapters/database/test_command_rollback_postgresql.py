from dataclasses import dataclass

import anyio
import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.command_failpoints import (
    CommandStage,
    InjectedCommandFailureError,
)
from scyg_agent.adapters.database.command_store import PostgreSQLCommandStore
from scyg_agent.adapters.database.run_records import RunRecord

from .t12_postgres_support import (
    RUN_ID,
    RowCounts,
    row_counts,
    seed_run,
    submission,
)


@dataclass(frozen=True, slots=True)
class FailAt:
    """在一个确定事务阶段抛出类型化故障。"""

    target: CommandStage

    async def reach(self, stage: CommandStage) -> None:
        """只在目标阶段中断事务。"""
        if stage is self.target:
            raise InjectedCommandFailureError(stage)


class PauseAt:
    """在目标阶段挂起直到测试取消任务。"""

    def __init__(self, target: CommandStage) -> None:
        """保存目标阶段和到达信号。"""
        self.target: CommandStage = target
        self.reached: anyio.Event = anyio.Event()

    async def reach(self, stage: CommandStage) -> None:
        """到达目标阶段后保持事务打开。"""
        if stage is self.target:
            self.reached.set()
            await anyio.sleep_forever()


@pytest.mark.anyio
@pytest.mark.parametrize("stage", list(CommandStage))
async def test_every_command_stage_failure_rolls_back_all_tables(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
    stage: CommandStage,
) -> None:
    # Given: 一个初始 Run 和在指定阶段失败的命令存储。
    _, sessions = t12_database
    await seed_run(sessions)
    store = PostgreSQLCommandStore(sessions, "scyg_t12_events", FailAt(stage))
    before = RowCounts(1, 0, 0, 0, 0, 0)

    # When: 命令事务越过若干写入后触发故障。
    with pytest.raises(InjectedCommandFailureError) as captured:
        _ = await store.apply(submission(f"t12rb{list(CommandStage).index(stage):02d}00"))

    # Then: 故障阶段准确且 Run/事件/命令/审计全部回滚。
    assert captured.value.stage is stage
    async with sessions() as session:
        assert await row_counts(session) == before
        run = (
            await session.execute(select(RunRecord).where(RunRecord.run_id == str(RUN_ID)))
        ).scalar_one()
        assert (run.revision, run.status) == (1, "pending")


@pytest.mark.anyio
async def test_cancellation_with_open_transaction_rolls_back_and_releases_connection(
    t12_database: tuple[AsyncEngine, async_sessionmaker[AsyncSession]],
) -> None:
    # Given: 命令已更新 Run 后暂停且事务仍打开。
    _, sessions = t12_database
    await seed_run(sessions)
    pause = PauseAt(CommandStage.RUN_UPDATED)
    store = PostgreSQLCommandStore(sessions, "scyg_t12_events", pause)

    async def apply_until_cancelled() -> None:
        """运行命令直到外层结构化取消。"""
        _ = await store.apply(submission("t12cancel0"))

    # When: 确认事务进入写后阶段再取消任务。
    async with anyio.create_task_group() as task_group:
        _ = task_group.start_soon(apply_until_cancelled)
        with anyio.fail_after(5):
            await pause.reached.wait()
        task_group.cancel_scope.cancel()

    # Then: 新会话可立即获得连接, 所有写入回滚且没有悬挂任务。
    with anyio.fail_after(5):
        async with sessions() as session:
            assert await row_counts(session) == RowCounts(1, 0, 0, 0, 0, 0)
            run = (await session.execute(select(RunRecord))).scalar_one()
            assert (run.revision, run.status) == (1, "pending")
