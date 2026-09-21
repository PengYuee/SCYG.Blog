"""幂等请求的规范语义身份。."""

from hashlib import sha256

from scyg_agent.domain.runs import (
    CancelRun,
    ClaimRun,
    FailRun,
    ReleaseExecution,
    RequestInput,
    RunCommand,
    SubmitInput,
    SucceedRun,
)

from .command_store import CommandSubmission
from .idempotency import RequestDigest
from .tool_store import ToolIntent, ToolOperation


def command_semantic_digest(request: CommandSubmission) -> RequestDigest:
    """绑定命令的全部不可变语义字段。."""
    command = request.command
    common = (
        str(request.command_id),
        str(request.run_id),
        str(request.expected_revision),
        str(request.expected_sequence),
        request.kind,
        str(request.request_digest),
        request.submitted_at.isoformat(),
        str(command.event_id),
        command.occurred_at.isoformat(),
        request.audit_metadata.key,
        request.audit_metadata.value,
    )
    return _digest((*common, *_command_payload(command)))


def tool_semantic_digest(request: ToolOperation) -> RequestDigest:
    """绑定工具请求和原始终态的全部不可变字段。."""
    return _digest(
        (
            str(request.tool_call_id),
            str(request.operation_id),
            str(request.run_id),
            request.tool_name,
            str(request.request_digest),
            request.status.value,
            request.result_reference.value if request.result_reference else "",
            request.metadata.key,
            request.metadata.value,
            request.error_code or "",
            request.occurred_at.isoformat(),
            request.audit_metadata.key,
            request.audit_metadata.value,
        )
    )


def tool_intent_semantic_digest(request: ToolIntent) -> RequestDigest:
    """仅绑定调用前可知且稳定的工具意图语义。."""
    return _digest(
        (
            str(request.tool_call_id),
            str(request.operation_id),
            str(request.run_id),
            request.tool_name,
            str(request.request_digest),
        )
    )


def interaction_request_digest(
    interaction_id: str, run_id: str, kind: str, requested_at: str
) -> RequestDigest:
    """绑定交互创建的完整不可变语义。."""
    return _digest((interaction_id, run_id, kind, requested_at))


def interaction_resolution_digest(
    response_digest: RequestDigest,
    result_reference: str,
    command_digest: RequestDigest,
) -> RequestDigest:
    """绑定交互响应、结果引用及命令语义。."""
    return _digest((str(response_digest), result_reference, str(command_digest)))


def _command_payload(command: RunCommand) -> tuple[str, ...]:
    """序列化闭合 T08 命令载荷而不保存正文。."""
    if type(command) is ClaimRun:
        return (str(command.execution_owner),)
    if type(command) is RequestInput or type(command) is SubmitInput:
        return (str(command.interaction_id),)
    if type(command) in (SucceedRun, FailRun, CancelRun, ReleaseExecution):
        return ()
    return (type(command).__name__,)


def _digest(parts: tuple[str, ...]) -> RequestDigest:
    """使用长度前缀避免字段边界碰撞。."""
    canonical = "".join(f"{len(part)}:{part}" for part in parts).encode()
    return RequestDigest.parse(sha256(canonical).hexdigest())
