"""Revision, ownership, and event transition tests."""

from dataclasses import FrozenInstanceError, replace

import pytest

from scyg_agent.domain.runs import (
    ClaimRun,
    CommandId,
    CommandKind,
    DomainEvent,
    EventId,
    EventKind,
    ExecutionOwnerId,
    InputRequested,
    InputResolved,
    InteractionId,
    InteractionMismatch,
    RevisionMismatch,
    RunStatus,
    StatusChanged,
    SubmitInput,
    TimestampRegression,
    Transitioned,
    event_kind,
    transition,
)

from .helpers import LATER, command_for, make_run


def legal_source(command_kind: CommandKind) -> RunStatus:
    """Return one legal source status for a command category."""
    sources = {
        CommandKind.CLAIM: RunStatus.PENDING,
        CommandKind.REQUEST_INPUT: RunStatus.RUNNING,
        CommandKind.SUBMIT_INPUT: RunStatus.WAITING_INPUT,
        CommandKind.SUCCEED: RunStatus.RUNNING,
        CommandKind.FAIL: RunStatus.RUNNING,
        CommandKind.CANCEL: RunStatus.PENDING,
        CommandKind.RELEASE_EXECUTION: RunStatus.RUNNING,
    }
    return sources[command_kind]


@pytest.mark.parametrize("command_kind", tuple(CommandKind))
def test_stale_revision_rejects_every_mutating_command(command_kind: CommandKind) -> None:
    # Given: a valid command category carries an obsolete expected revision.
    run = make_run(legal_source(command_kind), revision=5)
    command = command_for(command_kind, expected_revision=4)

    # When: revision fencing runs before transition evaluation.
    result = transition(run, command)

    # Then: no aggregate or event is produced.
    assert result == RevisionMismatch(expected_revision=4, actual_revision=5)
    assert run.revision == 5


def test_waiting_input_releases_execution_ownership() -> None:
    # Given: a claimed running aggregate requests user input.
    run = make_run(RunStatus.RUNNING)
    command = command_for(CommandKind.REQUEST_INPUT)

    # When: the request transition succeeds.
    result = transition(run, command)

    # Then: durable waiting state has no execution owner and names the interaction.
    assert isinstance(result, Transitioned)
    assert result.run.status is RunStatus.WAITING_INPUT
    assert result.run.execution_owner is None
    assert result.run.pending_interaction_id == InteractionId("int_12345678")
    assert result.events == (
        InputRequested(
            event_id=EventId("evt_12345678"),
            command_id=CommandId("cmd_12345678"),
            occurred_at=LATER,
            run_id=run.id,
            revision=6,
            interaction_id=InteractionId("int_12345678"),
        ),
    )


def test_pending_resume_remains_unowned_until_claim() -> None:
    # Given: a waiting aggregate receives its one interaction response.
    waiting = make_run(RunStatus.WAITING_INPUT)

    # When: input resolution queues durable resume.
    resolved = transition(waiting, command_for(CommandKind.SUBMIT_INPUT))

    # Then: resume is pending without execution ownership.
    assert isinstance(resolved, Transitioned)
    assert resolved.run.status is RunStatus.PENDING_RESUME
    assert resolved.run.execution_owner is None
    assert isinstance(resolved.events[0], InputResolved)


def test_submit_input_rejects_a_different_interaction_identity() -> None:
    # Given: a waiting run is bound to the interaction it requested.
    waiting = make_run(RunStatus.WAITING_INPUT)
    command = SubmitInput(
        CommandId("cmd_abcdefgh"),
        EventId("evt_abcdefgh"),
        waiting.revision,
        LATER,
        InteractionId("int_abcdefgh"),
    )

    # When: a response names a different valid interaction.
    result = transition(waiting, command)

    # Then: the mismatch is typed and cannot advance the aggregate.
    assert result == InteractionMismatch(
        expected=InteractionId("int_12345678"),
        actual=InteractionId("int_abcdefgh"),
    )
    assert waiting.status is RunStatus.WAITING_INPUT
    assert waiting.revision == 5


