"""AgentControl 真实 RPC 场景门面与服务夹具."""

from collections.abc import AsyncIterator
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Protocol

import anyio
import pytest
from grpc import aio

from scyg_agent.adapters.auth import JwtVerifier
from scyg_agent.application import (
    CreateRunInput,
    FacadeCancelled,
    FacadeConflict,
    FacadeInternal,
    FacadeNotFound,
    FacadePrecondition,
    FacadeSuccess,
    FacadeValidation,
    OwnerContext,
    SnapshotSuccess,
)
from scyg_agent.domain.ports.event_store import EventCursor
from scyg_agent.domain.runs import Run, RunId, RunStatus
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc
from scyg_agent.transport.grpc import AgentControlServicer

Metadata = tuple[tuple[str, str], ...]
type CreateOutcome = (
    FacadeSuccess
    | FacadeConflict
    | FacadePrecondition
    | FacadeValidation
    | FacadeCancelled
    | FacadeInternal
)
type GetOutcome = (
    SnapshotSuccess
    | FacadeNotFound
    | FacadePrecondition
    | FacadeValidation
    | FacadeCancelled
    | FacadeInternal
)


class ControlStub(Protocol):
    """声明生成 stub 的两个一元调用."""

    def CreateRun(  # noqa: N802
        self,
        request: service_pb2.CreateRunRequest,
        *,
        metadata: Metadata,
        timeout: float | None = None,
    ) -> aio.UnaryUnaryCall[service_pb2.CreateRunRequest, service_pb2.CreateRunResponse]: ...

    def GetRun(  # noqa: N802
        self,
        request: service_pb2.GetRunRequest,
        *,
        metadata: Metadata,
        timeout: float | None = None,
    ) -> aio.UnaryUnaryCall[service_pb2.GetRunRequest, service_pb2.GetRunResponse]: ...


@dataclass(slots=True)  # noqa: RUF100  # noqa: MUTABLE_OK
class ScenarioFacade:
    """以可观测状态控制真实 RPC 下游结果与生命周期."""

    create_outcome: CreateOutcome | None = None
    get_outcome: GetOutcome | None = None
    created: Run | None = None
    calls: int = 0
    block: bool = False
    started: anyio.Event = field(default_factory=anyio.Event)
    drained: anyio.Event = field(default_factory=anyio.Event)

    async def create_run(self, request: CreateRunInput) -> CreateOutcome:
        """返回配置结果或按请求构造成功事实."""
        await self._before()
        self.calls += 1
        if self.create_outcome is not None:
            return self.create_outcome
        if self.created is None:
            self.created = Run(
                request.run_id,
                request.owner.user_id,
                request.task_type,
                request.runtime,
                1,
                RunStatus.PENDING,
                request.requested_at,
                request.requested_at,
                0,
                None,
                None,
            )
        return FacadeSuccess(self.created, replayed=self.calls > 1)

    async def get_snapshot(self, owner: OwnerContext, run_id: RunId) -> GetOutcome:
        """返回配置结果或当前创建快照."""
        del owner
        await self._before()
        if self.get_outcome is not None:
            return self.get_outcome
        if self.created is None or self.created.id != run_id:
            return FacadeNotFound(run_id)
        return SnapshotSuccess(self.created, EventCursor(0))

    async def _before(self) -> None:
        if self.block:
            self.started.set()
            try:
                await anyio.sleep_forever()
            finally:
                self.drained.set()


@dataclass(frozen=True, slots=True)
class RpcHarness:
    """汇集真实生成 stub 与可观测门面."""

    stub: ControlStub
    facade: ScenarioFacade


if TYPE_CHECKING:

    def add_control_servicer(_servicer: AgentControlServicer, _server: aio.Server) -> None: ...
else:
    add_control_servicer = service_grpc.add_AgentControlServiceServicer_to_server


@pytest.fixture
async def rpc_harness(verifier: JwtVerifier) -> AsyncIterator[RpcHarness]:
    """启动并关闭真实本地 grpc.aio 服务和通道."""
    facade = ScenarioFacade()
    server = aio.server()
    add_control_servicer(AgentControlServicer(facade, verifier), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    channel = aio.insecure_channel(f"127.0.0.1:{port}")
    try:
        yield RpcHarness(service_grpc.AgentControlServiceStub(channel), facade)
    finally:
        await channel.close()
        await server.stop(None)
