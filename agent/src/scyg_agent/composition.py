"""Agent 生产应用组件工厂."""

from dataclasses import dataclass
from datetime import timedelta
from pathlib import Path
from typing import ClassVar, Protocol

from fastapi import FastAPI
from pydantic import BaseModel, ConfigDict, SecretStr
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.config import AsyncDatabaseConfig
from scyg_agent.adapters.database.run_repository import PostgreSQLRunRepository
from scyg_agent.adapters.database.terminal_commit import PostgreSQLTerminalCommitter
from scyg_agent.adapters.langgraph.checkpointer import CheckpointStore
from scyg_agent.adapters.langgraph.values import CheckpointerConfig
from scyg_agent.adapters.redis import RedisSettings, RedisStreamClient, RedisStreamStore
from scyg_agent.composition_redis import RedisResource
from scyg_agent.composition_resources import (
    CheckpointResource,
    DatabaseResource,
    MigrationHeadResource,
)
from scyg_agent.composition_runtime import NOTIFICATION_CHANNEL, RuntimeFacadeResource, utc_now
from scyg_agent.composition_services import DeferredComponent, WorkerResource
from scyg_agent.config import ApplicationSettings
from scyg_agent.domain.runs import ExecutionOwnerId
from scyg_agent.lifecycle import AgentApplication, LifecycleComponent, LifecycleComponents
from scyg_agent.servers import (
    GrpcServerComponent,
    GrpcServerConfig,
    HttpServerComponent,
    HttpServerConfig,
)
from scyg_agent.transport.grpc import AgentControlServicer
from scyg_agent.transport.http import HTTPDependencies, create_http_app
from scyg_agent.worker import Worker, WorkerConfig, WorkerDependencies


class ComponentDecorator(Protocol):
    """为测试注入类型化组件 failpoint."""

    def __call__(self, component: LifecycleComponent) -> LifecycleComponent:
        """返回生产组件或测试包装器."""
        ...


class HealthComponent(BaseModel):
    """公开脱敏组件就绪状态."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True)
    component: str
    ready: bool
    detail: str


class ReadinessResponse(BaseModel):
    """公开聚合就绪状态."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True)
    ready: bool
    components: tuple[HealthComponent, ...]


