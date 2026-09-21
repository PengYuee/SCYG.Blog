"""Worker 的不可变容量与时间配置."""

from dataclasses import dataclass
from datetime import timedelta
from typing import Final, override

DEFAULT_SIMPLE_CAPACITY: Final = 4
DEFAULT_DEEP_CAPACITY: Final = 1
MAX_CAPACITY: Final = 64


@dataclass(frozen=True, slots=True)
class InvalidWorkerConfigError(ValueError):
    """报告不安全或无界的 Worker 配置."""

    field: str

    @override
    def __str__(self) -> str:
        return f"Worker 配置无效: {self.field}"


@dataclass(frozen=True, slots=True)
class WorkerConfig:
    """保存独立运行时容量及有界生命周期时长."""

    simple_capacity: int = DEFAULT_SIMPLE_CAPACITY
    deep_capacity: int = DEFAULT_DEEP_CAPACITY
    lease_duration: timedelta = timedelta(seconds=30)
    renewal_fraction: float = 0.4
    poll_interval: timedelta = timedelta(milliseconds=100)
    error_backoff: timedelta = timedelta(seconds=1)
    drain_timeout: timedelta = timedelta(seconds=10)

    def __post_init__(self) -> None:
        """拒绝 bool、非正数、无界容量和不安全续租周期."""
        for field, value in (
            ("simple_capacity", self.simple_capacity),
            ("deep_capacity", self.deep_capacity),
        ):
            if type(value) is not int or value < 1 or value > MAX_CAPACITY:
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
