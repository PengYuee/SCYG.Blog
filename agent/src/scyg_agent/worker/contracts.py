"""Worker 组合根需要的严格端口."""

from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime
from typing import Protocol

from scyg_agent.adapters.redis import RedisStreamStore
from scyg_agent.agents.runner import AgentRunner
from scyg_agent.domain.ports.terminal_commit import TerminalCommitRequest, TerminalCommitResult
from scyg_agent.domain.runs import RunId
from scyg_agent.domain.runs.repository import (
    ClaimRequest,
    GetResult,
    RenewRequest,
    RenewResult,
    RunLease,
)


class WorkerRunRepository(Protocol):
    """暴露调度和续租所需的最小 Run 仓储表面."""

    async def claim(self, request: ClaimRequest) -> tuple[RunLease, ...]:
        """按运行时种类认领有界租约."""
        ...

    async def get(self, run_id: RunId) -> GetResult:
        """读取认领后的持久化 Run."""
        ...

    async def renew(self, request: RenewRequest) -> RenewResult:
        """续租并观察取消或丢租."""
        ...


class TerminalCommitter(Protocol):
    """原子提交终态 Run、事件及审计."""

    async def commit(self, request: TerminalCommitRequest) -> TerminalCommitResult:
        """提交一个围栏终态事务."""
        ...


@dataclass(frozen=True, slots=True)
class WorkerDependencies:
    """绑定 Worker 借用但不关闭的外部持久化依赖图."""

    repository: WorkerRunRepository
    terminal_committer: TerminalCommitter
    clock: Callable[[], datetime]
    agent_runner: AgentRunner
    stream_store: RedisStreamStore | None = None
