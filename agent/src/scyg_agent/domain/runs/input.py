"""Run 创建输入的持久化领域契约。."""

from dataclasses import dataclass
from typing import Protocol, override

from .models import RunId

MAX_MESSAGE_LENGTH = 16_000
MAX_ARTICLE_ID_LENGTH = 128


@dataclass(frozen=True, slots=True)
class RunInput:
    """保存新 Run 执行输入及可选 capability 快照。."""

    initial_message: str
    article_id: str
    capability: str | None = None
    recipe_id: str | None = None
    recipe_version: str | None = None
    input_schema_version: str | None = None
    input_payload: dict[str, object] | None = None
    input_digest: str | None = None
    locale: str | None = None

    def __post_init__(self) -> None:
        """拒绝空值和超出契约长度的输入。."""
        if not self.initial_message or len(self.initial_message) > MAX_MESSAGE_LENGTH:
            field = "initial_message"
            raise InvalidRunInputError(field)
        if not self.article_id or len(self.article_id) > MAX_ARTICLE_ID_LENGTH:
            field = "article_id"
            raise InvalidRunInputError(field)


class InvalidRunInputError(ValueError):
    """报告不回显正文的输入校验失败。."""

    field: str

    def __init__(self, field: str) -> None:
        """保存失败字段名而不保存正文。."""
        super().__init__(field)
        self.field = field

    @override
    def __str__(self) -> str:
        return f"Run 输入字段无效: {self.field}"


@dataclass(frozen=True, slots=True)
class MissingRunInput:
    """表示历史 Run 没有可恢复的创建输入。."""

    run_id: RunId


type RunInputResult = RunInput | MissingRunInput


class RunInputSource(Protocol):
    """异步读取 Agent 持久化输入真值。."""

    async def get_input(self, run_id: RunId) -> RunInputResult:
        """读取输入或返回历史缺失结果。."""
        ...  # pragma: no cover
