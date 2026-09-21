"""生产 Deep Runtime 复合适配器与组合前置校验。"""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from datetime import timedelta
from types import TracebackType
from typing import ClassVar

import pytest
from langgraph.checkpoint.memory import InMemorySaver
from pydantic import SecretStr
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

import scyg_agent.runtimes.deep.composition as composition_module
from scyg_agent.adapters.blog_grpc import (
    ArticleId,
    ArticleResult,
    ArticleSucceeded,
    BlogFailure,
    FailureKind,
    SearchSucceeded,
)
from scyg_agent.adapters.blog_grpc.contracts import BlogCommand
from scyg_agent.adapters.database.config import AsyncDatabaseConfig
from scyg_agent.adapters.database.deep_persistence import PostgreSQLApprovalPersistence
from scyg_agent.adapters.langgraph.values import CheckpointerConfig
from scyg_agent.domain.ports.tool_store import ToolOutcomeStatus
from scyg_agent.domain.runs import RuntimeKind, RuntimeSelection, TaskType
from scyg_agent.runtimes.deep import (
    BlogOutcomeFactory,
    DeepCompositionConfig,
    DeepRuntimeComposition,
    DeepRuntimeError,
)
from scyg_agent.runtimes.deep.engine import ApprovalPersistence
from scyg_agent.runtimes.profiles import (
    COMPOSE_DEEP_V1_PROFILE,
    RESEARCH_DEEP_V1_PROFILE,
    REVISE_DEEP_V1_PROFILE,
    RuntimeProfile,
    deep_profile_for_task,
)
from tests.runtimes.deep.test_runtime import proposal


@pytest.fixture
def anyio_backend() -> str:
    """数据库和 LangGraph 组合运行于 asyncio 后端。"""
    return "asyncio"


def test_production_persistence_satisfies_complete_deep_protocol() -> None:
    # Given: 一个由生产工厂使用的共享会话工厂。
    sessions = async_sessionmaker[AsyncSession]()

    # When: 构造 shipped 复合适配器。
    persistence = PostgreSQLApprovalPersistence(sessions, "scyg_agent_events")

    # Then: verifier 无需自建桥接对象即可满足完整端口。
    assert isinstance(persistence, ApprovalPersistence)


@pytest.mark.anyio
async def test_composition_rejects_selection_drift_before_external_io() -> None:
    # Given: 数据库配置不可用, 持久选择先发生漂移。
    config = DeepCompositionConfig(
        database=AsyncDatabaseConfig(SecretStr("not-opened")),
        checkpoints=CheckpointerConfig(
            dsn=SecretStr("postgresql://unused:unused@127.0.0.1:1/unused")
        ),
        task_type=TaskType.RESEARCH,
        selection=RuntimeSelection(RuntimeKind.SIMPLE, "v1"),
        blog_target="127.0.0.1:1",
        blog_deadline_seconds=1.0,
        notification_channel="scyg_agent_events",
        lease_duration=timedelta(minutes=5),
    )

    # When/Then: 工厂在 engine、T13 pool 和 T16 channel 之前失败。
    with pytest.raises(DeepRuntimeError):
        _ = await DeepRuntimeComposition.open(config)


def test_blog_outcome_factory_maps_all_t16_variants_without_response_body() -> None:
    # Given: 同一持久 proposal 和 T16 三种封闭结果。
    factory = BlogOutcomeFactory()
    envelope = proposal()
    article = ArticleResult(ArticleId("article-1"), "title", "body", "summary", 1, 7)

    # When: 映射文章、搜索和失败结果。
    article_outcome = factory.from_blog_result(envelope, ArticleSucceeded(article))
    search_outcome = factory.from_blog_result(envelope, SearchSucceeded((), None))
    failed_outcome = factory.from_blog_result(
        envelope, BlogFailure(FailureKind.UNAVAILABLE, retryable=True)
    )

    # Then: 成功引用稳定, 失败只保存关闭分类。
    assert article_outcome.status is ToolOutcomeStatus.SUCCEEDED
    assert article_outcome.result_reference is not None
    assert search_outcome.status is ToolOutcomeStatus.SUCCEEDED
    assert search_outcome.metadata.value == "0"
    assert failed_outcome.status is ToolOutcomeStatus.FAILED
    assert failed_outcome.error_code == FailureKind.UNAVAILABLE.value
    assert failed_outcome.metadata.value == "true"


