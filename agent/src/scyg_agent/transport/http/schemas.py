"""HTTP 适配器的严格请求与响应模型。."""

from datetime import datetime
from typing import Annotated, ClassVar, Literal

from pydantic import BaseModel, ConfigDict, Field

Identifier = Annotated[str, Field(min_length=1, max_length=128)]
DigestText = Annotated[str, Field(pattern=r"^[0-9a-f]{64}$")]


class StrictModel(BaseModel):
    """统一关闭额外字段并隐藏不可信输入。."""

    model_config: ClassVar[ConfigDict] = ConfigDict(
        extra="forbid", frozen=True, strict=True, hide_input_in_errors=True
    )


class InputRequest(StrictModel):
    """解析一次等待中的用户输入。."""

    interaction_id: Identifier
    command_id: Identifier
    event_id: Identifier
    expected_revision: Annotated[int, Field(ge=1)]
    expected_sequence: Annotated[int, Field(ge=0)]
    response_digest: DigestText
    result_reference: Identifier
    occurred_at: Annotated[datetime, Field(strict=False)]


class CommandRequest(StrictModel):
    """提交公共取消命令而不复制状态机。."""

    kind: Literal["cancel"]
    command_id: Identifier
    event_id: Identifier
    expected_revision: Annotated[int, Field(ge=1)]
    expected_sequence: Annotated[int, Field(ge=0)]
    request_digest: DigestText
    occurred_at: Annotated[datetime, Field(strict=False)]


class SnapshotResponse(StrictModel):
    """返回不含租约字段的稳定 Run 快照。."""

    run_id: str
    owner_user_id: str
    task_type: str
    runtime_kind: str
    runtime_version: str
    revision: int
    status: str
    created_at: datetime
    updated_at: datetime
    attempt: int
    pending_interaction_id: str | None
    cursor: int


class MutationResponse(StrictModel):
    """返回 mutation 的稳定重放事实。."""

    status: Literal["accepted"] = "accepted"
    replayed: bool


class ErrorResponse(StrictModel):
    """仅返回中文安全错误。."""

    detail: str
