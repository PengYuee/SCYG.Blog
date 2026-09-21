"""真实 PostgreSQL 的 LangGraph 检查点修复验收."""

from typing import TYPE_CHECKING

import pytest
from langgraph.checkpoint.base import empty_checkpoint
from psycopg import AsyncConnection
from pydantic import SecretStr

from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.adapters.langgraph.metadata import (
    CheckpointCompatibility,
    CompatibleCheckpointMetadata,
    checkpoint_metadata,
    ensure_checkpoint_compatible,
    parse_checkpoint_metadata,
)
from scyg_agent.adapters.langgraph.values import (
    CheckpointerConfig,
    IncompatibleCheckpointSchema,
    thread_id_for_run,
)
from scyg_agent.domain.runs import RunId
from tests.acceptance_settings import require_test_settings

if TYPE_CHECKING:
    from langchain_core.runnables import RunnableConfig


def _environment(name: str) -> str:
    """读取统一验收配置中的安全端点."""
    settings = require_test_settings()
    if name == "SCYG_T13_DATABASE_URL":
        return settings.normal_url
    if name == "SCYG_T13_ADMIN_DATABASE_URL":
        return settings.require_admin_url()
    raise ValueError(name)


def _config() -> CheckpointerConfig:
    """构造不泄密的共享应用池配置."""
    return CheckpointerConfig(
        dsn=SecretStr(_environment("SCYG_T13_DATABASE_URL")),
        min_pool_size=1,
        max_pool_size=2,
        close_timeout_seconds=2,
    )


async def _admin() -> AsyncConnection[tuple[str, ...]]:
    """打开仅用于隔离验收破坏与恢复的管理员连接."""
    return await AsyncConnection.connect(
        _environment("SCYG_T13_ADMIN_DATABASE_URL"), autocommit=True
    )


@pytest.mark.anyio
async def test_checkpoint_recovers_after_restart_and_closes_every_session() -> None:
    # Given: 重复 setup 后写入带兼容 metadata 的状态.
    config = _config()
    runnable: RunnableConfig = {
        "configurable": {
            "thread_id": str(thread_id_for_run(RunId("run_t13fixrestart"))),
            "checkpoint_ns": "deep.v1",
        }
    }
    compatibility = CheckpointCompatibility(
        runtime_kind="deep", runtime_version="v1", dependencies=config.versions
    )
    metadata: CompatibleCheckpointMetadata = {
        "source": "input",
        "step": 1,
        "parents": {},
        **checkpoint_metadata(compatibility),
    }
    checkpoint = empty_checkpoint()
    checkpoint["channel_values"] = {"state": "restart-recovered"}
    checkpoint["channel_versions"] = {"state": "1"}
    async with CheckpointStore(config) as first:
        await first.setup()
        await first.setup()
        async with first.saver() as saver:
            _ = await saver.aput(runnable, checkpoint, metadata, {"state": "1"})

    # When: 适配器和专用池被完整重建.
    async with CheckpointStore(config) as restarted:
        _ = await restarted.readiness()
        async with restarted.saver() as saver:
            recovered = await saver.aget_tuple(runnable)

    # Then: 状态恢复, 官方 schema 与依赖元数据保持兼容, 连接池已关闭.
    assert recovered is not None
    assert recovered.checkpoint["channel_values"]["state"] == "restart-recovered"
    ensure_checkpoint_compatible(parse_checkpoint_metadata(recovered.metadata), compatibility)
    admin = await _admin()
    try:
        result = await admin.execute(
            "SELECT count(*) FROM pg_stat_activity WHERE application_name=%s",
            ("scyg-langgraph-checkpoints",),
        )
        assert (await result.fetchone()) == (0,)
    finally:
        await admin.close()


@pytest.mark.anyio
async def test_setup_never_overwrites_conflicting_version_metadata() -> None:
    # Given: 已部署 metadata 被管理员改成不兼容版本.
    config = _config()
    admin = await _admin()
    try:
        _ = await admin.execute(
            "UPDATE langgraph.scyg_checkpoint_metadata SET psycopg=%s WHERE key=%s",
            ("0.0.0", "scyg-runtime-contract"),
        )
        async with CheckpointStore(config) as store:
            with pytest.raises(IncompatibleCheckpointSchema):
                _ = await store.readiness()

            # When: 部署 setup 再次执行.
            await store.setup()

            # Then: ON CONFLICT 不覆盖冲突, readiness 仍类型化失败.
            with pytest.raises(IncompatibleCheckpointSchema):
                _ = await store.readiness()
        _ = await admin.execute(
            "UPDATE langgraph.scyg_checkpoint_metadata SET psycopg=%s WHERE key=%s",
            (config.versions.psycopg, "scyg-runtime-contract"),
        )
    finally:
        await admin.close()


@pytest.mark.anyio
async def test_missing_table_fails_before_saver() -> None:
    # Given: 必需检查点表随后被删除.
    config = _config()
    admin = await _admin()
    try:
        _ = await admin.execute("DROP TABLE langgraph.checkpoints")
    finally:
        await admin.close()

    # When/Then: readiness 和 saver 均类型化失败且不修复 schema.
    async with CheckpointStore(config) as store:
        with pytest.raises(IncompatibleCheckpointSchema):
            _ = await store.readiness()
        with pytest.raises(IncompatibleCheckpointSchema):
            async with store.saver():
                pytest.fail("不兼容 schema 不得借出 saver")
