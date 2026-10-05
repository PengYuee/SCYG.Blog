"""Stable capability input, output, and approval contracts."""

from __future__ import annotations

import hashlib
import json
from enum import StrEnum
from typing import Annotated, ClassVar, Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator

MAX_REFERENCE_ARTICLES = 20


class Capability(StrEnum):
    """Public capabilities accepted at the run boundary."""

    SEARCH = "search"
    WRITE = "write"
    POLISH = "polish"
    CHAT = "chat"


class RecipeId(StrEnum):
    """Application-owned recipe identifiers; never client supplied."""

    SEARCH_V1 = "search-v1"
    WRITING_V1 = "writing-v1"
    POLISH_V1 = "polish-v1"
    CHAT_V1 = "chat-v1"


class ContractModel(BaseModel):
    """Reject unknown fields so model output cannot smuggle control data."""

    model_config: ClassVar[ConfigDict] = ConfigDict(extra="forbid", frozen=True)


class SearchInput(ContractModel):
    """Search request bounded to Blog-local query retrieval."""

    query: Annotated[str, Field(min_length=1, max_length=16_000)]
    max_results: Annotated[int, Field(ge=1, le=20)] = 5


class WritingInput(ContractModel):
    """Writing request with optional bounded article references."""

    topic: Annotated[str, Field(min_length=1, max_length=2_000)]
    requirements: Annotated[str, Field(max_length=16_000)] = ""
    reference_article_ids: tuple[Annotated[str, Field(min_length=1, max_length=128)], ...] = ()

    @field_validator("reference_article_ids")
    @classmethod
    def bound_references(cls, value: tuple[str, ...]) -> tuple[str, ...]:
        """Reject unbounded reference expansion."""
        if len(value) > MAX_REFERENCE_ARTICLES:
            reason = f"at most {MAX_REFERENCE_ARTICLES} reference articles are allowed"
            raise ValueError(reason)
        return value


class PolishInput(ContractModel):
    """Polishing request containing user-provided prose."""

    content: Annotated[str, Field(min_length=1, max_length=100_000)]
    requirements: Annotated[str, Field(max_length=16_000)] = ""


class ChatInput(ContractModel):
    """Short conversational request."""

    message: Annotated[str, Field(min_length=1, max_length=16_000)]


CapabilityInput = SearchInput | WritingInput | PolishInput | ChatInput

INPUT_SCHEMA_VERSION = "v1"
OUTPUT_SCHEMA_VERSION = "v1"


def recipe_for_capability(capability: Capability) -> RecipeId:
    """Resolve the server-owned recipe for a public capability."""
    return {
        Capability.SEARCH: RecipeId.SEARCH_V1,
        Capability.WRITE: RecipeId.WRITING_V1,
        Capability.POLISH: RecipeId.POLISH_V1,
        Capability.CHAT: RecipeId.CHAT_V1,
    }[capability]


def input_payload(value: CapabilityInput) -> dict[str, object]:
    """Return the canonical JSON-compatible input snapshot."""
    return value.model_dump(mode="json")


def input_digest(value: CapabilityInput) -> str:
    """Hash the canonical input snapshot without including secrets."""
    encoded = json.dumps(
        input_payload(value), ensure_ascii=False, sort_keys=True, separators=(",", ":")
    )
    return hashlib.sha256(encoded.encode("utf-8")).hexdigest()


class InvalidCapabilityInputError(ValueError):
    """Raised when a public capability is paired with the wrong input."""


def validate_capability_input(capability: Capability, value: CapabilityInput) -> CapabilityInput:
    """Require the closed capability/input pairing before recipe resolution."""
    expected = {
        Capability.SEARCH: SearchInput,
        Capability.WRITE: WritingInput,
        Capability.POLISH: PolishInput,
        Capability.CHAT: ChatInput,
    }[capability]
    if type(value) is not expected:
        reason = f"{capability.value} requires {expected.__name__}"
        raise InvalidCapabilityInputError(reason)
    return value


class ArticleEvidence(ContractModel):
    """Tool-backed article evidence allowed in final output."""

    article_id: Annotated[str, Field(min_length=1, max_length=128)]
    title: Annotated[str, Field(min_length=1, max_length=1_000)]
    visibility: Literal["published", "owned_draft"]
    evidence: Annotated[str, Field(min_length=1, max_length=4_000)]


