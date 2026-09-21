"""OpenAI 兼容异步流式 HTTP 提供方。."""

from collections.abc import AsyncIterator
from dataclasses import dataclass
from http import HTTPStatus
from typing import Final, override

import httpx
from pydantic import SecretStr

from .models import CompletionRequest
from .parser import StreamParser
from .results import FailureKind, ProviderFailure, ProviderResult

CHAT_COMPLETIONS_PATH: Final = "/chat/completions"
AUTHORIZATION_HEADER: Final = "Authorization"
CONTENT_TYPE_HEADER: Final = "Content-Type"
JSON_CONTENT_TYPE: Final = "application/json"
RETRYABLE_STATUS: Final = frozenset(
    {
        HTTPStatus.REQUEST_TIMEOUT,
        HTTPStatus.TOO_MANY_REQUESTS,
        HTTPStatus.BAD_GATEWAY,
        HTTPStatus.SERVICE_UNAVAILABLE,
        HTTPStatus.GATEWAY_TIMEOUT,
    }
)
MAX_RETRIES: Final = 3
MAX_TIMEOUT_SECONDS: Final = 3600


@dataclass(frozen=True, slots=True)
class ProviderConfig:
    """冻结提供方凭据与有界调用策略。."""

    api_key: SecretStr
    retries: int = 2
    timeout_seconds: int = 120

    def __post_init__(self) -> None:
        """拒绝无界重试或超时。."""
        if (
            not 0 <= self.retries <= MAX_RETRIES
            or not 1 <= self.timeout_seconds <= MAX_TIMEOUT_SECONDS
        ):
            raise InvalidProviderConfigError


class InvalidProviderConfigError(ValueError):
    """报告不包含配置值的提供方策略错误。."""

    @override
    def __str__(self) -> str:
        """返回稳定中文错误。."""
        return "SIMPLE 提供方配置无效"


class OpenAICompatibleProvider:
    """通过注入的异步 HTTP 客户端执行可测试的真实协议调用。."""

    __slots__: tuple[str, ...] = ("_client", "_closed", "_config", "_owns_client")

    _client: httpx.AsyncClient
    _config: ProviderConfig
    _owns_client: bool
    _closed: bool

    def __init__(
        self,
        client: httpx.AsyncClient,
        config: ProviderConfig,
        *,
        owns_client: bool = True,
    ) -> None:
        """保存客户端,并明确它是拥有资源还是借用资源。."""
        self._client = client
        self._config = config
        self._owns_client = owns_client
        self._closed = False

    async def aclose(self) -> None:
        """仅一次关闭提供方拥有的 HTTP 客户端资源。."""
        if self._closed:
            return
        self._closed = True
        if self._owns_client:
            await self._client.aclose()

    async def stream(  # noqa: C901
        self,
        request: CompletionRequest,
    ) -> AsyncIterator[ProviderResult]:
        """流式产生增量和唯一终态,提交后绝不重试。."""
        attempts = self._config.retries + 1
        for attempt in range(attempts):
            parser = StreamParser()
            try:
                async with self._client.stream(
                    "POST",
                    CHAT_COMPLETIONS_PATH,
                    headers={
                        AUTHORIZATION_HEADER: f"Bearer {self._config.api_key.get_secret_value()}",
                        CONTENT_TYPE_HEADER: JSON_CONTENT_TYPE,
                    },
                    content=request.model_dump_json(),
                    timeout=self._config.timeout_seconds,
                ) as response:
                    if response.status_code >= HTTPStatus.BAD_REQUEST:
                        failure = self._http_failure(response.status_code, attempt, attempts)
                        if failure is None:
                            continue
                        yield failure
                        return
                    async for line in response.aiter_lines():
                        result = parser.parse_line(line)
                        if result is None:
                            continue
                        yield result
                        if type(result) is ProviderFailure:
                            return
                    yield parser.end_of_stream()
                    return
            except (httpx.ConnectError, httpx.ConnectTimeout):
                if attempt + 1 < attempts:
                    continue
                yield ProviderFailure(FailureKind.CONNECT_RETRY_EXHAUSTED)
                return
            except (httpx.ReadTimeout, httpx.WriteTimeout, httpx.PoolTimeout):
                yield ProviderFailure(FailureKind.TIMEOUT)
                return
            except httpx.RemoteProtocolError:
                yield ProviderFailure(
                    FailureKind.TRUNCATED if parser.emitted_content else FailureKind.MALFORMED,
                )
                return

    @staticmethod
    def _http_failure(status_code: int, attempt: int, attempts: int) -> ProviderFailure | None:
        """将状态码映射为重试决定或无秘密终态。."""
        if status_code in RETRYABLE_STATUS and attempt + 1 < attempts:
            return None
        kind = (
            FailureKind.HTTP_RETRY_EXHAUSTED
            if status_code in RETRYABLE_STATUS
            else FailureKind.HTTP_CLIENT
        )
        return ProviderFailure(kind, status_code=status_code)
