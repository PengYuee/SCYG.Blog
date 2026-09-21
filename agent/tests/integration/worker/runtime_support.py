"""双 Worker PostgreSQL 测试的确定性运行时探针."""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import final

import anyio

from scyg_agent.domain.runs import ExecutionOwnerId, Run, RunId, RuntimeKind
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.deep.models import DeepFailureKind, DeepRuntimeError
from scyg_agent.runtimes.outputs import RuntimeNativeOutput, SimpleRuntimeOutput
from scyg_agent.runtimes.simple.results import CompletionFinished


@dataclass(frozen=True, slots=True)
class Visit:
    """记录一次实际运行时调用的围栏身份."""

    run_id: RunId
    owner: ExecutionOwnerId
    attempt: int
    kind: RuntimeKind


@final
class RuntimeProbe:
    """以确定性闸门记录两个 Worker 的实际运行时并发."""

    def __init__(self) -> None:
        self.gate = anyio.Event()
        self.started = anyio.Event()
        self.lock = anyio.Lock()
        self.visits: list[Visit] = []
        self.active: dict[tuple[ExecutionOwnerId, RuntimeKind], int] = {}
        self.maxima: dict[tuple[ExecutionOwnerId, RuntimeKind], int] = {}

    async def enter(self, run: Run) -> None:
        """记录活动计数并等待测试释放."""
        owner = run.execution_owner
        assert owner is not None
        key = (owner, run.runtime.kind)
        async with self.lock:
            self.visits.append(Visit(run.id, owner, run.attempt, run.runtime.kind))
            self.active[key] = self.active.get(key, 0) + 1
            self.maxima[key] = max(self.maxima.get(key, 0), self.active[key])
            self.started.set()
        try:
            await self.gate.wait()
        finally:
            with anyio.CancelScope(shield=True):
                async with self.lock:
                    self.active[key] -= 1


@final
class SimpleBarrierAdapter:
    """产生 SIMPLE 成功结果并仅用闸门控制执行时长."""

    identity = AdapterIdentity("worker-pg-simple", RuntimeKind.SIMPLE)

    def __init__(self, probe: RuntimeProbe) -> None:
        self._probe = probe

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        await self._probe.enter(run)
        yield SimpleRuntimeOutput(CompletionFinished("stop"))

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        return self.execute(run)


@final
class DeepFailureAdapter:
    """记录 DEEP 调度后产生封闭失败, 避免测试调用外部工具."""

    identity = AdapterIdentity("worker-pg-deep", RuntimeKind.DEEP)

    def __init__(self, probe: RuntimeProbe) -> None:
        self._probe = probe

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        await self._probe.enter(run)
        if run.runtime.kind is RuntimeKind.DEEP:
            raise DeepRuntimeError(DeepFailureKind.INVALID_INPUT)
        yield SimpleRuntimeOutput(CompletionFinished("stop"))

    def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        return self.execute(run)
