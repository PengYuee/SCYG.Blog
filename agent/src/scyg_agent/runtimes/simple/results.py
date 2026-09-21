"""SIMPLE 提供方流的应用自有结果类型."""

from dataclasses import dataclass
from enum import StrEnum
from typing import override


class FailureKind(StrEnum):
    """关闭所有不含提供方原文的失败类别."""

    CONNECT_RETRY_EXHAUSTED = "连接重试已耗尽"
    HTTP_CLIENT = "提供方拒绝请求"
    HTTP_RETRY_EXHAUSTED = "提供方暂时不可用且重试已耗尽"
    TIMEOUT = "提供方请求超时"
    MALFORMED = "提供方流格式无效"
    TRUNCATED = "提供方流意外中断"
    DUPLICATE_TERMINAL = "提供方重复发送终止帧"
    EMPTY = "提供方未返回内容"


@dataclass(frozen=True, slots=True)
class ProviderDelta:
    """承载一个已提交给领域消费者的文本增量."""

    # content 是非空增量正文。
    content: str


@dataclass(frozen=True, slots=True)
class CompletionFinished:
    """承载唯一成功终态及可选真实用量."""

    # reason 是提供方终止原因。
    reason: str
    # prompt_tokens 仅在提供方报告 usage 时存在。
    prompt_tokens: int | None = None
    # completion_tokens 仅在提供方报告 usage 时存在。
    completion_tokens: int | None = None
    # total_tokens 仅在提供方报告 usage 时存在。
    total_tokens: int | None = None


@dataclass(frozen=True, slots=True)
class ProviderFailure:
    """承载稳定、类型化且不包含秘密的失败终态."""

    # kind 是允许调用方穷尽处理的失败类别。
    kind: FailureKind
    # status_code 仅暴露无秘密的 HTTP 状态码。
    status_code: int | None = None

    @override
    def __str__(self) -> str:
        """返回不包含请求、响应或凭据的中文错误."""
        return f"SIMPLE 提供方失败: {self.kind}"


type ProviderResult = ProviderDelta | CompletionFinished | ProviderFailure
