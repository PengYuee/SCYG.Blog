"""持久化领域事件载荷的严格版本化模型."""

from typing import Annotated, ClassVar, Final, Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator

from scyg_agent.domain.runs import EventKind

SCHEMA_VERSION = "1"
MAX_TOKEN_USAGE: Final = 2_147_483_647
CounterText = Annotated[str, Field(pattern=r"^(0|[1-9][0-9]{0,9})$")]


class PayloadBase(BaseModel):
    """定义所有事件载荷共享的严格信封."""

    model_config: ClassVar[ConfigDict] = ConfigDict(
        extra="forbid", frozen=True, strict=True, hide_input_in_errors=True
    )
    schema_version: Literal["1"] = SCHEMA_VERSION
    command_id: str


class EmptyPayload(PayloadBase):
    """承载无附加字段的生命周期事实."""


class StatusPayload(PayloadBase):
    """承载状态转换字段."""

    previous: str
    current: str


class InteractionPayload(PayloadBase):
    """承载交互身份字段."""

    interaction_id: str


class TextPayload(PayloadBase):
    """承载非空文本增量."""

    content: Annotated[str, Field(min_length=1)]


class UsagePayload(PayloadBase):
    """承载严格一致的十进制令牌计数."""

    prompt_tokens: CounterText
    completion_tokens: CounterText
    total_tokens: CounterText

    @model_validator(mode="after")
    def validate_total(self) -> "UsagePayload":
        """拒绝溢出和内部不一致的用量."""
        prompt = int(self.prompt_tokens)
        completion = int(self.completion_tokens)
        total = int(self.total_tokens)
        if total > MAX_TOKEN_USAGE or prompt + completion != total:
            message = "invalid usage counters"
            raise ValueError(message)
        return self


class ApprovalPayload(InteractionPayload):
    """承载审批决定."""

    approved: Literal["true", "false"]


class ToolPayload(PayloadBase):
    """承载逻辑工具调用身份."""

    tool_call_id: str


type PersistedPayload = (
    EmptyPayload
    | StatusPayload
    | InteractionPayload
    | TextPayload
    | UsagePayload
    | ApprovalPayload
    | ToolPayload
)


def payload_model(kind: EventKind) -> type[PersistedPayload]:
    """按事件种类穷尽选择唯一版本模型."""
    models: dict[EventKind, type[PersistedPayload]] = {
        EventKind.STATUS_CHANGED: StatusPayload,
        EventKind.INPUT_REQUESTED: InteractionPayload,
        EventKind.INPUT_RESOLVED: InteractionPayload,
        EventKind.APPROVAL_REQUIRED: InteractionPayload,
        EventKind.TEXT_DELTA: TextPayload,
        EventKind.TOKEN_USAGE: UsagePayload,
        EventKind.APPROVAL_RESOLVED: ApprovalPayload,
        EventKind.TOOL_STARTED: ToolPayload,
        EventKind.TOOL_SUCCEEDED: ToolPayload,
        EventKind.TOOL_FAILED: ToolPayload,
        EventKind.TOOL_OUTCOME_UNKNOWN: ToolPayload,
        EventKind.RUN_SUCCEEDED: EmptyPayload,
        EventKind.RUN_FAILED: EmptyPayload,
        EventKind.RUN_CANCELLED: EmptyPayload,
        EventKind.EXECUTION_RELEASED: EmptyPayload,
    }
    return models[kind]


def parse_payload(kind: EventKind, raw: dict[str, str]) -> PersistedPayload:
    """解析严格载荷; 缺失版本按既有 v1 行兼容处理."""
    candidate = raw if "schema_version" in raw else {**raw, "schema_version": SCHEMA_VERSION}
    return payload_model(kind).model_validate(candidate)


__all__ = [
    "ApprovalPayload",
    "EmptyPayload",
    "InteractionPayload",
    "PersistedPayload",
    "StatusPayload",
    "TextPayload",
    "ToolPayload",
    "UsagePayload",
    "ValidationError",
    "parse_payload",
]
