"""T13 连接错误脱敏与不可取消关闭测试."""

from typing import final

import anyio
import pytest
from psycopg import OperationalError
from pydantic import SecretStr

from scyg_agent.adapters.langgraph.checkpointer import (
    CheckpointStore,
    CheckpointStoreUnavailableError,
)
from scyg_agent.adapters.langgraph.values import CheckpointerConfig

DATABASE_ERROR_SENTINEL = "database-runtime-detail-sentinel"
CLEANUP_ERROR_SENTINEL = "cleanup-detail-sentinel"


@pytest.fixture
def anyio_backend() -> str:
    """固定项目安装的 asyncio 后端."""
    return "asyncio"


@final
class PrimaryOpenError(OSError):
    """表示应被保留的原始打开错误."""


@final
class DatabaseFailurePool:
    """模拟包含敏感连接细节的 psycopg 打开失败."""

    def __init__(self) -> None:
        """初始化失败池状态."""
        self.closed: bool = False
        self.close_calls: int = 0

    async def open(self, *, wait: bool, timeout: float) -> None:  # noqa: ASYNC109
        """抛出携带唯一哨兵的数据库错误."""
        _ = (wait, timeout)
        raise OperationalError(DATABASE_ERROR_SENTINEL)

    async def close(self, timeout: float) -> None:  # noqa: ASYNC109
        """完成失败路径清理."""
        _ = timeout
        self.close_calls += 1
        self.closed = True


@final
class SlowClosePool:
    """模拟外部取消发生时仍可完成的池关闭."""

    def __init__(self) -> None:
        """初始化关闭同步信号."""
        self.closed: bool = False
        self.close_calls: int = 0
        self.started: anyio.Event = anyio.Event()

    async def close(self, timeout: float) -> None:  # noqa: ASYNC109
        """发出开始信号后经过一个取消点再关闭."""
        _ = timeout
        self.close_calls += 1
        self.started.set()
        await anyio.sleep(0.01)
        self.closed = True


@final
class CleanupProbeError(RuntimeError):
    """表示任意非 psycopg 清理失败."""


@final
class CleanupFailurePool:
    """模拟原始打开失败后清理也报告数据库错误."""

    def __init__(self) -> None:
        """初始化清理失败状态."""
        self.closed: bool = False

    async def open(self, *, wait: bool, timeout: float) -> None:  # noqa: ASYNC109
        """触发需要保留的原始错误."""
        _ = (wait, timeout)
        raise PrimaryOpenError

    async def close(self, timeout: float) -> None:  # noqa: ASYNC109
        """先关闭入口再报告清理错误."""
        _ = timeout
        self.closed = True
        raise CleanupProbeError(CLEANUP_ERROR_SENTINEL)


def _store() -> CheckpointStore:
    """构造不会连接数据库的适配器."""
    return CheckpointStore(
        CheckpointerConfig(dsn=SecretStr("postgresql://agent:secret@localhost/agent"))
    )


@pytest.mark.anyio
async def test_runtime_database_error_is_typed_and_value_free(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: psycopg 打开错误包含唯一敏感哨兵.
    store = _store()
    pool = DatabaseFailurePool()
    monkeypatch.setattr(store, "_pool", pool)

    # When: 适配器打开池.
    with pytest.raises(CheckpointStoreUnavailableError) as caught:
        await store.open()

    # Then: 错误脱敏且失败池已关闭.
    assert str(caught.value) == "LangGraph 检查点存储不可用"
    assert DATABASE_ERROR_SENTINEL not in str(caught.value)
    assert DATABASE_ERROR_SENTINEL not in repr(caught.value)
    assert caught.value.__cause__ is None
    assert pool.closed
    assert pool.close_calls == 1


@pytest.mark.anyio
async def test_external_cancellation_cannot_interrupt_pool_close(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: 一个关闭期间包含取消点的池.
    store = _store()
    pool = SlowClosePool()
    monkeypatch.setattr(store, "_pool", pool)

    # When: close 开始后取消其任务组.
    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(store.close)
        await pool.started.wait()
        tasks.cancel_scope.cancel()

    # Then: 屏蔽清理仍完成且没有开放池.
    assert pool.closed
    assert pool.close_calls == 1
    assert store.is_closed


@pytest.mark.anyio
async def test_cleanup_error_never_masks_original_open_error(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: 打开和随后清理分别失败.
    store = _store()
    pool = CleanupFailurePool()
    monkeypatch.setattr(store, "_pool", pool)

    # When: 进入适配器.
    with pytest.raises(PrimaryOpenError) as caught:
        await store.open()

    # Then: 原始错误保留, 清理细节不泄漏且池入口已关闭.
    assert pool.closed
    assert caught.value.__notes__ == ["检查点连接池清理未完成"]
    assert CLEANUP_ERROR_SENTINEL not in repr(caught.value)