class FakeEngine:
    """记录生产组合是否释放 Agent engine。"""

    disposed: ClassVar[int] = 0

    async def dispose(self, *, close: bool = True) -> None:
        """记录一次 SQLAlchemy 兼容释放。"""
        _ = close
        type(self).disposed += 1


class FakeCheckpointStore:
    """提供 T13 相同生命周期和 saver 表面。"""

    closed: ClassVar[int] = 0

    def __init__(self, _config: CheckpointerConfig) -> None:
        """接受生产检查点配置。"""

    async def __aenter__(self) -> "FakeCheckpointStore":
        """模拟已通过 readiness 的打开存储。"""
        return self

    async def __aexit__(
        self,
        _exception_type: type[BaseException] | None,
        _exception: BaseException | None,
        _traceback: TracebackType | None,
    ) -> None:
        """记录一次检查点池关闭。"""
        type(self).closed += 1

    @asynccontextmanager
    async def saver(self) -> AsyncIterator[InMemorySaver]:
        """借出与官方 saver 协议兼容的聚焦测试实现。"""
        yield InMemorySaver()


class FakeBlogClient:
    """提供 T16 相同资源生命周期表面。"""

    closed: ClassVar[int] = 0

    def __init__(self, _target: str, _profile: RuntimeProfile, _deadline_seconds: float) -> None:
        """接受生产 Blog 客户端构造参数。"""

    async def __aenter__(self) -> "FakeBlogClient":
        """返回已打开客户端。"""
        return self

    async def __aexit__(
        self,
        _exception_type: type[BaseException] | None,
        _exception: BaseException | None,
        _traceback: TracebackType | None,
    ) -> None:
        """记录一次通道关闭。"""
        type(self).closed += 1

    async def invoke(self, _tool_name: str, _version: str, _command: BlogCommand) -> BlogFailure:
        """生产构造测试不执行工具。"""
        return BlogFailure(FailureKind.INTERNAL, retryable=False)


def fake_create_engine(_config: AsyncDatabaseConfig) -> FakeEngine:
    """避免聚焦组合测试连接数据库。"""
    return FakeEngine()


@pytest.mark.anyio
async def test_production_composition_builds_runtime_and_closes_resources_once(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: shipped 工厂依赖的三个资源使用可观察生命周期替身。
    FakeEngine.disposed = 0
    FakeCheckpointStore.closed = 0
    FakeBlogClient.closed = 0
    monkeypatch.setattr(AsyncDatabaseConfig, "create_engine", fake_create_engine)
    monkeypatch.setattr(composition_module, "CheckpointStore", FakeCheckpointStore)
    monkeypatch.setattr(composition_module, "BlogGrpcClient", FakeBlogClient)
    config = DeepCompositionConfig(
        database=AsyncDatabaseConfig(SecretStr("not-opened")),
        checkpoints=CheckpointerConfig(
            dsn=SecretStr("postgresql://unused:unused@127.0.0.1:1/unused")
        ),
        task_type=TaskType.RESEARCH,
        selection=RuntimeSelection(RuntimeKind.DEEP, "v1"),
        blog_target="127.0.0.1:1",
        blog_deadline_seconds=1.0,
        notification_channel="scyg_agent_events",
        lease_duration=timedelta(minutes=5),
    )

    # When: 打开生产组合并重复关闭。
    composition = await DeepRuntimeComposition.open(config)
    await composition.close()
    await composition.close()

    # Then: verifier 得到可用 runtime, 每个资源只关闭一次。
    assert composition.runtime.profile.selection == config.selection
    assert FakeBlogClient.closed == 1
    assert FakeCheckpointStore.closed == 1
    assert FakeEngine.disposed == 1


@pytest.mark.parametrize(
    ("task_type", "expected"),
    [
        (TaskType.COMPOSE, COMPOSE_DEEP_V1_PROFILE),
        (TaskType.RESEARCH, RESEARCH_DEEP_V1_PROFILE),
        (TaskType.REVISE, REVISE_DEEP_V1_PROFILE),
        (TaskType.SUMMARY, None),
        (TaskType.QUESTION, None),
        (TaskType.POLISH, None),
    ],
)
def test_t14_deep_profile_lookup_is_the_only_composition_truth(
    task_type: TaskType,
    expected: RuntimeProfile | None,
) -> None:
    """生产组合对六个任务使用 T14 唯一画像查询。"""
    assert deep_profile_for_task(task_type) is expected
