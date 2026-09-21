"""OpenAI 兼容请求与响应的严格边界模型."""

from enum import StrEnum
from typing import Annotated, ClassVar

from pydantic import BaseModel, ConfigDict, Field

type NonNegativeToken = Annotated[int, Field(ge=0, strict=True)]


class MessageRole(StrEnum):
    """限制提供方消息角色闭集."""

    SYSTEM = "system"
    USER = "user"


class Message(BaseModel):
    """表示一个不可变且非空的模型消息."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid", strict=True)

    # role 是 OpenAI 兼容消息角色。
    role: MessageRole
    # content 是待发送的完整消息正文。
    content: Annotated[str, Field(min_length=1)]


class StreamOptions(BaseModel):
    """声明流式响应需要提供方返回真实 usage."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid", strict=True)

    # include_usage 固定请求终态 usage。
    include_usage: bool = True


class CompletionRequest(BaseModel):
    """表示流式聊天完成请求."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid", strict=True)

    # model 是部署配置的提供方模型名称。
    model: Annotated[str, Field(min_length=1)]
    # messages 是不可变且至少包含一项的提示消息。
    messages: Annotated[tuple[Message, ...], Field(min_length=1)]
    # stream 固定启用流式响应。
    stream: bool = True
    # stream_options 请求提供方在终态返回真实用量。
    stream_options: StreamOptions = StreamOptions()


class DeltaPayload(BaseModel):
    """解析单个 choice 的文本增量."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid", strict=True)

    # content 缺省表示该帧没有文本增量。
    content: str | None = None


class ChoicePayload(BaseModel):
    """解析 OpenAI 兼容流中的唯一 choice."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="ignore", strict=True)

    # delta 是本帧增量载荷。
    delta: DeltaPayload
    # finish_reason 是提供方终止原因。
    finish_reason: str | None = None


class UsagePayload(BaseModel):
    """解析提供方报告的真实令牌用量."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="ignore", strict=True)

    # prompt_tokens 是输入令牌数。
    prompt_tokens: NonNegativeToken
    # completion_tokens 是输出令牌数。
    completion_tokens: NonNegativeToken
    # total_tokens 是提供方报告的总令牌数。
    total_tokens: NonNegativeToken


class ChunkPayload(BaseModel):
    """解析一个 OpenAI 兼容 SSE JSON 数据帧."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="ignore", strict=True)

    # choices 允许标准 usage-only 空集合, 非空时必须恰好一个 choice。
    choices: Annotated[tuple[ChoicePayload, ...], Field(max_length=1)]
    # usage 仅在提供方真实返回时存在。
    usage: UsagePayload | None = None
