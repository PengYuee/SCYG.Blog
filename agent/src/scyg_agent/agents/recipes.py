"""Closed server-owned Agent recipe catalog."""

from dataclasses import dataclass
from typing import Final

from pydantic import BaseModel

from .contracts import (
    ArticleDraft,
    Capability,
    ChatInput,
    ChatResponse,
    PolishInput,
    PolishResponse,
    RecipeId,
    SearchInput,
    SearchResponse,
    WritingInput,
)
from .tool_catalog import ReadToolName


@dataclass(frozen=True, slots=True)
class AgentRecipe:
    """Describe one executable capability contract without framework objects."""

    recipe_id: RecipeId
    version: str
    capability: Capability
    input_schema: type[BaseModel]
    output_schema: type[BaseModel]
    prompt: str
    model_tier: str
    tool_names: tuple[ReadToolName, ...] = ()


class InvalidRecipeRegistryError(ValueError):
    """Reject a recipe catalog that is incomplete or ambiguous."""


EXPECTED_RECIPE_COUNT: Final = 4


@dataclass(frozen=True, slots=True)
class RecipeRegistry:
    """Hold the complete immutable recipe catalog."""

    recipes: tuple[AgentRecipe, ...]

    def __post_init__(self) -> None:
        """Reject duplicate or incomplete recipe keys."""
        keys = {(recipe.recipe_id, recipe.version) for recipe in self.recipes}
        capabilities = {recipe.capability for recipe in self.recipes}
        if (
            len(keys) != len(self.recipes)
            or len(self.recipes) != EXPECTED_RECIPE_COUNT
            or len(capabilities) != EXPECTED_RECIPE_COUNT
            or any(recipe.version != "v1" or not recipe.prompt for recipe in self.recipes)
            or any(
                len(set(recipe.tool_names)) != len(recipe.tool_names)
                or (recipe.recipe_id is not RecipeId.SEARCH_V1 and bool(recipe.tool_names))
                for recipe in self.recipes
            )
        ):
            raise InvalidRecipeRegistryError

    def resolve(self, recipe_id: RecipeId, version: str) -> AgentRecipe | None:
        """Return the exact server-approved recipe or no match."""
        return next(
            (
                recipe
                for recipe in self.recipes
                if recipe.recipe_id is recipe_id and recipe.version == version
            ),
            None,
        )


SEARCH_PROMPT: Final = (
    "Search the site using registered Blog management read tools. Cite only articles "
    "returned for the current user, including authorized drafts and archived articles. "
    "Use numeric article IDs and page/page_size pagination; never invent evidence. "
    "Return a concise structured result."
)
WRITING_PROMPT: Final = "Write a structured article draft from the supplied topic and requirements."
POLISH_PROMPT: Final = "Polish the supplied prose without inventing facts."
CHAT_PROMPT: Final = "Answer the user clearly and concisely."


def default_recipe_registry() -> RecipeRegistry:
    """Build the four approved v1 capability recipes."""
    return RecipeRegistry(
        (
            AgentRecipe(
                RecipeId.SEARCH_V1,
                "v1",
                Capability.SEARCH,
                SearchInput,
                SearchResponse,
                SEARCH_PROMPT,
                "standard",
                tuple(ReadToolName),
            ),
            AgentRecipe(
                RecipeId.WRITING_V1,
                "v1",
                Capability.WRITE,
                WritingInput,
                ArticleDraft,
                WRITING_PROMPT,
                "standard",
            ),
            AgentRecipe(
                RecipeId.POLISH_V1,
                "v1",
                Capability.POLISH,
                PolishInput,
                PolishResponse,
                POLISH_PROMPT,
                "fast",
            ),
            AgentRecipe(
                RecipeId.CHAT_V1,
                "v1",
                Capability.CHAT,
                ChatInput,
                ChatResponse,
                CHAT_PROMPT,
                "standard",
            ),
        )
    )
