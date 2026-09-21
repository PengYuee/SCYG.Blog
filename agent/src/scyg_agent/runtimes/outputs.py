"""Closed application-native outputs consumed by Worker."""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from scyg_agent.runtimes.deep.models import (
        ApprovalReply,
        DeepResult,
        ProposalEnvelope,
    )
    from scyg_agent.runtimes.normalization_common import DeepFailureState
    from scyg_agent.runtimes.simple.results import ProviderResult


@dataclass(frozen=True, slots=True)
class SimpleRuntimeOutput:
    """Carry one sanitized SIMPLE streaming outcome."""

    outcome: ProviderResult


@dataclass(frozen=True, slots=True)
class DeepRuntimeOutput:
    """Carry one sanitized DEEP approval, tool, or graph outcome."""

    proposal: ProposalEnvelope
    reply: ApprovalReply | None
    result: DeepResult | DeepFailureState


type RuntimeNativeOutput = SimpleRuntimeOutput | DeepRuntimeOutput
