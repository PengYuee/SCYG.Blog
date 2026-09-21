"""LangGraph PostgreSQL checkpoint 适配器."""

from __future__ import annotations

from contextlib import asynccontextmanager
from dataclasses import dataclass
from typing import TYPE_CHECKING, Self, final, override

import anyio
from langgraph.checkpoint.postgres.aio import AsyncPostgresSaver
from psycopg import AsyncConnection
from psycopg import Error as PsycopgError
from psycopg.rows import DictRow, class_row, dict_row
from psycopg_pool import AsyncConnectionPool, PoolTimeout

from .schema_sql import CREATE_METADATA_SQL, INSERT_METADATA_SQL, READINESS_SQL
from .values import (
    METADATA_KEY,
    SCHEMA,
    CheckpointerConfig,
    DependencyVersions,
    IncompatibleCheckpointSchema,
    SchemaInventory,
    ensure_schema_compatible,
)

if TYPE_CHECKING:
    from collections.abc import AsyncIterator
    from types import TracebackType


@dataclass(frozen=True, slots=True)
class SchemaProbe:
    """映射单次 readiness SQL 的确定行形状."""

    tables: list[str]
    indexes: list[str]
    migrations: list[int]
    langgraph: str
    checkpoint: str
    checkpoint_postgres: str
    psycopg: str
    psycopg_pool: str


class CheckpointPoolOpenError(RuntimeError):
    """表示已关闭的专用池不能重新打开."""

    @override
    def __str__(self) -> str:
        """返回不携带连接信息的稳定诊断."""
        return "LangGraph 检查点连接池打开失败"


class CheckpointStoreUnavailableError(RuntimeError):
    """表示检查点数据库当前不可用."""

    @override
    def __str__(self) -> str:
        """返回不携带连接信息的稳定诊断."""
        return "LangGraph 检查点存储不可用"


class CheckpointPoolCloseError(RuntimeError):
    """表示专用池未完成清理."""

    @override
    def __str__(self) -> str:
        """返回不携带连接信息的稳定诊断."""
        return "LangGraph 检查点连接池关闭失败"


@final
class CheckpointStore:
    """显式拥有专用 psycopg 异步池及其生命周期."""

    def __init__(self, config: CheckpointerConfig) -> None:
        """创建独立连接池,但使用与 Agent truth 相同的数据库账号."""
        self._config: CheckpointerConfig = config
        self._opened: bool = False
        self._retired: bool = False
        self._pool: AsyncConnectionPool[AsyncConnection[DictRow]] = AsyncConnectionPool(
            config.dsn.get_secret_value(),
            min_size=config.min_pool_size,
            max_size=config.max_pool_size,
            timeout=config.pool_timeout_seconds,
            open=False,
            name="scyg-langgraph-checkpoints",
            kwargs={
                "autocommit": True,
                "prepare_threshold": 0,
                "row_factory": dict_row,
                "options": (
                    f"-c search_path={SCHEMA} -c application_name=scyg-langgraph-checkpoints"
                ),
            },
        )

    @property
    def is_closed(self) -> bool:
        """报告专用池是否已完全关闭."""
        return self._pool.closed

    async def open(self) -> None:
        """幂等打开专用池并翻译连接错误."""
        if self._opened:
            return
        if self._retired:
            raise CheckpointPoolOpenError
        try:
            await self._pool.open(wait=True, timeout=self._config.pool_timeout_seconds)
            self._opened = True
        except (PsycopgError, PoolTimeout):
            cleaned = await self._cleanup_failed_open()
            unavailable = CheckpointStoreUnavailableError()
            if not cleaned:
                unavailable.add_note("检查点连接池清理未完成")
            raise unavailable from None
        except OSError as error:
            if not await self._cleanup_failed_open():
                error.add_note("检查点连接池清理未完成")
            raise
        except anyio.get_cancelled_exc_class() as error:
            if not await self._cleanup_failed_open():
                error.add_note("检查点连接池清理未完成")
            raise

    async def _cleanup_failed_open(self) -> bool:
        """屏蔽取消并验证失败打开路径的池清理."""
        try:
            with anyio.CancelScope(shield=True):
                await self._pool.close(timeout=self._config.close_timeout_seconds)
        except (PsycopgError, PoolTimeout, TimeoutError, OSError, RuntimeError):
            return False
        self._retired = True
        return self._pool.closed

    async def close(self) -> None:
        """不可取消地等待 psycopg 自身有界关闭完成."""
        if self._pool.closed:
            self._opened = False
            self._retired = True
            return
        try:
            with anyio.CancelScope(shield=True):
                await self._pool.close(timeout=self._config.close_timeout_seconds)
        except (PsycopgError, PoolTimeout, TimeoutError, OSError, RuntimeError):
            raise CheckpointPoolCloseError from None
        self._opened = False
        self._retired = True
        if not self._pool.closed:
            raise CheckpointPoolCloseError

    async def setup(self) -> None:
        """部署期幂等执行官方迁移并首次记录精确依赖元数据."""
        try:
            with anyio.fail_after(self._config.operation_timeout_seconds):
                await AsyncPostgresSaver(self._pool).setup()
                async with self._pool.connection() as connection:
                    _ = await connection.execute(CREATE_METADATA_SQL)
                    values = self._config.versions
                    _ = await connection.execute(
                        INSERT_METADATA_SQL,
                        (
                            METADATA_KEY,
                            values.langgraph,
                            values.checkpoint,
                            values.checkpoint_postgres,
                            values.psycopg,
                            values.psycopg_pool,
                        ),
                    )
        except (PsycopgError, PoolTimeout):
            raise CheckpointStoreUnavailableError from None

    async def readiness(self) -> SchemaInventory:
        """只读验证对象, 迁移和依赖版本."""
        try:
            with anyio.fail_after(self._config.operation_timeout_seconds):
                async with self._pool.connection() as connection:
                    async with connection.cursor(row_factory=class_row(SchemaProbe)) as cursor:
                        row = await (
                            await cursor.execute(READINESS_SQL, (METADATA_KEY,))
                        ).fetchone()
        except (TimeoutError, PsycopgError, PoolTimeout):
            raise IncompatibleCheckpointSchema from None
        if row is None:
            raise IncompatibleCheckpointSchema
        inventory = schema_inventory_from_probe(row)
        ensure_schema_compatible(inventory, self._config.versions)
        return inventory

    @asynccontextmanager
    async def saver(self) -> AsyncIterator[AsyncPostgresSaver]:
        """Readiness 成功后向适配器层借出官方 saver."""
        _ = await self.readiness()
        yield AsyncPostgresSaver(self._pool)

    async def __aenter__(self) -> Self:
        """打开适配器, 失败或取消时先完成屏蔽清理."""
        await self.open()
        return self

    async def __aexit__(
        self,
        exception_type: type[BaseException] | None,
        exception: BaseException | None,
        traceback: TracebackType | None,
    ) -> None:
        """无论上下文结果如何都关闭专用池."""
        await self.close()


def schema_inventory_from_probe(row: SchemaProbe) -> SchemaInventory:
    """将数据库探针映射为冻结的兼容性清单."""
    return SchemaInventory(
        schema_name=SCHEMA,
        tables=frozenset(row.tables),
        indexes=frozenset(row.indexes),
        migration_versions=tuple(row.migrations),
        metadata_versions=DependencyVersions(
            langgraph=row.langgraph,
            checkpoint=row.checkpoint,
            checkpoint_postgres=row.checkpoint_postgres,
            psycopg=row.psycopg,
            psycopg_pool=row.psycopg_pool,
        ),
    )