def test_transition_rejects_time_older_than_current_aggregate() -> None:
    # Given: a valid command instant precedes the aggregate's latest mutation.
    pending = make_run(RunStatus.PENDING)
    current = replace(pending, updated_at=LATER)
    command = ClaimRun(
        CommandId("cmd_abcdefgh"),
        EventId("evt_abcdefgh"),
        current.revision,
        pending.created_at,
        ExecutionOwnerId("worker_abcdefgh"),
    )

    # When: the stale instant is evaluated for a mutating command.
    result = transition(current, command)

    # Then: time cannot move backward or mutate the aggregate.
    assert result == TimestampRegression(current=LATER, occurred_at=pending.created_at)
    assert current.updated_at == LATER
    assert current.revision == 5


def test_claim_assigns_owner_and_increments_attempt_once() -> None:
    # Given: an unowned pending-resume aggregate.
    run = make_run(RunStatus.PENDING_RESUME)

    # When: a worker atomically claims execution.
    result = transition(run, command_for(CommandKind.CLAIM))

    # Then: ownership, attempt, and revision change exactly once.
    assert isinstance(result, Transitioned)
    assert result.run.execution_owner == ExecutionOwnerId("worker_abcdefgh")
    assert result.run.attempt == run.attempt + 1
    assert result.run.revision == run.revision + 1
    assert isinstance(result.events[0], StatusChanged)


def test_successful_lifecycle_emits_ordered_stable_event_variants() -> None:
    # Given: a pending run and deterministic command/event identities.
    run = make_run(RunStatus.PENDING)
    commands = (
        command_for(CommandKind.CLAIM, expected_revision=5, identity="aaaaaaaa"),
        command_for(CommandKind.REQUEST_INPUT, expected_revision=6, identity="bbbbbbbb"),
        command_for(CommandKind.SUBMIT_INPUT, expected_revision=7, identity="cccccccc"),
        command_for(CommandKind.CLAIM, expected_revision=8, identity="dddddddd"),
        command_for(CommandKind.SUCCEED, expected_revision=9, identity="eeeeeeee"),
    )

    # When: the lifecycle is reduced through its only legal resume path.
    events: list[DomainEvent] = []
    current = run
    for command in commands:
        result = transition(current, command)
        assert isinstance(result, Transitioned)
        current = result.run
        events.extend(result.events)

    # Then: event meaning and aggregate revisions are deterministic and ordered.
    assert tuple(event_kind(event) for event in events) == (
        EventKind.STATUS_CHANGED,
        EventKind.INPUT_REQUESTED,
        EventKind.INPUT_RESOLVED,
        EventKind.STATUS_CHANGED,
        EventKind.RUN_SUCCEEDED,
    )
    assert tuple(event.revision for event in events) == (6, 7, 8, 9, 10)
    assert len({event.event_id for event in events}) == len(events)
    assert current.status is RunStatus.SUCCEEDED
    assert current.revision == 10


@pytest.mark.parametrize(
    ("command_kind", "expected_kind"),
    [
        (CommandKind.FAIL, EventKind.RUN_FAILED),
        (CommandKind.CANCEL, EventKind.RUN_CANCELLED),
        (CommandKind.RELEASE_EXECUTION, EventKind.EXECUTION_RELEASED),
    ],
)
def test_terminal_and_release_events_have_stable_discriminators(
    command_kind: CommandKind,
    expected_kind: EventKind,
) -> None:
    # Given: a command with one legal source status.
    run = make_run(legal_source(command_kind))

    # When: the transition emits its payload-free semantic event.
    result = transition(run, command_for(command_kind))

    # Then: the event maps to its stable closed discriminator.
    assert isinstance(result, Transitioned)
    assert event_kind(result.events[0]) is expected_kind


def test_events_are_frozen() -> None:
    # Given: a successful transition emits an immutable event.
    result = transition(make_run(RunStatus.PENDING), command_for(CommandKind.CLAIM))
    assert isinstance(result, Transitioned)

    # When/Then: downstream consumers cannot mutate event identity.
    field = "revision"
    with pytest.raises(FrozenInstanceError):
        setattr(result.events[0], field, 99)
