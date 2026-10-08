"""Controlled AgentRunner outcomes for real PostgreSQL Worker integration."""

from dataclasses import dataclass
from typing import final

import anyio

from scyg_agent.adapters.database.run_request_source import PostgreSQLRunInputSource
from scyg_agent.agents.contracts import AgentFailure, Capability, ChatResponse, FailureKind
from scyg_agent.agents.runner import AgentFailed, AgentRunOutcome, AgentSucceeded
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.runs import ExecutionOwnerId, Run, RunId
from scyg_agent.domain.runs.input import RunInput
from scyg_agent.domain.runs.repository import RunLease


@dataclass(frozen=True, slots=True)
class Visit:
    """Record the actual runner invocation's persisted execution fence."""

    run_id: RunId
    owner: ExecutionOwnerId
    attempt: int
    capability: Capability


@final
class RunnerProbe:
    """Hold real Worker execution while observing unified per-owner capacity."""

    def __init__(self) -> None:
        self.gate = anyio.Event()
        self.both_started = anyio.Event()
        self.lock = anyio.Lock()
        self.visits: list[Visit] = []
        self.active: dict[ExecutionOwnerId, int] = {}
        self.maxima: dict[ExecutionOwnerId, int] = {}

    async def enter(self, run: Run, lease: RunLease, capability: Capability) -> None:
        assert run.execution_owner == lease.owner
        assert run.id == lease.run_id
        assert run.attempt == lease.attempt
        owner = lease.owner
        async with self.lock:
            self.visits.append(Visit(run.id, owner, lease.attempt, capability))
            self.active[owner] = self.active.get(owner, 0) + 1
            self.maxima[owner] = max(self.maxima.get(owner, 0), self.active[owner])
            if len(self.maxima) == 2:
                self.both_started.set()
        try:
            await self.gate.wait()
        finally:
            with anyio.CancelScope(shield=True):
                async with self.lock:
                    self.active[owner] -= 1


@final
class BarrierAgentRunner:
    """Return validated success or closed failure without external provider/tool I/O."""

    def __init__(self, probe: RunnerProbe, inputs: PostgreSQLRunInputSource) -> None:
        self._probe = probe
        self._inputs = inputs

    async def execute(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        persisted = await self._inputs.get_input(run.id)
        assert isinstance(persisted, RunInput)
        assert persisted.capability is not None
        capability = Capability(persisted.capability)
        await self._probe.enter(run, lease, capability)
        if capability is Capability.SEARCH:
            return AgentFailed(
                AgentFailure(kind=FailureKind.VALIDATION, message="Invalid search input"),
            )
        return AgentSucceeded(Capability.CHAT, ChatResponse(response="Persisted worker result"))

    async def resume(
        self,
        run: Run,
        command: CommandSubmission,
        lease: RunLease,
    ) -> AgentRunOutcome:
        assert command.run_id == run.id
        return await self.execute(run, lease)
