"""异步持久化 Run 输入源测试。"""

from collections.abc import Iterator, Mapping
from dataclasses import dataclass
from typing import override

import pytest
from pydantic import ValidationError

from scyg_agent.adapters.database.run_request_source import (
    MissingRunInputError,
    PersistedInputRow,
    PersistedRunRequestSource,
    map_input_row,
)
from scyg_agent.domain.runs import RunId, RunStatus
from scyg_agent.domain.runs.input import (
    InvalidRunInputError,
    MissingRunInput,
    RunInput,
    RunInputResult,
)
from tests.domain.runs.helpers import make_run


@dataclass(frozen=True, slots=True)
class InputSource:
    """返回注入结果的异步输入源。"""

    result: RunInputResult

    async def get_input(self, run_id: RunId) -> RunInputResult:
        del run_id
        return self.result


@dataclass(frozen=True, slots=True)
class RowMappingLike(Mapping[str, str | int | None]):
    """模拟 SQLAlchemy RowMapping 的 Mapping 协议而非 dict 继承。"""

    data: dict[str, str | int | None]

    @override
    def __getitem__(self, key: str) -> str | int | None:
        return self.data[key]

    @override
    def __iter__(self) -> Iterator[str]:
        return iter(self.data)

    @override
    def __len__(self) -> int:
        return len(self.data)


@pytest.mark.anyio
async def test_persisted_input_reaches_completion_request() -> None:
    # Given
    source = PersistedRunRequestSource(InputSource(RunInput("完整输入", "article-1")), "m")

    # When
    request = await source.request_for(make_run(RunStatus.PENDING))

    # Then
    assert request.messages[0].content == "完整输入"


@pytest.mark.anyio
async def test_historical_null_input_fails_with_typed_chinese_error() -> None:
    # Given
    run = make_run(RunStatus.PENDING)
    source = PersistedRunRequestSource(InputSource(MissingRunInput(run.id)), "m")

    # When / Then
    with pytest.raises(MissingRunInputError, match="缺少可执行"):
        _ = await source.request_for(run)


@pytest.mark.parametrize(
    ("message", "article"),
    [("", "article-1"), ("x" * 16_001, "article-1"), ("输入", ""), ("输入", "x" * 129)],
)
def test_new_run_input_rejects_empty_or_oversized_values(message: str, article: str) -> None:
    with pytest.raises(InvalidRunInputError):
        _ = RunInput(message, article)


@pytest.mark.parametrize("row", [None, (None, "article-1"), ("输入", None)])
def test_nullable_historical_rows_map_to_missing(
    row: tuple[str | None, str | None] | None,
) -> None:
    result = map_input_row(RunId("run_history01"), row)
    assert isinstance(result, MissingRunInput)


def test_complete_row_maps_capability_snapshot() -> None:
    result = map_input_row(
        RunId("run_history01"),
        ("输入", "article-1"),
        capability="write",
        recipe_id="writing-v1",
        recipe_version="v1",
        input_schema_version="v1",
        input_payload={"topic": "主题"},
        input_digest="a" * 64,
        locale="zh-CN",
    )
    assert result == RunInput(
        "输入",
        "article-1",
        "write",
        "writing-v1",
        "v1",
        "v1",
        {"topic": "主题"},
        "a" * 64,
        "zh-CN",
    )


def test_complete_row_maps_without_optional_capability_snapshot() -> None:
    result = map_input_row(RunId("run_history01"), ("输入", "article-1"))
    assert result == RunInput("输入", "article-1")


def test_sqlalchemy_mapping_shape_preserves_exact_scalars() -> None:
    mapping = RowMappingLike({"initial_message": "完整输入", "article_id": "article-1"})
    row = PersistedInputRow.model_validate(dict(mapping), strict=True)
    assert (row.initial_message, row.article_id) == ("完整输入", "article-1")


@pytest.mark.parametrize(
    "values",
    [
        {"initial_message": "输入"},
        {"initial_message": "输入", "article_id": "article-1", "extra": "拒绝"},
        {"initial_message": 1, "article_id": "article-1"},
        {"initial_message": "输入", "article_id": 1},
    ],
)
def test_mapping_copy_rejects_missing_extra_and_wrong_scalar_types(
    values: dict[str, str | int | None],
) -> None:
    mapping = RowMappingLike(values)
    with pytest.raises(ValidationError):
        _ = PersistedInputRow.model_validate(dict(mapping), strict=True)
