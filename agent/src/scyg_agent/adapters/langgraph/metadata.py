"""LangGraph checkpoint metadata 的类型化恢复边界."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Annotated, ClassVar, Literal, TypedDict, overload, override

from langgraph.checkpoint.base import CheckpointMetadata
from pydantic import BaseModel, ConfigDict, Field, TypeAdapter, ValidationError

from .values import DependencyVersions  # noqa: TC001 - Pydantic 在运行时解析字段类型.

type MetadataValue = str | int | None | dict[str, str] | dict[str, tuple[int, int]]
type MetadataCandidate = Mapping[str, MetadataValue]


class CheckpointMetadataValues(TypedDict):
    """定义检查点 metadata 的最小兼容字段."""

    scyg_runtime_kind: str
    scyg_runtime_version: str
    scyg_dependency_fingerprint: str


class CompatibleCheckpointMetadata(CheckpointMetadataValues, CheckpointMetadata):
    """组合官方字段与 SCYG 恢复兼容字段."""


class _CheckpointMetadataBoundary(BaseModel):
    """仅接受官方字段与三个 SCYG 兼容字段."""

    model_config: ClassVar[ConfigDict] = ConfigDict(
        frozen=True, extra="forbid", hide_input_in_errors=True, strict=True
    )
    source: Literal["input", "loop", "update", "fork"] | None = None
    step: int | None = None
    parents: dict[str, str] | None = None
    run_id: str | None = None
    counters_since_delta_snapshot: dict[str, tuple[int, int]] | None = None
    scyg_runtime_kind: str
    scyg_runtime_version: str
    scyg_dependency_fingerprint: str


class CheckpointCompatibility(BaseModel):
    """绑定恢复允许使用的运行时与依赖版本."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True)
    runtime_kind: Annotated[str, Field(pattern=r"^[a-z][a-z0-9_-]{0,31}$")]
    runtime_version: Annotated[str, Field(pattern=r"^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$")]
    dependencies: DependencyVersions


class IncompatibleCheckpoint(Exception):  # noqa: N818
    """表示检查点 metadata 不能由当前运行时恢复."""

    @override
    def __str__(self) -> str:
        """返回稳定且无值诊断."""
        return "LangGraph 检查点运行时不兼容"


@overload
def parse_checkpoint_metadata(value: CheckpointMetadata) -> CheckpointMetadataValues: ...


@overload
def parse_checkpoint_metadata(value: MetadataCandidate) -> CheckpointMetadataValues: ...


def parse_checkpoint_metadata(
    value: CheckpointMetadata | MetadataCandidate,
) -> CheckpointMetadataValues:
    """解析数据库 metadata 并翻译所有结构错误."""
    try:
        parsed = TypeAdapter(_CheckpointMetadataBoundary).validate_python(value)
    except ValidationError:
        raise IncompatibleCheckpoint from None
    return CheckpointMetadataValues(
        scyg_runtime_kind=parsed.scyg_runtime_kind,
        scyg_runtime_version=parsed.scyg_runtime_version,
        scyg_dependency_fingerprint=parsed.scyg_dependency_fingerprint,
    )


def checkpoint_metadata(value: CheckpointCompatibility) -> CheckpointMetadataValues:
    """生成不携带业务数据或秘密的 metadata."""
    return CheckpointMetadataValues(
        scyg_runtime_kind=value.runtime_kind,
        scyg_runtime_version=value.runtime_version,
        scyg_dependency_fingerprint=value.dependencies.fingerprint(),
    )


def ensure_checkpoint_compatible(
    metadata: CheckpointMetadataValues, expected: CheckpointCompatibility
) -> None:
    """恢复状态前验证运行时和依赖指纹."""
    if metadata != checkpoint_metadata(expected):
        raise IncompatibleCheckpoint
