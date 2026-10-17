"""Attempt-local public budgets; synchronous reservations precede asynchronous dispatch."""

import json
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Final, final

from scyg_agent.config import PublicSearchSettings

EXECUTION_TIME_LIMIT: Final = "execution time"
TOOL_CALL_LIMIT: Final = "tool calls"
READ_CHARACTER_LIMIT: Final = "read characters"


@final
class PolicyLimitError(BaseException):
    """A public policy limit was reached; never turn this into tool output."""

    def __init__(self, limit: str) -> None:
        """Carry only the public name of the exhausted policy."""
        self.limit = limit
        super().__init__(f"Public search {limit} limit reached")


@dataclass(slots=True)
class PublicAttemptBudget:
    """Shared by all concurrent tool/model calls in one execution attempt."""

    settings: PublicSearchSettings
    deadline: datetime
    tool_calls: int = 0
    read_chars: int = 0

    def check_deadline(self) -> None:
        """Reject dispatch after the persistent total execution deadline."""
        if datetime.now(UTC) >= self.deadline:
            raise PolicyLimitError(EXECUTION_TIME_LIMIT)

    def reserve_tool(self) -> None:
        """Reserve before awaiting so concurrent RPCs cannot overspend."""
        self.check_deadline()
        if self.tool_calls >= self.settings.max_tool_calls_per_attempt:
            raise PolicyLimitError(TOOL_CALL_LIMIT)
        self.tool_calls += 1

    def authorize_result(self, payload: object) -> None:
        """Charge the complete SDK serialization or reject the whole result."""
        self.check_deadline()
        size = len(json.dumps(payload, ensure_ascii=False))
        if self.read_chars + size > self.settings.max_read_chars_per_attempt:
            raise PolicyLimitError(READ_CHARACTER_LIMIT)
        self.read_chars += size
