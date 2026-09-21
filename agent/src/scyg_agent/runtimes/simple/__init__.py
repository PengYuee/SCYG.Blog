"""SIMPLE 运行时与 OpenAI 兼容提供方公共边界."""

from .factory import (
    SimpleHttpClientConfig,
    create_simple_http_client,
    open_simple_provider,
)
from .models import CompletionRequest, Message, MessageRole
from .provider import OpenAICompatibleProvider, ProviderConfig
from .results import CompletionFinished, FailureKind, ProviderDelta, ProviderFailure

__all__ = [
    "CompletionFinished",
    "CompletionRequest",
    "FailureKind",
    "Message",
    "MessageRole",
    "OpenAICompatibleProvider",
    "ProviderConfig",
    "ProviderDelta",
    "ProviderFailure",
    "SimpleHttpClientConfig",
    "create_simple_http_client",
    "open_simple_provider",
]
