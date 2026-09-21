"""认证成功后允许进入应用层的封闭主体。."""

from dataclasses import dataclass

from scyg_agent.domain.runs import RunId, UserId


@dataclass(frozen=True, slots=True)
class BlogServicePrincipal:
    """代表配置中唯一受信任的 Blog 服务主体。."""

    subject: str


@dataclass(frozen=True, slots=True)
class WebRunPrincipal:
    """代表一个用户对单一 Run 的最小权限绑定。."""

    user_id: UserId
    run_id: RunId

    @classmethod
    def from_strings(cls, user_id: str, run_id: str) -> "WebRunPrincipal":
        """严格解析外部用户与 Run 品牌标识。."""
        return cls(UserId(user_id), RunId(run_id))


type Principal = BlogServicePrincipal | WebRunPrincipal