class LivenessResponse(BaseModel):
    """公开不依赖下游的存活状态."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True)
    alive: bool


def _identity(component: LifecycleComponent) -> LifecycleComponent:
    return component


@dataclass(frozen=True, slots=True)
class ProductionApplication:
    """持有已组合但尚未启动的生产生命周期."""

    lifecycle: AgentApplication


@dataclass(frozen=True, slots=True)
class ProductionApplicationFactory:
    """从严格设置构造全部真实生产组件."""

    settings: ApplicationSettings
    decorate: ComponentDecorator = _identity

    def build(self) -> ProductionApplication:
        """按权威顺序构造无 I/O 组件计划."""
        database = DatabaseResource(AsyncDatabaseConfig(self.settings.database_url).create_engine())
        sessions = async_sessionmaker(database.engine, expire_on_commit=False)
        checkpoint_dsn = SecretStr(
            self.settings.database_url.get_secret_value().replace(
                "postgresql+asyncpg://", "postgresql://", 1
            )
        )
        checkpoint = CheckpointResource(CheckpointStore(CheckpointerConfig(checkpoint_dsn)))
        redis = RedisResource(
            RedisStreamClient.create(
                RedisSettings(
                    self.settings.redis_url,
                    self.settings.redis_ttl_seconds,
                    self.settings.redis_stream_maxlen,
                    self.settings.redis_connect_timeout_seconds,
                    self.settings.redis_read_timeout_seconds,
                )
            )
        )
        runtime = RuntimeFacadeResource(self.settings, sessions, checkpoint.store)
        lifecycle_slot: list[AgentApplication] = []
        components: list[LifecycleComponent] = [
            self.decorate(database),
            self.decorate(
                MigrationHeadResource(database.engine, Path(__file__).parents[2] / "alembic.ini")
            ),
            self.decorate(checkpoint),
            self.decorate(redis),
            self.decorate(runtime),
        ]
        self._append_surfaces(components, sessions, runtime, redis, lifecycle_slot)
        lifecycle = AgentApplication(
            LifecycleComponents(tuple(components)), float(self.settings.shutdown_seconds)
        )
        lifecycle_slot.append(lifecycle)
        return ProductionApplication(lifecycle)

    def _append_surfaces(
        self,
        components: list[LifecycleComponent],
        sessions: async_sessionmaker[AsyncSession],
        runtime: RuntimeFacadeResource,
        redis: RedisResource,
        lifecycle_slot: list[AgentApplication],
    ) -> None:
        """按 gRPC、Worker、HTTP 顺序追加已启用表面."""
        if self.settings.feature_flags.grpc:
            components.append(self.decorate(self._grpc(runtime, redis.client)))
        if self.settings.feature_flags.worker:
            components.append(self.decorate(self._worker(sessions, runtime, redis.client)))
        if self.settings.feature_flags.http:
            components.append(self.decorate(self._http(runtime, lifecycle_slot, redis.client)))

    def _grpc(
        self, runtime: RuntimeFacadeResource, stream_store: RedisStreamStore
    ) -> LifecycleComponent:
        def build() -> LifecycleComponent:
            return GrpcServerComponent(
                AgentControlServicer(
                    runtime.require_facade(), runtime.require_verifier(), stream_store=stream_store
                ),
                GrpcServerConfig(
                    self.settings.grpc_host,
                    self.settings.grpc_port,
                    float(self.settings.heartbeat_seconds),
                    float(self.settings.shutdown_seconds),
                ),
            )

        return DeferredComponent("grpc", build)

    def _worker(
        self,
        sessions: async_sessionmaker[AsyncSession],
        runtime: RuntimeFacadeResource,
        stream_store: RedisStreamStore,
    ) -> LifecycleComponent:
        def build() -> Worker:
            return Worker(
                ExecutionOwnerId("worker_process01"),
                WorkerDependencies(
                    PostgreSQLRunRepository(sessions),
                    PostgreSQLTerminalCommitter(sessions, NOTIFICATION_CHANNEL),
                    utc_now,
                    runtime.require_agent_runner(),
                    stream_store,
                ),
                runtime.require_router(),
                WorkerConfig(
                    simple_capacity=self.settings.simple_concurrency,
                    deep_capacity=self.settings.deep_concurrency,
                    lease_duration=timedelta(seconds=self.settings.lease_seconds),
                    poll_interval=timedelta(milliseconds=self.settings.worker_poll_milliseconds),
                    error_backoff=timedelta(seconds=self.settings.worker_error_backoff_seconds),
                    drain_timeout=timedelta(seconds=self.settings.worker_drain_seconds),
                    stream_flush_chars=self.settings.stream_flush_chars,
                    stream_flush_interval=timedelta(
                        milliseconds=self.settings.stream_flush_interval_ms
                    ),
                ),
            )

        return WorkerResource(build, float(self.settings.heartbeat_seconds))

    def _http(
        self,
        runtime: RuntimeFacadeResource,
        lifecycle_slot: list[AgentApplication],
        stream_store: RedisStreamStore,
    ) -> LifecycleComponent:
        def build() -> LifecycleComponent:
            app = create_http_app(
                HTTPDependencies(
                    runtime.require_facade(), runtime.require_verifier(), stream_store=stream_store
                )
            )
            self.mount_health(app, lifecycle_slot)
            return HttpServerComponent(
                app,
                HttpServerConfig(
                    self.settings.http_host,
                    self.settings.http_port,
                    float(self.settings.heartbeat_seconds),
                    float(self.settings.shutdown_seconds),
                ),
            )

        return DeferredComponent("http", build)

    @staticmethod
    def mount_health(app: FastAPI, lifecycle_slot: list[AgentApplication]) -> None:
        """挂载不泄露配置的存活与就绪路由."""

        @app.get("/health/live")
        async def live() -> LivenessResponse:
            return LivenessResponse(alive=lifecycle_slot[0].liveness().alive)

        @app.get("/health/ready")
        async def ready() -> ReadinessResponse:
            report = await lifecycle_slot[0].readiness()
            return ReadinessResponse(
                ready=report.ready,
                components=tuple(
                    HealthComponent(component=item.component, ready=item.ready, detail=item.detail)
                    for item in report.components
                ),
            )

        _ = live, ready
