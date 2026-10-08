"""Decode persisted capability snapshots at the Agent boundary."""

import json
from dataclasses import dataclass
from typing import Final, cast

from scyg_agent.domain.runs.input import RunInput

from .contracts import (
    INPUT_SCHEMA_VERSION,
    Capability,
    CapabilityInput,
    input_digest,
    input_payload,
    recipe_for_capability,
    validate_capability_input,
)
from .recipes import AgentRecipe, RecipeRegistry


@dataclass(frozen=True, slots=True)
class AgentInputSnapshot:
    """Carry one validated capability input and its frozen recipe identity."""

    capability: Capability
    recipe: AgentRecipe
    value: CapabilityInput
    locale: str


class InvalidAgentInputError(ValueError):
    """Reject an incomplete, forged, or stale persisted capability snapshot."""


MAX_LOCALE_LENGTH: Final = 32
_REQUIRED_VERSION: Final = "v1"


def decode_agent_input(run_input: RunInput, registry: RecipeRegistry) -> AgentInputSnapshot:
    """Parse and verify the immutable capability snapshot stored with a Run."""
    capability = _parse_capability(run_input)
    recipe = _resolve_recipe(run_input, capability, registry)
    value = _parse_value(run_input, capability, recipe)
    locale = run_input.locale or "und"
    if not locale or len(locale) > MAX_LOCALE_LENGTH:
        raise InvalidAgentInputError
    return AgentInputSnapshot(capability, recipe, value, locale)


def _parse_capability(run_input: RunInput) -> Capability:
    """Parse the persisted capability enum without echoing its value."""
    try:
        return Capability(run_input.capability or "")
    except ValueError as error:
        raise InvalidAgentInputError from error


def _resolve_recipe(
    run_input: RunInput, capability: Capability, registry: RecipeRegistry
) -> AgentRecipe:
    """Require the persisted recipe identity to match the server catalog."""
    recipe_id = recipe_for_capability(capability)
    version = run_input.recipe_version
    if (
        run_input.recipe_id != recipe_id.value
        or version != _REQUIRED_VERSION
        or run_input.input_schema_version != INPUT_SCHEMA_VERSION
    ):
        raise InvalidAgentInputError
    recipe = registry.resolve(recipe_id, _REQUIRED_VERSION)
    if recipe is None or recipe.capability is not capability:
        raise InvalidAgentInputError
    return recipe


def _parse_value(
    run_input: RunInput, capability: Capability, recipe: AgentRecipe
) -> CapabilityInput:
    """Strictly parse and digest-check the persisted input payload."""
    if run_input.input_payload is None or run_input.input_digest is None:
        raise InvalidAgentInputError
    try:
        value = cast(
            "CapabilityInput",
            recipe.input_schema.model_validate_json(
                json.dumps(run_input.input_payload, allow_nan=False), strict=True
            ),
        )
        _ = validate_capability_input(capability, value)
    except (TypeError, ValueError) as error:
        raise InvalidAgentInputError from error
    if (
        input_payload(value) != run_input.input_payload
        or input_digest(value) != run_input.input_digest
    ):
        raise InvalidAgentInputError
    return value
