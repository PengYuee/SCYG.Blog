"""OpenAI 兼容 SSE 数据帧的严格状态解析."""

from dataclasses import dataclass, replace
from json import JSONDecodeError
from typing import Final

from pydantic import ValidationError

from .models import ChunkPayload
from .results import CompletionFinished, FailureKind, ProviderDelta, ProviderFailure, ProviderResult

DATA_PREFIX: Final = "data:"
DONE_SENTINEL: Final = "[DONE]"


@dataclass(slots=True)  # noqa: RUF100  # noqa: MUTABLE_OK
class StreamParser:
    """维护单次流的提交与终止状态, 状态机本身需要可变."""

    # emitted_content 记录是否已产生领域可见内容。
    emitted_content: bool = False
    # pending_finish 延迟到 DONE 后提交, 避免重复终态。
    pending_finish: CompletionFinished | None = None
    # completed 记录是否已消费 DONE。
    completed: bool = False

    def parse_line(self, line: str) -> ProviderResult | None:  # noqa: PLR0911
        """解析一行 SSE 并返回零或一个类型化结果."""
        if not line or line.startswith(":"):
            return None
        if self.completed:
            return self._trailing_failure(line)
        if not line.startswith(DATA_PREFIX):
            return None
        data = line.removeprefix(DATA_PREFIX).strip()
        if data == DONE_SENTINEL:
            self.completed = True
            return None
        try:
            chunk = ChunkPayload.model_validate_json(data)
        except (ValidationError, JSONDecodeError):
            return ProviderFailure(FailureKind.MALFORMED)
        if not chunk.choices:
            return self._parse_usage_only(chunk)
        choice = chunk.choices[0]
        if choice.finish_reason is not None:
            if self.pending_finish is not None:
                return ProviderFailure(FailureKind.DUPLICATE_TERMINAL)
            usage = chunk.usage
            self.pending_finish = CompletionFinished(
                choice.finish_reason,
                prompt_tokens=usage.prompt_tokens if usage is not None else None,
                completion_tokens=usage.completion_tokens if usage is not None else None,
                total_tokens=usage.total_tokens if usage is not None else None,
            )
        content = choice.delta.content
        if content:
            self.emitted_content = True
            return ProviderDelta(content)
        return None

    def _parse_usage_only(self, chunk: ChunkPayload) -> ProviderFailure | None:
        """仅在已有 finish 后接受携带完整 usage 的空 choices 帧."""
        usage = chunk.usage
        if usage is None or self.pending_finish is None:
            return ProviderFailure(FailureKind.MALFORMED)
        self.pending_finish = replace(
            self.pending_finish,
            prompt_tokens=usage.prompt_tokens,
            completion_tokens=usage.completion_tokens,
            total_tokens=usage.total_tokens,
        )
        return None

    @staticmethod
    def _trailing_failure(line: str) -> ProviderFailure:
        """拒绝首个 DONE 后的所有不可忽略协议数据."""
        data = line.removeprefix(DATA_PREFIX).strip() if line.startswith(DATA_PREFIX) else line
        kind = FailureKind.DUPLICATE_TERMINAL if data == DONE_SENTINEL else FailureKind.MALFORMED
        return ProviderFailure(kind)

    def end_of_stream(self) -> CompletionFinished | ProviderFailure:
        """将物理 EOF 收敛为唯一终态."""
        if self.completed and self.pending_finish is not None:
            return self.pending_finish
        if self.emitted_content:
            return ProviderFailure(FailureKind.TRUNCATED)
        return ProviderFailure(FailureKind.EMPTY)