class SearchResponse(ContractModel):
    """Structured search result with cited evidence."""

    query: str
    conclusion: Annotated[str, Field(min_length=1, max_length=16_000)]
    articles: tuple[ArticleEvidence, ...] = ()


class ArticleDraft(ContractModel):
    """Structured writing result."""

    title: Annotated[str, Field(min_length=1, max_length=1_000)]
    outline: Annotated[str, Field(min_length=1, max_length=16_000)]
    markdown: Annotated[str, Field(min_length=1, max_length=200_000)]
    source_article_ids: tuple[str, ...] = ()


class PolishResponse(ContractModel):
    """Structured polishing result."""

    content: Annotated[str, Field(min_length=1, max_length=200_000)]
    change_summary: Annotated[str, Field(max_length=16_000)]


class ChatResponse(ContractModel):
    """Structured conversational result."""

    response: Annotated[str, Field(min_length=1, max_length=32_000)]
    source_article_ids: tuple[str, ...] = ()


CapabilityOutput = SearchResponse | ArticleDraft | PolishResponse | ChatResponse


class InvalidCapabilityOutputError(ValueError):
    """Raised when a capability produces the wrong structured output."""


def validate_capability_output(capability: Capability, value: CapabilityOutput) -> CapabilityOutput:
    """Require the closed capability/output pairing at the Runner boundary."""
    expected = {
        Capability.SEARCH: SearchResponse,
        Capability.WRITE: ArticleDraft,
        Capability.POLISH: PolishResponse,
        Capability.CHAT: ChatResponse,
    }[capability]
    if type(value) is not expected:
        reason = f"{capability.value} requires {expected.__name__}"
        raise InvalidCapabilityOutputError(reason)
    return value


def output_payload(value: CapabilityOutput) -> dict[str, object]:
    """Return the canonical JSON-compatible terminal result payload."""
    return value.model_dump(mode="json")


def output_digest(value: CapabilityOutput) -> str:
    """Hash the canonical terminal result without including secrets."""
    encoded = json.dumps(
        output_payload(value), ensure_ascii=False, sort_keys=True, separators=(",", ":")
    )
    return hashlib.sha256(encoded.encode("utf-8")).hexdigest()


class FailureKind(StrEnum):
    """Stable failure categories persisted by the Run layer."""

    VALIDATION = "validation_failure"
    RESULT_VALIDATION = "result_validation_failure"
    INVALID_REQUEST = "invalid_request"
    UNAUTHORIZED = "unauthorized"
    FORBIDDEN = "forbidden"
    DEPENDENCY = "dependency_unavailable"
    REDIS_FAILURE = "redis_failure"
    TOOL_FAILURE = "tool_failure"
    APPROVAL_REJECTED = "approval_rejected"
    CANCELLED = "cancelled"
    INTERNAL = "internal_failure"


class AgentFailure(ContractModel):
    """Sanitized terminal failure information."""

    kind: FailureKind
    message: Annotated[str, Field(min_length=1, max_length=512)]
    retryable: bool = False


class AgentInterrupt(ContractModel):
    """Framework-independent approval interruption."""

    interaction_id: Annotated[str, Field(min_length=1, max_length=128)]
    reason: Annotated[str, Field(min_length=1, max_length=512)]


class ApprovalDecision(StrEnum):
    """Allowed user decisions for an approval interaction."""

    APPROVE = "approve"
    EDIT = "edit"
    REJECT = "reject"


class ApprovalRequest(ContractModel):
    """Approval payload emitted before writing continues."""

    interaction_id: Annotated[str, Field(min_length=1, max_length=128)]
    title: Annotated[str, Field(min_length=1, max_length=1_000)]
    outline: Annotated[str, Field(min_length=1, max_length=16_000)]


class ApprovalReply(ContractModel):
    """User response resolving one approval interaction."""

    interaction_id: Annotated[str, Field(min_length=1, max_length=128)]
    decision: ApprovalDecision
    feedback: Annotated[str, Field(max_length=16_000)] = ""


CapabilityContract = (
    tuple[Literal[Capability.SEARCH], SearchInput, SearchResponse]
    | tuple[Literal[Capability.WRITE], WritingInput, ArticleDraft]
    | tuple[Literal[Capability.POLISH], PolishInput, PolishResponse]
    | tuple[Literal[Capability.CHAT], ChatInput, ChatResponse]
)
