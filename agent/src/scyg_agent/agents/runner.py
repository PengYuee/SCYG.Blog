"""AgentRunner boundary between recipes and the lease Worker."""

from dataclasses import dataclass
from typing import Protocol

from scyg_agent.agents.contracts import (
    OUTPUT_SCHEMA_VERSION,
    AgentFailure,
    ApprovalRequest,
    Capability,
    CapabilityOutput,
    output_digest,
    output_payload,
    validate_capability_output,
)
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.terminal_commit import TerminalResult
from scyg_agent.domain.runs import Run
from scyg_agent.domain.runs.repository_values import RunLease


@dataclass(frozen=True, slots=True)
class AgentSucceeded:
    """Carry one capability-checked terminal result to the Worker."""

    capability: Capability
    output: CapabilityOutput

    def __post_init__(self) -> None:
        """Reject a result whose concrete schema does not match the capability."""
        _ = validate_capability_output(self.capability, self.output)

    def terminal_result(self) -> TerminalResult:
        """Return the canonical result payload for atomic persistence."""
        return TerminalResult(
            OUTPUT_SCHEMA_VERSION,
            self.capability.value,
            output_payload(self.output),
            output_digest(self.output),
        )


@dataclass(frozen=True, slots=True)
class AgentWaitingForApproval:
    """Carry one sanitized approval request without a terminal result."""

    request: ApprovalRequest


@dataclass(frozen=True, slots=True)
class AgentFailed:
    """Carry one stable, secret-free failure to the Worker."""

    failure: AgentFailure


type AgentRunOutcome = AgentSucceeded | AgentWaitingForApproval | AgentFailed


class AgentRunner(Protocol):
    """Execute or resume a recipe under the Worker lease boundary."""

    async def execute(self, run: Run, lease: RunLease) -> AgentRunOutcome:
        """Run a newly claimed Run and return a closed outcome."""
        ...

    async def resume(
        self,
        run: Run,
        command: CommandSubmission,
        lease: RunLease,
    ) -> AgentRunOutcome:
        """Resume a claimed Run after a persisted user interaction."""
        ...


def validated_agent_success(capability: Capability, output: CapabilityOutput) -> AgentSucceeded:
    """Construct the only successful outcome accepted by the Worker boundary."""
    return AgentSucceeded(capability, validate_capability_output(capability, output))
