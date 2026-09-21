"""幂等存储端口共享的纯领域值。."""

import re
from dataclasses import dataclass
from datetime import datetime
from typing import ClassVar, Self, override

from scyg_agent.domain.runs.models import validate_utc_timestamp

MAX_METADATA_KEY_LENGTH = 64
MAX_METADATA_VALUE_LENGTH = 256


@dataclass(frozen=True, slots=True)
class InvalidDigestError(ValueError):
    """报告不符合规范的请求摘要且不回显输入。."""

    @override
    def __str__(self) -> str:
        """返回稳定且无敏感值的诊断。."""
        return "request digest must be lowercase SHA-256 hex"


@dataclass(frozen=True, slots=True)
class RequestDigest:
    """保存规范化的小写 SHA-256 十六进制摘要。."""

    _pattern: ClassVar[re.Pattern[str]] = re.compile(r"[0-9a-f]{64}")
    value: str

    @classmethod
    def parse(cls, raw: str) -> Self:
        """在边界将不可信文本解析为摘要。."""
        if cls._pattern.fullmatch(raw) is None:
            raise InvalidDigestError
        return cls(raw)

    @override
    def __str__(self) -> str:
        """返回已验证的摘要。."""
        return self.value


@dataclass(frozen=True, slots=True)
class ResultReference:
    """引用外部已清洗结果而不保存结果正文。."""

    value: str

    def __post_init__(self) -> None:
        """限制引用为短且非空的安全文本。."""
        if (
            not self.value
            or len(self.value) > MAX_METADATA_VALUE_LENGTH
            or any(char.isspace() for char in self.value)
        ):
            msg = "reference"
            raise InvalidResultValueError(msg)


@dataclass(frozen=True, slots=True)
class ResultMetadata:
    """保存一个允许列表式的标量结果属性。."""

    key: str
    value: str

    def __post_init__(self) -> None:
        """禁止空键值和可能承载正文的超长值。."""
        if (
            not self.key
            or len(self.key) > MAX_METADATA_KEY_LENGTH
            or len(self.value) > MAX_METADATA_VALUE_LENGTH
        ):
            msg = "metadata"
            raise InvalidResultValueError(msg)


@dataclass(frozen=True, slots=True)
class AuditMetadata:
    """保存一个清洗后的审计关联属性。."""

    key: str
    value: str

    def __post_init__(self) -> None:
        """限制审计属性为小型标量。."""
        if (
            not self.key
            or len(self.key) > MAX_METADATA_KEY_LENGTH
            or len(self.value) > MAX_METADATA_VALUE_LENGTH
        ):
            msg = "audit metadata"
            raise InvalidResultValueError(msg)


@dataclass(frozen=True, slots=True)
class InvalidResultValueError(ValueError):
    """报告清洗结果字段不满足持久化边界。."""

    field: str

    @override
    def __str__(self) -> str:
        """返回不含输入值的稳定诊断。."""
        return f"invalid sanitized {self.field}"


def require_utc(value: datetime, field: str) -> None:
    """复用 Run 的 UTC 时间边界校验。."""
    validate_utc_timestamp(value, field)
