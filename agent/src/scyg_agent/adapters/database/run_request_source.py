"""从持久化输入构造 SIMPLE 请求。."""

from dataclasses import dataclass
from typing import ClassVar, override

from pydantic import BaseModel, ConfigDict
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.domain.runs import Run, RunId
from scyg_agent.domain.runs.input import MissingRunInput, RunInput, RunInputSource
from scyg_agent.runtimes.simple.models import CompletionRequest, Message, MessageRole

from .run_records import RunRecord


class PersistedInputRow(BaseModel):
    """严格解析 SQLAlchemy mapping 返回的两个可空标量。."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid", strict=True)
    initial_message: str | None
    article_id: str | None
    capability: str | None = None
    recipe_id: str | None = None
    recipe_version: str | None = None
    input_schema_version: str | None = None
    input_payload: dict[str, object] | None = None
    input_digest: str | None = None
    locale: str | None = None


@dataclass(frozen=True, slots=True)
class MissingRunInputError(RuntimeError):
    """阻止执行缺少创建输入的历史 Run。."""

    @override
    def __str__(self) -> str:
        """返回不包含持久化值的中文错误。."""
        return "Run 缺少可执行的创建输入"


@dataclass(frozen=True, slots=True)
class PersistedRunRequestSource:
    """异步读取数据库且不缓存传输输入。."""

    source: RunInputSource
    model: str

    async def request_for(self, run: Run) -> CompletionRequest:
        """把持久化用户消息映射为提供方请求。."""
        result = await self.source.get_input(run.id)
        if isinstance(result, RunInput):
            return CompletionRequest(
                model=self.model,
                messages=(Message(role=MessageRole.USER, content=result.initial_message),),
            )
        raise MissingRunInputError


@dataclass(frozen=True, slots=True)
class PostgreSQLRunInputSource:
    """使用独立短会话读取 Run 创建输入."""

    sessions: async_sessionmaker[AsyncSession]

    async def get_input(self, run_id: RunId) -> RunInput | MissingRunInput:
        """读取输入和 capability 快照、并把历史空列映射为类型化结果."""
        async with self.sessions() as session:
            result = await session.execute(
                select(
                    RunRecord.initial_message.label("initial_message"),
                    RunRecord.article_id.label("article_id"),
                    RunRecord.capability.label("capability"),
                    RunRecord.recipe_id.label("recipe_id"),
                    RunRecord.recipe_version.label("recipe_version"),
                    RunRecord.input_schema_version.label("input_schema_version"),
                    RunRecord.input_payload.label("input_payload"),
                    RunRecord.input_digest.label("input_digest"),
                    RunRecord.locale.label("locale"),
                ).where(RunRecord.run_id == str(run_id))
            )
            row = result.mappings().one_or_none()
        if row is None:
            return map_input_row(run_id, None)
        parsed = PersistedInputRow.model_validate(dict(row), strict=True)
        return map_input_row(
            run_id,
            (parsed.initial_message, parsed.article_id),
            capability=parsed.capability,
            recipe_id=parsed.recipe_id,
            recipe_version=parsed.recipe_version,
            input_schema_version=parsed.input_schema_version,
            input_payload=parsed.input_payload,
            input_digest=parsed.input_digest,
            locale=parsed.locale,
        )


def map_input_row(  # noqa: PLR0913
    run_id: RunId,
    row: tuple[str | None, str | None] | None,
    *,
    capability: str | None = None,
    recipe_id: str | None = None,
    recipe_version: str | None = None,
    input_schema_version: str | None = None,
    input_payload: dict[str, object] | None = None,
    input_digest: str | None = None,
    locale: str | None = None,
) -> RunInput | MissingRunInput:
    """把可空历史行映射为严格输入或缺失结果."""
    if row is None:
        return MissingRunInput(run_id)
    initial_message, article_id = row
    if initial_message is None or article_id is None:
        return MissingRunInput(run_id)
    return RunInput(
        initial_message,
        article_id,
        capability,
        recipe_id,
        recipe_version,
        input_schema_version,
        input_payload,
        input_digest,
        locale,
    )
