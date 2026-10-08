"""LangGraph checkpoint adapter contract tests."""

from collections.abc import Callable
from dataclasses import FrozenInstanceError

import pytest
from pydantic import SecretStr

from scyg_agent.adapters.langgraph.checkpointer import (
    CheckpointStore,
    SchemaProbe,
    schema_inventory_from_probe,
)
from scyg_agent.adapters.langgraph.metadata import (
    CheckpointCompatibility,
    CompatibleCheckpointMetadata,
    IncompatibleCheckpoint,
    checkpoint_metadata,
    ensure_checkpoint_compatible,
    parse_checkpoint_metadata,
)
from scyg_agent.adapters.langgraph.values import (
    CheckpointerConfig,
    CheckpointThreadId,
    DependencyVersions,
    IncompatibleCheckpointSchema,
    InvalidCheckpointerConfigError,
    SchemaInventory,
    ensure_schema_compatible,
    thread_id_for_run,
)
from scyg_agent.domain.runs import InvalidIdentifierError, RunId


@pytest.fixture
def anyio_backend() -> str:
    """固定项目已安装的 asyncio 后端."""
    return "asyncio"


VERSIONS = DependencyVersions(
    langgraph="1.2.11",
    checkpoint="4.1.1",
    checkpoint_postgres="3.1.2",
    psycopg="3.3.2",
    psycopg_pool="3.3.1",
)


def test_config_is_frozen_bounded_and_secret_free() -> None:
    # Given: 经过边界解析的专用检查点配置.
    config = CheckpointerConfig(
        dsn=SecretStr("postgresql://agent:database-secret@localhost/agent"),
        min_pool_size=1,
        max_pool_size=3,
        pool_timeout_seconds=4,
        operation_timeout_seconds=5,
        close_timeout_seconds=6,
        versions=VERSIONS,
    )
    # When: 配置进入诊断表面.
    rendered = repr(config)
    # Then: 边界被保留且凭据不可见.
    assert config.schema_name == "langgraph"
    assert config.max_pool_size == 3
    assert "database-secret" not in rendered
    with pytest.raises(FrozenInstanceError):
        config.__setattr__("max_pool_size", 4)


@pytest.mark.parametrize(
    "construct",
    [
        lambda: CheckpointerConfig(
            dsn=SecretStr("postgresql://agent:secret@localhost/agent"),
            schema_name="langgraph; DROP SCHEMA public",
        ),
        lambda: CheckpointerConfig(
            dsn=SecretStr("postgresql://agent:secret@localhost/agent"), min_pool_size=0
        ),
        lambda: CheckpointerConfig(
            dsn=SecretStr("postgresql://agent:secret@localhost/agent"), max_pool_size=0
        ),
        lambda: CheckpointerConfig(
            dsn=SecretStr("postgresql://agent:secret@localhost/agent"),
            pool_timeout_seconds=0,
        ),
    ],
)
def test_config_rejects_malformed_values(
    construct: Callable[[], CheckpointerConfig],
) -> None:
    # Given/When: 不可信配置进入公开构造边界.
    with pytest.raises(InvalidCheckpointerConfigError) as caught:
        _ = construct()

    # Then: 仅暴露稳定且无秘密的领域错误.
    assert type(caught.value) is InvalidCheckpointerConfigError
    assert str(caught.value) == "LangGraph 检查点配置无效"
    assert "secret" not in repr(caught.value)
    assert caught.value.__cause__ is None
    assert caught.value.__context__ is None


def test_thread_id_is_stable_short_and_reversible() -> None:
    # Given: 达到 RunId 上限的合法标识.
    run_id = RunId("run_" + "A" * 64)
    # When: 转换并恢复线程标识.
    thread_id = thread_id_for_run(run_id)
    # Then: 映射稳定, 缩短且没有截断.
    assert str(thread_id) == "A" * 64
    assert thread_id.to_run_id() == run_id
    assert len(str(thread_id)) < len(str(run_id))


def test_metadata_fingerprint_rejects_runtime_or_dependency_drift() -> None:
    # Given: 绑定运行时和依赖指纹的检查点.
    expected = CheckpointCompatibility(
        runtime_kind="deep", runtime_version="v1", dependencies=VERSIONS
    )
    metadata = checkpoint_metadata(expected)
    # When/Then: 相同契约可恢复, 依赖漂移被拒绝.
    ensure_checkpoint_compatible(metadata, expected)
    changed_versions = DependencyVersions(
        langgraph=VERSIONS.langgraph,
        checkpoint=VERSIONS.checkpoint,
        checkpoint_postgres=VERSIONS.checkpoint_postgres,
        psycopg="9.9.9",
        psycopg_pool=VERSIONS.psycopg_pool,
    )
    changed = CheckpointCompatibility(
        runtime_kind="deep", runtime_version="v1", dependencies=changed_versions
    )
    with pytest.raises(IncompatibleCheckpoint):
        ensure_checkpoint_compatible(metadata, changed)


