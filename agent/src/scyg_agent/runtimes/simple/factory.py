"""SIMPLE HTTP 客户端与提供方的显式所有权工厂。."""

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from dataclasses import dataclass
from typing import Final, override

import httpx

from scyg_agent.config import ApplicationSettings

from .provider import OpenAICompatibleProvider, ProviderConfig

DEFAULT_MAX_CONNECTIONS: Final = 200
DEFAULT_MAX_KEEPALIVE_CONNECTIONS: Final = 40
DEFAULT_KEEPALIVE_EXPIRY_SECONDS: Final = 30.0
DEFAULT_CONNECT_TIMEOUT_SECONDS: Final = 5.0
DEFAULT_WRITE_TIMEOUT_SECONDS: Final = 10.0
DEFAULT_POOL_TIMEOUT_SECONDS: Final = 10.0


@dataclass(frozen=True, slots=True)
class SimpleHttpClientConfig:
    """保存生产 HTTP 连接池和分阶段超时。."""

    max_connections: int = DEFAULT_MAX_CONNECTIONS
    max_keepalive_connections: int = DEFAULT_MAX_KEEPALIVE_CONNECTIONS
    keepalive_expiry_seconds: float = DEFAULT_KEEPALIVE_EXPIRY_SECONDS
    connect_timeout_seconds: float = DEFAULT_CONNECT_TIMEOUT_SECONDS
    write_timeout_seconds: float = DEFAULT_WRITE_TIMEOUT_SECONDS
    pool_timeout_seconds: float = DEFAULT_POOL_TIMEOUT_SECONDS

    def __post_init__(self) -> None:
        """拒绝无界、空或倒置的连接参数。."""
        values = (
            self.max_connections,
            self.max_keepalive_connections,
            self.keepalive_expiry_seconds,
            self.connect_timeout_seconds,
            self.write_timeout_seconds,
            self.pool_timeout_seconds,
        )
        if any(type(value) not in {int, float} or value <= 0 for value in values):
            raise InvalidSimpleHttpClientConfigError
        if self.max_keepalive_connections > self.max_connections:
            raise InvalidSimpleHttpClientConfigError


class InvalidSimpleHttpClientConfigError(ValueError):
    """报告不包含配置值的客户端参数错误。."""

    @override
    def __str__(self) -> str:
        """返回稳定中文诊断。."""
        return "SIMPLE HTTP 客户端配置无效"


def create_simple_http_client(
    settings: ApplicationSettings,
    config: SimpleHttpClientConfig | None = None,
) -> httpx.AsyncClient:
    """创建由调用方显式拥有的共享生产 HTTP 客户端。."""
    resolved = SimpleHttpClientConfig() if config is None else config
    limits = httpx.Limits(
        max_connections=resolved.max_connections,
        max_keepalive_connections=resolved.max_keepalive_connections,
        keepalive_expiry=resolved.keepalive_expiry_seconds,
    )
    timeout = httpx.Timeout(
        connect=resolved.connect_timeout_seconds,
        read=settings.shutdown_seconds,
        write=resolved.write_timeout_seconds,
        pool=resolved.pool_timeout_seconds,
    )
    return httpx.AsyncClient(
        base_url=str(settings.provider_base_url),
        limits=limits,
        timeout=timeout,
        follow_redirects=True,
    )


@asynccontextmanager
async def open_simple_provider(
    settings: ApplicationSettings,
    *,
    client: httpx.AsyncClient | None = None,
) -> AsyncIterator[OpenAICompatibleProvider]:
    """内部客户端归工厂所有,外部注入客户端仅借用。."""
    if client is None:
        owned = True
        shared = create_simple_http_client(settings)
    else:
        owned = False
        shared = client
    provider = OpenAICompatibleProvider(
        shared,
        ProviderConfig(settings.provider_api_key, timeout_seconds=settings.shutdown_seconds),
        owns_client=False,
    )
    try:
        yield provider
    finally:
        if owned:
            await shared.aclose()
