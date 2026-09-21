"""Runtime probes for variants that satisfy static base types but are not domain variants."""

from dataclasses import dataclass

from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    ReleaseExecution,
    RunStatus,
    RunSucceeded,
    UnknownVariant,
    event_kind,
)
from scyg_agent.domain.runs.commands import command_kind
from scyg_agent.domain.runs.transition_handlers import apply_transition

from .helpers import LATER, make_run


@dataclass(frozen=True, slots=True)
class ReleaseImpostor(ReleaseExecution):
    """Satisfy the static command base while remaining outside the closed union."""


@dataclass(frozen=True, slots=True)
class SucceededEventImpostor(RunSucceeded):
    """Satisfy the static event base while remaining outside the closed union."""


def test_command_kind_rejects_runtime_subclass_impostor() -> None:
    # Given: a runtime subclass inherits the current mutable class discriminator.
    command = ReleaseImpostor(
        CommandId("cmd_abcdefgh"),
        EventId("evt_abcdefgh"),
        5,
        LATER,
    )

    # When: command discrimination receives a non-union concrete class.
    result = command_kind(command)

    # Then: it must be rejected rather than inherit RELEASE_EXECUTION meaning.
    assert result == UnknownVariant("command", "ReleaseImpostor")
    assert not hasattr(ReleaseExecution, "kind")


def test_apply_transition_rejects_release_subclass_impostor() -> None:
    # Given: a running aggregate and a release-shaped subclass impostor.
    run = make_run(RunStatus.RUNNING)
    command = ReleaseImpostor(
        CommandId("cmd_abcdefgh"),
        EventId("evt_abcdefgh"),
        run.revision,
        LATER,
    )

    # When: the post-matrix handler receives the runtime impostor directly.
    result = apply_transition(run, command)

    # Then: no valid release transition or event is emitted.
    assert result == UnknownVariant("command", "ReleaseImpostor")
    assert run.status is RunStatus.RUNNING
    assert run.revision == 5


def test_event_kind_rejects_runtime_subclass_impostor() -> None:
    # Given: an event subclass has valid inherited event fields.
    run = make_run(RunStatus.SUCCEEDED)
    event = SucceededEventImpostor(
        EventId("evt_abcdefgh"),
        CommandId("cmd_abcdefgh"),
        LATER,
        run.id,
        run.revision,
    )

    # When: event discrimination receives a non-union concrete class.
    result = event_kind(event)

    # Then: it must be rejected rather than inherit RUN_SUCCEEDED meaning.
    assert result == UnknownVariant("event", "SucceededEventImpostor")
    assert not hasattr(RunSucceeded, "kind")