def test_schema_readiness_requires_exact_inventory_and_versions() -> None:
    # Given: 官方 schema 的完整清单.
    inventory = SchemaInventory(
        schema_name="langgraph",
        tables=frozenset(
            {
                "checkpoint_migrations",
                "checkpoints",
                "checkpoint_blobs",
                "checkpoint_writes",
                "scyg_checkpoint_metadata",
            }
        ),
        indexes=frozenset(
            {
                "checkpoints_thread_id_idx",
                "checkpoint_blobs_thread_id_idx",
                "checkpoint_writes_thread_id_idx",
            }
        ),
        migration_versions=tuple(range(10)),
        metadata_versions=VERSIONS,
    )
    # When/Then: 完整清单通过, 缺表稳定失败.
    ensure_schema_compatible(inventory, VERSIONS)
    missing = inventory.model_copy(update={"tables": inventory.tables - {"checkpoints"}})
    with pytest.raises(IncompatibleCheckpointSchema) as caught:
        ensure_schema_compatible(missing, VERSIONS)
    assert str(caught.value) == "LangGraph 检查点 schema 不兼容"


@pytest.mark.anyio
async def test_store_constructs_and_closes_dedicated_pool_without_database() -> None:
    # Given: 一个尚未打开的专用池配置.
    store = CheckpointStore(
        CheckpointerConfig(
            dsn=SecretStr("postgresql://agent:secret@localhost/agent"), versions=VERSIONS
        )
    )
    # When: 未打开的适配器执行幂等清理.
    await store.close()
    await store.__aexit__(None, None, None)
    # Then: 生命周期完成且诊断表面不包含秘密.
    assert "secret" not in repr(store)
    assert store.is_closed


def test_schema_probe_maps_to_typed_inventory_without_database() -> None:
    # Given: readiness SQL 的完整类型化结果.
    row = SchemaProbe(
        tables=[
            "checkpoint_migrations",
            "checkpoints",
            "checkpoint_blobs",
            "checkpoint_writes",
            "scyg_checkpoint_metadata",
        ],
        indexes=[
            "checkpoints_thread_id_idx",
            "checkpoint_blobs_thread_id_idx",
            "checkpoint_writes_thread_id_idx",
        ],
        migrations=list(range(10)),
        langgraph=VERSIONS.langgraph,
        checkpoint=VERSIONS.checkpoint,
        checkpoint_postgres=VERSIONS.checkpoint_postgres,
        psycopg=VERSIONS.psycopg,
        psycopg_pool=VERSIONS.psycopg_pool,
    )
    # When: 行被映射为冻结清单.
    inventory = schema_inventory_from_probe(row)
    # Then: 版本与对象集合保持精确.
    assert inventory.metadata_versions == VERSIONS
    assert inventory.migration_versions == tuple(range(10))


def test_value_error_surfaces_and_metadata_parser_are_typed() -> None:
    # Given: 外部 metadata 和包含非法字符的线程值.
    expected = CheckpointCompatibility(
        runtime_kind="deep", runtime_version="v1", dependencies=VERSIONS
    )
    raw: CompatibleCheckpointMetadata = {
        "source": "input",
        "step": 1,
        "parents": {},
        **checkpoint_metadata(expected),
    }
    # When: metadata 通过边界解析.
    parsed = parse_checkpoint_metadata(raw)
    # Then: 类型保留, 错误诊断稳定且不泄露输入.
    assert parsed == checkpoint_metadata(expected)
    assert str(InvalidCheckpointerConfigError()) == "LangGraph 检查点配置无效"
    assert str(IncompatibleCheckpoint()) == "LangGraph 检查点运行时不兼容"
    assert CheckpointThreadId("bad").to_run_id() == RunId("run_bad")
    with pytest.raises(InvalidIdentifierError):
        _ = CheckpointThreadId("bad/path").to_run_id()


def test_config_rejects_reversed_pool_bounds() -> None:
    # Given/When/Then: 最小池大于最大池时边界解析失败.
    with pytest.raises(InvalidCheckpointerConfigError):
        _ = CheckpointerConfig(
            dsn=SecretStr("postgresql://agent:secret@localhost/agent"),
            min_pool_size=3,
            max_pool_size=2,
            versions=VERSIONS,
        )
