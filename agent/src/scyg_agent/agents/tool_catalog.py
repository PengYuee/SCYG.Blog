"""Agent 侧封闭、不可变的只读工具目录."""

from enum import StrEnum
from typing import Final


class ReadToolName(StrEnum):
    """Management reads authorized by Blog for the current user."""

    SEARCH_ARTICLES = "search_articles"
    GET_ARTICLE = "get_article"


READ_TOOL_CATALOG: Final[tuple[ReadToolName, ...]] = tuple(ReadToolName)
