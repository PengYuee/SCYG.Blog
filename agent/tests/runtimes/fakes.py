"""T14 运行时目录的应用协议假适配器。"""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Final, Protocol

from scyg_agent.domain.runs import (
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.runtimes.base import AdapterIdentity
from scyg_agent.runtimes.outputs import RuntimeNativeOutput, SimpleRuntimeOutput
from scyg_agent.runtimes.simple.results import CompletionFinished

NOW = datetime(2026, 7, 12, 8, 0, tzinfo=UTC)
MISSING_IDENTITY_FALLBACK: Final = AdapterIdentity("missing-fallback", RuntimeKind.SIMPLE)


@dataclass(frozen=True, slots=True)
class FakeRuntimeAdapter:
    """返回确定性应用事件的完整 RuntimeAdapter 实现。"""

    identity: AdapterIdentity

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """产生首次执行事件。"""
        _ = run
        yield SimpleRuntimeOutput(CompletionFinished("stop"))

    async def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """产生恢复执行事件。"""
        _ = run
        yield SimpleRuntimeOutput(CompletionFinished("stop"))


class MissingIdentityAdapter:
    """模拟 identity 属性读取失败的协议冒充对象。"""

    @property
    def identity(self) -> AdapterIdentity:
        """模拟缺失身份。"""
        raise AttributeError

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """保持执行签名完整。"""
        async for event in FakeRuntimeAdapter(MISSING_IDENTITY_FALLBACK).execute(run):
            yield event

    async def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """保持恢复签名完整。"""
        async for event in FakeRuntimeAdapter(MISSING_IDENTITY_FALLBACK).resume(run):
            yield event


class RuntimeCall(Protocol):
    """描述测试冒充对象声称提供的运行时调用。"""

    def __call__(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """返回应用事件流。"""
        ...


class MissingExecuteAdapter:
    """模拟 execute 属性读取失败的协议冒充对象。"""

    identity: AdapterIdentity = MISSING_IDENTITY_FALLBACK

    @property
    def execute(self) -> RuntimeCall:
        """模拟缺失执行方法。"""
        raise AttributeError

    async def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """保持恢复签名完整。"""
        async for event in FakeRuntimeAdapter(MISSING_IDENTITY_FALLBACK).resume(run):
            yield event


class MutableRuntimeAdapter:
    """允许测试替换 identity 引用并观察分发计数。"""

    __slots__: tuple[str, ...] = (
        "_identity",
        "execute_calls",
        "identity_available",
        "resume_calls",
    )

    _identity: AdapterIdentity
    execute_calls: int
    identity_available: bool
    resume_calls: int

    def __init__(self, identity: AdapterIdentity) -> None:
        """以可替换引用和零调用计数初始化。"""
        self._identity = identity
        self.execute_calls = 0
        self.identity_available = True
        self.resume_calls = 0

    @property
    def identity(self) -> AdapterIdentity:
        """返回当前身份或模拟属性读取失败。"""
        if not self.identity_available:
            raise AttributeError
        return self._identity

    @identity.setter
    def identity(self, value: AdapterIdentity) -> None:
        """替换身份引用以模拟注册后漂移。"""
        self._identity = value

    async def execute(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """记录真实执行并产生事件。"""
        self.execute_calls += 1
        async for event in FakeRuntimeAdapter(self._identity).execute(run):
            yield event

    async def resume(self, run: Run) -> AsyncIterator[RuntimeNativeOutput]:
        """记录真实恢复并产生事件。"""
        self.resume_calls += 1
        async for event in FakeRuntimeAdapter(self._identity).resume(run):
            yield event


def make_run(task_type: TaskType, runtime: RuntimeSelection) -> Run:
    """构造用于直接分发验证的冻结 Run。"""
    return Run(
        id=RunId("run_12345678"),
        owner_user_id=UserId("user-1"),
        task_type=task_type,
        runtime=runtime,
        revision=1,
        status=RunStatus.PENDING,
        created_at=NOW,
        updated_at=NOW,
        attempt=0,
        execution_owner=None,
        pending_interaction_id=None,
    )
