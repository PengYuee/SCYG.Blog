"""T13 独立审查缺陷的无数据库回归测试."""

from __future__ import annotations

from typing import final, override

import anyio
import pytest
from pydantic import SecretStr

from scyg_agent.adapters.langgraph.checkpointer import (
    CheckpointPoolCloseError,
    CheckpointStore,
)
from scyg_agent.adapters.langgraph.metadata import (
    IncompatibleCheckpoint,
    MetadataCandidate,
    parse_checkpoint_metadata,
)
from scyg_agent.adapters.langgraph.values import (
    CheckpointerConfig,
    InvalidCheckpointerConfigError,
)

DSN_USER_SENTINEL = "t13_unique_user_sentinel"


def _secret_sentinel() -> str:
    """构造仅用于泄漏探针的唯一值."""
    return "t13_unique_" + "password_sentinel"


DSN_SECRET_SENTINEL = _secret_sentinel()
MALFORMED_DSN = f"postgresql://{DSN_USER_SENTINEL}:{DSN_SECRET_SENTINEL}@ bad host/db"


@pytest.fixture
def anyio_backend() -> str:
    """固定项目安装的 asyncio 后端."""
    return "asyncio"


@final
class IncompleteClosePool:
    """模拟 psycopg 池已标记关闭但工作线程未及时退出."""

    def __init__(self) -> None:
        """初始化可观察生命周期状态."""
        self.closed: bool = False
        self.close_calls: int = 0

    async def close(self, timeout: float) -> None:  # noqa: ASYNC109
        """先关闭借用入口, 再模拟后台清理超时."""
        _ = timeout
        self.close_calls += 1
        self.closed = False


@final
class OpenProbeError(OSError):
    """表示测试池的确定打开失败."""

    @override
    def __str__(self) -> str:
        """返回稳定探针消息."""
        return "open-probe"


@final
class OpenFailurePool:
    """模拟打开失败并记录屏蔽清理."""

    def __init__(self) -> None:
        """初始化失败池状态."""
        self.closed: bool = False
        self.close_calls: int = 0

    async def open(self, *, wait: bool, timeout: float) -> None:  # noqa: ASYNC109
        """稳定触发打开失败."""
        _ = (wait, timeout)
        raise OpenProbeError

    async def close(self, timeout: float) -> None:  # noqa: ASYNC109
        """记录失败路径执行了关闭."""
        _ = timeout
        self.close_calls += 1
        self.closed = True


@final
class CancelledOpenPool:
    """模拟打开期间被取消的池."""

    def __init__(self) -> None:
        """初始化取消路径状态."""
        self.closed: bool = False
        self.close_calls: int = 0
        self.started: anyio.Event = anyio.Event()

    async def open(self, *, wait: bool, timeout: float) -> None:  # noqa: ASYNC109
        """发出已开始信号并等待取消."""
        _ = (wait, timeout)
        self.started.set()
        await anyio.sleep_forever()

    async def close(self, timeout: float) -> None:  # noqa: ASYNC109
        """记录取消后的屏蔽关闭."""
        _ = timeout
        self.close_calls += 1
        self.closed = True


@final
class SuccessfulPool:
    """模拟可重复进入且可观察关闭的专用池."""

    def __init__(self) -> None:
        """初始化成功池计数."""
        self.closed: bool = False
        self.open_calls: int = 0
        self.close_calls: int = 0

    async def open(self, *, wait: bool, timeout: float) -> None:  # noqa: ASYNC109
        """记录一次成功打开."""
        _ = (wait, timeout)
        self.open_calls += 1

    async def close(self, timeout: float) -> None:  # noqa: ASYNC109
        """记录一次成功关闭."""
        _ = timeout
        self.close_calls += 1
        self.closed = True


@final
class BodyProbeError(RuntimeError):
    """表示上下文主体的确定失败."""

    @override
    def __str__(self) -> str:
        """返回稳定探针消息."""
        return "body-probe"


def _store() -> CheckpointStore:
    """构造不连接数据库的适配器."""
    return CheckpointStore(
        CheckpointerConfig(
            dsn=SecretStr("postgresql://agent:secret@localhost/agent"),
            close_timeout_seconds=0.01,
        )
    )


