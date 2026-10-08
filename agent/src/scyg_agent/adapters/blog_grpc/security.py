"""Immutable, bounded deadline configuration for BlogContent RPCs."""

from dataclasses import dataclass
from math import isfinite
from typing import Final, override

MIN_DEADLINE_SECONDS: Final = 0.001
MAX_DEADLINE_SECONDS: Final = 3600.0


@dataclass(frozen=True, slots=True)
class InvalidDeadlineError(ValueError):
    """报告非正、非有限或无界截止期。."""

    @override
    def __str__(self) -> str:
        """返回不回显配置值的中文错误。."""
        return "Blog gRPC 截止期必须为有限正数且不超过 3600 秒"


@dataclass(frozen=True, slots=True)
class RpcDeadline:
    """保存经过精确类型和有限上界解析的 RPC 截止期。."""

    seconds: float

    def __post_init__(self) -> None:
        """拒绝绕过解析工厂直接构造的无效值。."""
        if (
            type(self.seconds) is not float
            or not isfinite(self.seconds)
            or not MIN_DEADLINE_SECONDS <= self.seconds <= MAX_DEADLINE_SECONDS
        ):
            raise InvalidDeadlineError

    @classmethod
    def parse(cls, value: float) -> "RpcDeadline":
        """在创建通道前解析严格浮点截止期。."""
        return cls(value)
