"""Application-owned request context for Agent execution."""

from dataclasses import dataclass
from enum import StrEnum

from scyg_agent.domain.runs import UserId

from .contracts import Capability, RecipeId

MAX_LOCALE_LENGTH = 32


class InvalidAgentContextError(ValueError):
    """Reject an invalid application-owned Agent request context."""


class Quality(StrEnum):
    """Closed model quality policy selected by Blog-facing application code."""

    FAST = "fast"
    STANDARD = "standard"
    STRONG = "strong"


@dataclass(frozen=True, slots=True)
class AgentRequestContext:
    """Carry authenticated ownership and bounded request metadata."""

    owner_user_id: UserId
    run_id: str
    capability: Capability
    recipe_id: RecipeId
    recipe_version: str
    locale: str
    quality: Quality
    scopes: tuple[str, ...] = ()

    def __post_init__(self) -> None:
        """Reject a context whose recipe identity disagrees with capability."""
        expected = {
            Capability.SEARCH: RecipeId.SEARCH_V1,
            Capability.WRITE: RecipeId.WRITING_V1,
            Capability.POLISH: RecipeId.POLISH_V1,
            Capability.CHAT: RecipeId.CHAT_V1,
        }[self.capability]
        if self.recipe_id is not expected or self.recipe_version != "v1":
            raise InvalidAgentContextError
        if not self.locale or len(self.locale) > MAX_LOCALE_LENGTH:
            raise InvalidAgentContextError


@dataclass(frozen=True, slots=True)
class AgentInterrupt:
    """Framework-independent approval interruption exposed to the application."""

    interaction_id: str
    recipe_id: RecipeId
    reason: str


__all__ = ["AgentInterrupt", "AgentRequestContext", "Quality"]
