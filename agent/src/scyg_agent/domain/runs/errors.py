"""Typed construction failures for Run domain boundaries and invariants."""

from dataclasses import dataclass
from typing import override


@dataclass(frozen=True, slots=True)
class InvalidIdentifierError(Exception):
    """Report an identifier that cannot cross into the Run domain."""

    identifier_type: str
    value: str

    @override
    def __str__(self) -> str:
        # Render a stable diagnostic without interpretation by callers.
        return f"invalid {self.identifier_type}: {self.value!r}"


@dataclass(frozen=True, slots=True)
class InvalidEnumValueError(Exception):
    """Report an unknown external value for a closed domain enum."""

    enum_type: str
    value: str

    @override
    def __str__(self) -> str:
        # Render the closed enum name and rejected value.
        return f"invalid {self.enum_type}: {self.value!r}"


@dataclass(frozen=True, slots=True)
class InvalidRuntimeVersionError(Exception):
    """Report a runtime profile version outside the immutable vN form."""

    value: str

    @override
    def __str__(self) -> str:
        # Render a stable runtime version diagnostic.
        return f"invalid runtime version: {self.value!r}"


@dataclass(frozen=True, slots=True)
class InvalidTimestampError(Exception):
    """Report a timestamp that is not an aware UTC instant."""

    field: str

    @override
    def __str__(self) -> str:
        # Name the invalid timestamp field without rendering its value.
        return f"{self.field} must be timezone-aware UTC"


@dataclass(frozen=True, slots=True)
class InvalidRunError(Exception):
    """Report an aggregate invariant violation."""

    invariant: str

    @override
    def __str__(self) -> str:
        # Render the stable invariant name.
        return f"invalid run: {self.invariant}"


@dataclass(frozen=True, slots=True)
class UnknownVariant:
    """Reject a runtime concrete class outside a closed domain union."""

    family: str
    concrete_type: str
