"""Trusted-row mapper between SQLAlchemy Run records and the pure domain."""

from scyg_agent.domain.runs import (
    ExecutionOwnerId,
    InteractionId,
    Run,
    RunId,
    RunStatus,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
    UserId,
)
from scyg_agent.domain.runs.errors import (
    InvalidEnumValueError,
    InvalidIdentifierError,
    InvalidRunError,
    InvalidRuntimeVersionError,
    InvalidTimestampError,
)
from scyg_agent.domain.runs.repository import DataIntegrityError

from .run_records import RunRecord


def map_run(record: RunRecord) -> Run | DataIntegrityError:
    """Parse one persisted row and contain domain integrity failures."""
    try:
        run_id = RunId(record.run_id)
        return Run(
            id=run_id,
            owner_user_id=UserId(record.owner_user_id),
            task_type=TaskType.parse(record.task_type),
            runtime=RuntimeSelection(
                RuntimeKind.parse(record.runtime_kind), record.runtime_version
            ),
            revision=record.revision,
            status=RunStatus.parse(record.status),
            created_at=record.created_at,
            updated_at=record.updated_at,
            attempt=record.attempt,
            execution_owner=(
                ExecutionOwnerId(record.lease_owner) if record.lease_owner is not None else None
            ),
            pending_interaction_id=(
                InteractionId(record.pending_interaction_id)
                if record.pending_interaction_id is not None
                else None
            ),
        )
    except (
        InvalidEnumValueError,
        InvalidIdentifierError,
        InvalidRunError,
        InvalidRuntimeVersionError,
        InvalidTimestampError,
    ) as error:
        return DataIntegrityError(record.run_id, str(error))
