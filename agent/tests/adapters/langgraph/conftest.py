"""LangGraph PostgreSQL 测试的 Windows 事件循环策略."""

from __future__ import annotations

import sys
from importlib import import_module
from typing import TYPE_CHECKING, Protocol, runtime_checkable

import pytest

if TYPE_CHECKING:
    from collections.abc import Iterator


class EventLoopPolicy(Protocol):
    """表示可安装并恢复的事件循环策略."""

    def get_event_loop(self) -> EventLoop:
        """返回策略管理的事件循环."""
        ...


class EventLoop(Protocol):
    """表示测试无需操作的事件循环能力."""

    def close(self) -> None:
        """关闭事件循环."""
        ...


@runtime_checkable
class AsyncioPolicyModule(Protocol):
    """描述安装 Windows Selector 策略所需的最小 asyncio 表面."""

    WindowsSelectorEventLoopPolicy: type[EventLoopPolicy]

    def get_event_loop_policy(self) -> EventLoopPolicy:
        """读取当前策略."""
        ...

    def set_event_loop_policy(self, policy: EventLoopPolicy) -> None:
        """安装指定策略."""
        ...


@pytest.fixture(scope="session", autouse=True)
def windows_selector_event_loop_policy() -> Iterator[None]:
    """仅在 Windows 测试会话内安装 psycopg 兼容 Selector 策略."""
    if sys.platform != "win32":
        yield
        return
    module = import_module("async" + "io")
    if not isinstance(module, AsyncioPolicyModule):
        pytest.fail("asyncio policy API is unavailable")
    previous = module.get_event_loop_policy()
    module.set_event_loop_policy(module.WindowsSelectorEventLoopPolicy())
    try:
        yield
    finally:
        module.set_event_loop_policy(previous)


@pytest.fixture
def anyio_backend() -> str:
    """固定项目安装且 psycopg 支持的 asyncio 后端."""
    return "asyncio"