def test_malformed_dsn_never_reaches_pydantic_diagnostic_surfaces() -> None:
    # Given/When: 唯一凭据哨兵进入不合法 DSN 边界.
    with pytest.raises(InvalidCheckpointerConfigError) as caught:
        _ = CheckpointerConfig(dsn=SecretStr(MALFORMED_DSN))

    # Then: 稳定领域错误及完整异常链均不包含输入值.
    error = caught.value
    assert type(error) is InvalidCheckpointerConfigError
    assert str(error) == "LangGraph 检查点配置无效"
    assert error.__cause__ is None
    assert error.__context__ is None
    surfaces = (str(error), repr(error))
    assert all(DSN_USER_SENTINEL not in surface for surface in surfaces)
    assert all(DSN_SECRET_SENTINEL not in surface for surface in surfaces)


@pytest.mark.parametrize(
    "metadata",
    [
        {"source": "input", "step": 1, "parents": {}},
        {
            "source": "input",
            "step": "1",
            "parents": {},
            "scyg_runtime_kind": "deep",
            "scyg_runtime_version": "v1",
            "scyg_dependency_fingerprint": "coercion-sentinel",
        },
        {
            "source": "input",
            "step": 1,
            "parents": {},
            "scyg_runtime_kind": 7,
            "scyg_runtime_version": "v1",
            "scyg_dependency_fingerprint": "secret-metadata-sentinel",
        },
        {
            "source": "input",
            "step": 1,
            "parents": {},
            "scyg_runtime_kind": "deep",
            "scyg_runtime_version": "v1",
            "scyg_dependency_fingerprint": "valid",
            "unexpected": "secret-extra-sentinel",
        },
    ],
)
def test_malformed_metadata_is_always_a_typed_value_free_error(
    metadata: MetadataCandidate,
) -> None:
    # Given/When/Then: 缺失, 错型或额外 metadata 均转换为稳定错误.
    with pytest.raises(IncompatibleCheckpoint) as caught:
        _ = parse_checkpoint_metadata(metadata)
    assert str(caught.value) == "LangGraph 检查点运行时不兼容"
    assert "sentinel" not in str(caught.value)
    assert "sentinel" not in repr(caught.value)


@pytest.mark.anyio
async def test_close_timeout_is_typed_and_pool_is_already_closed(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: close 在标记池关闭后永久阻塞.
    store = _store()
    pool = IncompleteClosePool()
    monkeypatch.setattr(store, "_pool", pool)

    # When/Then: 有界关闭不能静默成功, 且池已拒绝新借用.
    with pytest.raises(CheckpointPoolCloseError):
        await store.close()
    assert not pool.closed
    assert pool.close_calls == 1


@pytest.mark.anyio
async def test_enter_failure_closes_pool_and_preserves_original_error(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: 专用池打开失败.
    store = _store()
    pool = OpenFailurePool()
    monkeypatch.setattr(store, "_pool", pool)

    # When/Then: 原始错误传播前完成关闭.
    with pytest.raises(OpenProbeError, match="open-probe"):
        _ = await store.__aenter__()
    assert pool.closed
    assert pool.close_calls == 1


@pytest.mark.anyio
async def test_enter_cancellation_shields_pool_cleanup(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: 专用池正在打开.
    store = _store()
    pool = CancelledOpenPool()
    monkeypatch.setattr(store, "_pool", pool)

    async def enter() -> None:
        """进入适配器并允许外层取消."""
        _ = await store.__aenter__()

    # When: 打开任务在开始后被取消.
    async with anyio.create_task_group() as tasks:
        _ = tasks.start_soon(enter)
        await pool.started.wait()
        tasks.cancel_scope.cancel()

    # Then: 取消路径仍已关闭池.
    assert pool.closed
    assert pool.close_calls == 1


@pytest.mark.anyio
async def test_repeated_open_close_and_body_error_leave_no_pool_residue(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: 一个可观察的成功专用池.
    store = _store()
    pool = SuccessfulPool()
    monkeypatch.setattr(store, "_pool", pool)

    # When: 重复打开后上下文主体失败, 随后重复关闭.
    await store.open()
    await store.open()
    with pytest.raises(BodyProbeError, match="body-probe"):
        async with store:
            raise BodyProbeError
    await store.close()

    # Then: 打开和关闭均幂等, 原始主体错误未被替换.
    assert pool.open_calls == 1
    assert pool.close_calls == 1
    assert pool.closed
    assert store.is_closed
