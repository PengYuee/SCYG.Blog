"""Worker 持久化事实的确定性身份."""

from hashlib import sha256

from scyg_agent.domain.runs import CommandId, EventId, Run


def command_id(run: Run, attempt: int) -> CommandId:
    """按 Run 和执行次数构造稳定命令身份."""
    digest = sha256(f"{run.id}:{attempt}:worker".encode()).hexdigest()[:24]
    return CommandId(f"cmd_{digest}")


def event_id(run: Run, kind: str) -> EventId:
    """按 Run 当前执行次数和事件种类构造稳定事件身份."""
    digest = sha256(f"{run.id}:{run.attempt}:{kind}".encode()).hexdigest()[:24]
    return EventId(f"evt_{digest}")
