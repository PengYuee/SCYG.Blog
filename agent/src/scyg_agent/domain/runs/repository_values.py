"""Shared typed values for Run repository contracts."""

from dataclasses import dataclass
from datetime import datetime
from typing import Never, Self, override
from uuid import UUID

from .models import ExecutionOwnerId, RunId


@dataclass(frozen=True, slots=True)
class InvalidRepositoryInputError(ValueError):
    """Report an invalid repository boundary value without infrastructure details."""

    field: str
    rule: str

    @override
    def __str__(self) -> str:
        return f"{self.field} {self.rule}"


def reject_repository_input(field: str, rule: str) -> Never:
    """Raise one stable repository boundary error."""
    raise InvalidRepositoryInputError(field, rule)


@dataclass(frozen=True, slots=True)
class LeaseToken:
    """Carry an opaque UUID fencing token."""

    value: UUID

    @classmethod
    def parse(cls, raw: str) -> Self:
        """Parse an untrusted textual UUID into a fenced token."""
        try:
            return cls(UUID(raw))
        except ValueError:
            reject_repository_input("lease token", "must be a UUID")


@dataclass(frozen=True, slots=True)
class RunLease:
    """Return an immutable execution grant with its exact revision fence."""

    run_id: RunId
    owner: ExecutionOwnerId
    token: LeaseToken
    revision: int
    attempt: int
    expires_at: datetime
    cancellation_requested_at: datetime | None = None
