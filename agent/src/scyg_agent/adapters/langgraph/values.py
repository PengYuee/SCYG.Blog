"""LangGraph 检查点的类型化配置与 schema 兼容规则."""

from __future__ import annotations

from dataclasses import dataclass
from dataclasses import field as dataclass_field
from importlib.metadata import version
from typing import Annotated, ClassVar, Final, Literal, override

from pydantic import (
    BaseModel,
    ConfigDict,
    Field,
    PostgresDsn,
    SecretStr,
    TypeAdapter,
    ValidationError,
)

from scyg_agent.domain.runs import RunId

SCHEMA: Final = "langgraph"
EXPECTED_TABLES: Final = frozenset(
    {
        "checkpoint_migrations",
        "checkpoints",
        "checkpoint_blobs",
        "checkpoint_writes",
        "scyg_checkpoint_metadata",
    }
)
EXPECTED_INDEXES: Final = frozenset(
    {
        "checkpoints_thread_id_idx",
        "checkpoint_blobs_thread_id_idx",
        "checkpoint_writes_thread_id_idx",
    }
)
EXPECTED_MIGRATIONS: Final = tuple(range(10))
METADATA_KEY: Final = "scyg-runtime-contract"
PositiveFloat = Annotated[float, Field(gt=0, le=300)]
PoolSize = Annotated[int, Field(ge=1, le=64)]


class DependencyVersions(BaseModel):
    """记录检查点兼容性依赖的精确包版本."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True)
    langgraph: str
    checkpoint: str
    checkpoint_postgres: str
    psycopg: str
    psycopg_pool: str

    @classmethod
    def installed(cls) -> DependencyVersions:
        """读取当前进程实际安装的依赖版本."""
        return cls(
            langgraph=version("langgraph"),
            checkpoint=version("langgraph-checkpoint"),
            checkpoint_postgres=version("langgraph-checkpoint-postgres"),
            psycopg=version("psycopg"),
            psycopg_pool=version("psycopg-pool"),
        )

    def fingerprint(self) -> str:
        """生成稳定且不含凭据的依赖指纹."""
        return "|".join(
            (
                f"langgraph={self.langgraph}",
                f"checkpoint={self.checkpoint}",
                f"checkpoint_postgres={self.checkpoint_postgres}",
                f"psycopg={self.psycopg}",
                f"psycopg_pool={self.psycopg_pool}",
            )
        )


class InvalidCheckpointerConfigError(ValueError):
    """表示检查点配置与当前进程契约不一致."""

    @override
    def __str__(self) -> str:
        """返回无输入值的稳定诊断."""
        return "LangGraph 检查点配置无效"


class _CheckpointerConfigInput(BaseModel):
    """在私有边界解析检查点配置的字段约束."""

    model_config: ClassVar[ConfigDict] = ConfigDict(
        frozen=True, extra="forbid", hide_input_in_errors=True, strict=True
    )
    dsn: SecretStr
    schema_name: Literal["langgraph"]
    min_pool_size: PoolSize
    max_pool_size: PoolSize
    pool_timeout_seconds: PositiveFloat
    operation_timeout_seconds: PositiveFloat
    close_timeout_seconds: PositiveFloat
    versions: DependencyVersions


@dataclass(frozen=True, slots=True)
class CheckpointerConfig:
    """保存经过私有解析边界验证的专用池配置."""

    dsn: SecretStr
    schema_name: str = SCHEMA
    min_pool_size: int = 1
    max_pool_size: int = 4
    pool_timeout_seconds: float = 10
    operation_timeout_seconds: float = 15
    close_timeout_seconds: float = 10
    versions: DependencyVersions = dataclass_field(default_factory=DependencyVersions.installed)

    def __post_init__(self) -> None:
        """解析构造参数并统一翻译所有配置错误."""
        if self.schema_name != SCHEMA:
            raise InvalidCheckpointerConfigError

        try:
            parsed = _CheckpointerConfigInput(
                dsn=self.dsn,
                schema_name=SCHEMA,
                min_pool_size=self.min_pool_size,
                max_pool_size=self.max_pool_size,
                pool_timeout_seconds=self.pool_timeout_seconds,
                operation_timeout_seconds=self.operation_timeout_seconds,
                close_timeout_seconds=self.close_timeout_seconds,
                versions=self.versions,
            )
            _ = TypeAdapter(PostgresDsn).validate_python(parsed.dsn.get_secret_value())
        except (ValidationError, ValueError):
            parsed = None

        if (
            parsed is None
            or parsed.min_pool_size > parsed.max_pool_size
            or parsed.versions != DependencyVersions.installed()
        ):
            raise InvalidCheckpointerConfigError

        object.__setattr__(self, "schema_name", parsed.schema_name)
        object.__setattr__(self, "pool_timeout_seconds", parsed.pool_timeout_seconds)
        object.__setattr__(self, "operation_timeout_seconds", parsed.operation_timeout_seconds)
        object.__setattr__(self, "close_timeout_seconds", parsed.close_timeout_seconds)


@dataclass(frozen=True, slots=True)
class CheckpointThreadId:
    """保存从 RunId 去除固定前缀后的可逆线程标识."""

    value: str

    def to_run_id(self) -> RunId:
        """无损恢复原始 RunId."""
        return RunId(f"run_{self.value}")

    @override
    def __str__(self) -> str:
        """返回验证后的短标识."""
        return self.value


def thread_id_for_run(run_id: RunId) -> CheckpointThreadId:
    """将 RunId 一对一映射为稳定短线程标识."""
    return CheckpointThreadId(str(run_id).removeprefix("run_"))


class SchemaInventory(BaseModel):
    """承载 readiness 的对象与版本清单."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True)
    schema_name: str
    tables: frozenset[str]
    indexes: frozenset[str]
    migration_versions: tuple[int, ...]
    metadata_versions: DependencyVersions


class IncompatibleCheckpointSchema(Exception):  # noqa: N818
    """表示 schema 缺失, 损坏或版本不兼容."""

    @override
    def __str__(self) -> str:
        """隐藏底层 SQL 和连接信息."""
        return "LangGraph 检查点 schema 不兼容"


def ensure_schema_compatible(inventory: SchemaInventory, versions: DependencyVersions) -> None:
    """验证对象, 迁移和依赖版本."""
    compatible = (
        inventory.schema_name == SCHEMA
        and inventory.tables == EXPECTED_TABLES
        and inventory.indexes >= EXPECTED_INDEXES
        and inventory.migration_versions == EXPECTED_MIGRATIONS
        and inventory.metadata_versions == versions
    )
    if not compatible:
        raise IncompatibleCheckpointSchema
