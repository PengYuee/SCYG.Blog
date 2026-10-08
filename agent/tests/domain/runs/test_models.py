"""Run domain value and invariant tests."""

from collections.abc import Callable
from dataclasses import FrozenInstanceError, replace

import pytest

from scyg_agent.domain.runs import (
    CommandId,
    EventId,
    ExecutionOwnerId,
    InteractionId,
    InvalidEnumValueError,
    InvalidIdentifierError,
    InvalidRunError,
    InvalidRuntimeVersionError,
    InvalidTimestampError,
    OperationId,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    ToolCallId,
    UserId,
)

from .helpers import NOW, make_run

type IdentifierValue = (
    RunId
    | UserId
    | OperationId
    | CommandId
    | EventId
    | InteractionId
    | ToolCallId
    | ExecutionOwnerId
)


@pytest.mark.parametrize(
    ("identifier_type", "valid"),
    [
        (RunId, "run_12345678"),
        (RunId, "short.safe~id"),
        (UserId, "user-1"),
        (OperationId, "create-run:123"),
        (CommandId, "cmd_12345678"),
        (EventId, "evt_12345678"),
        (InteractionId, "int_12345678"),
        (InteractionId, "opaque response"),
        (ToolCallId, "tool_12345678"),
        (ExecutionOwnerId, "worker_12345678"),
    ],
)
def test_identifier_accepts_valid_branded_value(
    identifier_type: Callable[[str], IdentifierValue],
    valid: str,
) -> None:
    # Given/When: a valid external identity is parsed by its semantic type.
    identifier = identifier_type(valid)

    # Then: the value remains branded and stable.
    assert identifier.value == valid
    assert str(identifier) == valid


@pytest.mark.parametrize(
    ("identifier_type", "invalid"),
    [
        (RunId, ""),
        (RunId, ".."),
        (UserId, ""),
        (OperationId, "has spaces"),
        (CommandId, "command_12345678"),
        (EventId, "evt_bad!value"),
        (InteractionId, "界" * 43),
        (ToolCallId, "tool_short"),
        (ExecutionOwnerId, "worker_short"),
    ],
)
def test_identifier_rejects_empty_or_malformed_value(
    identifier_type: Callable[[str], IdentifierValue],
    invalid: str,
) -> None:
    # Given/When/Then: malformed boundary identity returns its typed parser exception.
    with pytest.raises(InvalidIdentifierError):
        _ = identifier_type(invalid)


@pytest.mark.parametrize(
    ("parser", "raw"),
    [(TaskType.parse, "unknown"), (RuntimeKind.parse, "plugin"), (RunStatus.parse, "paused")],
)
def test_closed_enum_parser_rejects_unknown_value(
    parser: Callable[[str], TaskType | RuntimeKind | RunStatus],
    raw: str,
) -> None:
    # Given/When/Then: an unknown external enum value cannot become a domain variant.
    with pytest.raises(InvalidEnumValueError):
        _ = parser(raw)


def test_runtime_selection_requires_registered_version_shape() -> None:
    # Given/When/Then: a runtime profile version must use the immutable vN shape.
    with pytest.raises(InvalidRuntimeVersionError):
        _ = RuntimeSelection(RuntimeKind.DEEP, "latest")


def test_run_rejects_naive_timestamp() -> None:
    # Given: an aggregate timestamp has no timezone proof.
    run = make_run(RunStatus.PENDING)

    # When/Then: construction rejects the ambiguous instant.
    with pytest.raises(InvalidTimestampError):
        _ = replace(run, updated_at=NOW.replace(tzinfo=None))


def test_run_rejects_execution_owner_outside_running() -> None:
    # Given: a pending aggregate incorrectly carries execution ownership.
    run = make_run(RunStatus.PENDING)

    # When/Then: aggregate invariants reject the impossible state.
    with pytest.raises(InvalidRunError):
        _ = replace(run, execution_owner=ExecutionOwnerId("worker_abcdefgh"))


def test_waiting_run_requires_pending_interaction_identity() -> None:
    # Given: a valid waiting aggregate.
    run = make_run(RunStatus.WAITING_INPUT)

    # When/Then: removing its interaction binding violates the waiting invariant.
    with pytest.raises(InvalidRunError):
        _ = replace(run, pending_interaction_id=None)


def test_non_waiting_run_rejects_pending_interaction_identity() -> None:
    # Given: a pending aggregate is not waiting for user input.
    run = make_run(RunStatus.PENDING)

    # When/Then: it cannot retain an interaction binding.
    with pytest.raises(InvalidRunError):
        _ = replace(run, pending_interaction_id=InteractionId("int_abcdefgh"))


def test_run_values_are_frozen() -> None:
    # Given: a valid immutable aggregate.
    run = make_run(RunStatus.PENDING)

    # When/Then: direct mutation is rejected by the value object contract.
    field = "revision"
    with pytest.raises(FrozenInstanceError):
        setattr(run, field, 99)


def test_run_timestamp_is_timezone_aware_utc() -> None:
    # Given/When: a valid aggregate is built from an injected UTC instant.
    run = make_run(RunStatus.PENDING)

    # Then: the exact deterministic instant is retained.
    assert run.created_at == NOW
    assert run.created_at.utcoffset() is not None
