"""Exhaustive status by command transition matrix tests."""

from itertools import product
from typing import Final

import pytest

from scyg_agent.domain.runs import (
    CommandKind,
    IllegalTransition,
    RunStatus,
    Transitioned,
    transition,
)

from .helpers import command_for, make_run

LEGAL_TRANSITIONS: Final = (
    (RunStatus.PENDING, CommandKind.CLAIM, RunStatus.RUNNING),
    (RunStatus.PENDING_RESUME, CommandKind.CLAIM, RunStatus.RUNNING),
    (RunStatus.RUNNING, CommandKind.REQUEST_INPUT, RunStatus.WAITING_INPUT),
    (RunStatus.WAITING_INPUT, CommandKind.SUBMIT_INPUT, RunStatus.PENDING_RESUME),
    (RunStatus.RUNNING, CommandKind.SUCCEED, RunStatus.SUCCEEDED),
    (RunStatus.RUNNING, CommandKind.FAIL, RunStatus.FAILED),
    (RunStatus.PENDING, CommandKind.CANCEL, RunStatus.CANCELLED),
    (RunStatus.RUNNING, CommandKind.CANCEL, RunStatus.CANCELLED),
    (RunStatus.WAITING_INPUT, CommandKind.CANCEL, RunStatus.CANCELLED),
    (RunStatus.PENDING_RESUME, CommandKind.CANCEL, RunStatus.CANCELLED),
    (RunStatus.RUNNING, CommandKind.RELEASE_EXECUTION, RunStatus.PENDING),
)
MATRIX_CASES: Final = tuple(product(RunStatus, CommandKind))


@pytest.mark.parametrize(
    ("status", "command_kind"),
    MATRIX_CASES,
    ids=[f"{status.value}--{command.value}" for status, command in MATRIX_CASES],
)
def test_every_status_command_pair_has_exact_outcome(
    status: RunStatus,
    command_kind: CommandKind,
) -> None:
    # Given: one cell from the complete durable status by command matrix.
    run = make_run(status)
    command = command_for(command_kind)

    # When: the pure state machine evaluates the cell.
    result = transition(run, command)

    # Then: legal cells transition exactly; every other cell has one typed failure.
    target = next(
        (
            legal_target
            for source, legal_command, legal_target in LEGAL_TRANSITIONS
            if source is status and legal_command is command_kind
        ),
        None,
    )
    if target is None:
        assert result == IllegalTransition(status=status, command=command_kind)
    else:
        assert isinstance(result, Transitioned)
        assert result.run.status is target
        assert result.run.revision == run.revision + 1
        assert len(result.events) == 1


def test_transition_matrix_counts_are_stable() -> None:
    # Given/When: all seven statuses and seven commands form the matrix.
    legal_count = len(LEGAL_TRANSITIONS)
    total_count = len(MATRIX_CASES)

    # Then: additions must update the explicit legal and illegal contract counts.
    assert total_count == 49
    assert legal_count == 11
    assert total_count - legal_count == 38


@pytest.mark.parametrize("terminal", [RunStatus.SUCCEEDED, RunStatus.FAILED, RunStatus.CANCELLED])
def test_terminal_status_rejects_every_command(terminal: RunStatus) -> None:
    # Given: a terminal aggregate.
    run = make_run(terminal)

    # When/Then: no mutating command can leave the terminal state.
    for command_kind in CommandKind:
        result = transition(run, command_for(command_kind))
        assert result == IllegalTransition(status=terminal, command=command_kind)


def test_duplicate_wait_and_second_input_response_fail_deterministically() -> None:
    # Given: input is already awaited or already resolved.
    waiting = make_run(RunStatus.WAITING_INPUT)
    pending_resume = make_run(RunStatus.PENDING_RESUME)

    # When/Then: repeated lifecycle commands cannot replay the prior mutation.
    assert transition(waiting, command_for(CommandKind.REQUEST_INPUT)) == IllegalTransition(
        RunStatus.WAITING_INPUT,
        CommandKind.REQUEST_INPUT,
    )
    assert transition(pending_resume, command_for(CommandKind.SUBMIT_INPUT)) == IllegalTransition(
        RunStatus.PENDING_RESUME,
        CommandKind.SUBMIT_INPUT,
    )
