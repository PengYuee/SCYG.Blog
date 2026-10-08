"""Worker 的不可变容量与时间配置."""

from dataclasses import dataclass
from datetime import timedelta
from typing import Final, override

DEFAULT_CAPACITY: Final = 4
MAX_CAPACITY: Final = 64
DEFAULT_STREAM_FLUSH_CHARS: Final = 4_096
MAX_STREAM_FLUSH_CHARS: Final = 32_000
DEFAULT_STREAM_FLUSH_INTERVAL: Final = timedelta(milliseconds=100)
MAX_STREAM_FLUSH_INTERVAL: Final = timedelta(minutes=1)


@dataclass(frozen=True, slots=True)
class InvalidWorkerConfigError(ValueError):
    """报告不安全或无界的 Worker 配置."""

    field: str

    @override
    def __str__(self) -> str:
        return f"Worker 配置无效: {self.field}"


@dataclass(frozen=True, slots=True)
class WorkerConfig:
    """保存统一 Recipe 执行容量及有界生命周期时长."""

    capacity: int = DEFAULT_CAPACITY
    lease_duration: timedelta = timedelta(seconds=30)
    renewal_fraction: float = 0.4
    poll_interval: timedelta = timedelta(milliseconds=100)
    error_backoff: timedelta = timedelta(seconds=1)
    drain_timeout: timedelta = timedelta(seconds=10)
    stream_flush_chars: int = DEFAULT_STREAM_FLUSH_CHARS
    stream_flush_interval: timedelta = DEFAULT_STREAM_FLUSH_INTERVAL

    def __post_init__(self) -> None:
        """拒绝 bool、非正数、无界容量和不安全续租周期."""
        if type(self.capacity) is not int or not 1 <= self.capacity <= MAX_CAPACITY:
            field = "capacity"
            raise InvalidWorkerConfigError(field)
        if type(self.renewal_fraction) is not float or not 0.0 < self.renewal_fraction < 1.0:
            field = "renewal_fraction"
            raise InvalidWorkerConfigError(field)
        for field, value in (
            ("lease_duration", self.lease_duration),
            ("poll_interval", self.poll_interval),
            ("error_backoff", self.error_backoff),
            ("drain_timeout", self.drain_timeout),
        ):
            if type(value) is not timedelta or value <= timedelta(0):
                raise InvalidWorkerConfigError(field)
        if (
            type(self.stream_flush_chars) is not int
            or self.stream_flush_chars < 1
            or self.stream_flush_chars > MAX_STREAM_FLUSH_CHARS
        ):
            field = "stream_flush_chars"
            raise InvalidWorkerConfigError(field)
        if (
            type(self.stream_flush_interval) is not timedelta
            or self.stream_flush_interval <= timedelta(0)
            or self.stream_flush_interval > MAX_STREAM_FLUSH_INTERVAL
        ):
            field = "stream_flush_interval"
            raise InvalidWorkerConfigError(field)
