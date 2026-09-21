"""Worker 配置与生命周期边界测试。"""

from dataclasses import replace
from datetime import timedelta

import pytest

from scyg_agent.worker import (
    InvalidWorkerConfigError,
    InvalidWorkerTransitionError,
    WorkerConfig,
    WorkerState,
)


@pytest.mark.parametrize("value", [True, False, 0, -1, 65])
def test_capacity_rejects_non_positive_boolean_and_unbounded_values(value: int) -> None:
    """Given 非法容量, When 构造配置, Then 在边界拒绝."""
    with pytest.raises(InvalidWorkerConfigError):
        _ = WorkerConfig(simple_capacity=value)


def test_default_capacities_are_independent_and_bounded() -> None:
    """Given 默认配置, When 读取容量, Then SIMPLE 与 DEEP 分别为 4 和 1."""
    config = WorkerConfig()

    assert config.simple_capacity == 4
    assert config.deep_capacity == 1


@pytest.mark.parametrize(
    "renewal_fraction",
    [True, 0.0, 1.0, -0.1],
)
def test_renewal_fraction_must_be_strictly_between_zero_and_one(
    renewal_fraction: float,
) -> None:
    """Given 非法续租比例, When 构造配置, Then 拒绝可能越过租期的值."""
    with pytest.raises(InvalidWorkerConfigError):
        _ = WorkerConfig(renewal_fraction=renewal_fraction)


def test_durations_must_be_positive_and_drain_not_exceed_bound() -> None:
    """Given 非正时长, When 构造配置, Then 立即拒绝."""
    with pytest.raises(InvalidWorkerConfigError):
        _ = WorkerConfig(poll_interval=timedelta(0))


def test_every_capacity_and_duration_field_uses_the_same_strict_boundary() -> None:
    """Given 其余容量与时长字段, When 置零, Then 全部拒绝."""
    config = WorkerConfig()
    with pytest.raises(InvalidWorkerConfigError):
        _ = replace(config, deep_capacity=0)
    with pytest.raises(InvalidWorkerConfigError):
        _ = replace(config, lease_duration=timedelta(0))
    with pytest.raises(InvalidWorkerConfigError):
        _ = replace(config, error_backoff=timedelta(0))
    with pytest.raises(InvalidWorkerConfigError):
        _ = replace(config, drain_timeout=timedelta(0))


def test_worker_errors_expose_stable_chinese_messages() -> None:
    """Given 配置和生命周期错误, When 格式化, Then 返回稳定中文消息."""
    assert str(InvalidWorkerConfigError("capacity")) == "Worker 配置无效: capacity"
    assert (
        str(InvalidWorkerTransitionError(WorkerState.NEW, "stop"))
        == "Worker 生命周期转换无效: new/stop"
    )
