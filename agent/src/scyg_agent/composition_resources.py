"""生产数据库、迁移和检查点资源组件."""

from pathlib import Path
from typing import final

from alembic.config import Config
from alembic.script import ScriptDirectory
from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncEngine

from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.adapters.langgraph.values import IncompatibleCheckpointSchema
from scyg_agent.lifecycle import ComponentDiagnostic


@final
class DatabaseResource:
    """拥有共享 Agent 数据库引擎."""

    def __init__(self, engine: AsyncEngine) -> None:
        """接管尚未连接的异步引擎."""
        self.engine, self._opened = engine, False

    @property
    def name(self) -> str:
        """返回脱敏组件名."""
        return "database"

    async def start(self) -> None:
        """通过只读查询验证数据库连接."""
        await self._ping()
        self._opened = True

    async def stop(self) -> None:
        """释放全部数据库连接."""
        await self.engine.dispose()
        self._opened = False

    async def probe(self) -> ComponentDiagnostic:
        """实时验证数据库连接状态."""
        ready = False
        if self._opened:
            try:
                await self._ping()
                ready = True
            except (OSError, RuntimeError):
                ready = False
        return ComponentDiagnostic(self.name, ready, "已就绪" if ready else "未就绪")

    async def _ping(self) -> None:
        async with self.engine.connect() as connection:
            _ = await connection.execute(text("SELECT 1"))


@final
class MigrationHeadResource:
    """只读验证数据库等于代码唯一 Alembic 头."""

    def __init__(self, engine: AsyncEngine, configuration: Path) -> None:
        """解析唯一代码迁移头且不连接数据库."""
        heads = ScriptDirectory.from_config(Config(configuration)).get_heads()
        if len(heads) != 1:
            message = "迁移脚本必须只有一个迁移头"
            raise RuntimeError(message)
        self._engine, self._head, self._current = engine, heads[0], False

    @property
    def name(self) -> str:
        """返回脱敏组件名."""
        return "migration"

    async def start(self) -> None:
        """拒绝落后、缺失或分叉迁移且不自动升级."""
        self._current = await self._is_current()
        if not self._current:
            message = "Agent 数据库迁移未处于当前版本"
            raise RuntimeError(message)

    async def stop(self) -> None:
        """撤销借用资源的迁移就绪状态."""
        self._current = False

    async def probe(self) -> ComponentDiagnostic:
        """实时比较迁移表和代码头."""
        ready = self._current and await self._is_current()
        return ComponentDiagnostic(self.name, ready, "已就绪" if ready else "版本不匹配")

    async def _is_current(self) -> bool:
        async with self._engine.connect() as connection:
            result = await connection.execute(text("SELECT version_num FROM alembic_version"))
            versions = tuple(result.scalars())
        return versions == (self._head,)


@final
class CheckpointResource:
    """拥有隔离 T13 池并只读验证 checkpoint schema."""

    def __init__(self, store: CheckpointStore) -> None:
        """接管尚未打开的检查点存储."""
        self.store, self._compatible = store, False

    @property
    def name(self) -> str:
        """返回脱敏组件名."""
        return "checkpoint"

    async def start(self) -> None:
        """打开专用池并验证 schema,不执行 setup."""
        await self.store.open()
        _ = await self.store.readiness()
        self._compatible = True

    async def stop(self) -> None:
        """关闭专用检查点池."""
        await self.store.close()
        self._compatible = False

    async def probe(self) -> ComponentDiagnostic:
        """实时执行 T13 兼容性探针."""
        ready = False
        if self._compatible:
            try:
                _ = await self.store.readiness()
                ready = True
            except IncompatibleCheckpointSchema:
                ready = False
        return ComponentDiagnostic(self.name, ready, "已就绪" if ready else "schema 不兼容")
