"""Blog gRPC 授权与截止期安全快照。."""

from dataclasses import dataclass
from math import isfinite
from typing import Final, override

from scyg_agent.domain.runs import RuntimeSelection
from scyg_agent.runtimes.profiles import RuntimeProfile, ToolPermission

MIN_DEADLINE_SECONDS: Final = 0.001
MAX_DEADLINE_SECONDS: Final = 3600.0


@dataclass(frozen=True, slots=True)
class InvalidDeadlineError(ValueError):
    """报告非正、非有限或无界截止期。."""

    @override
    def __str__(self) -> str:
        """返回不回显配置值的中文错误。."""
        return "Blog gRPC 截止期必须为有限正数且不超过 3600 秒"


@dataclass(frozen=True, slots=True)
class InvalidAuthorizationError(ValueError):
    """报告运行时画像不属于精确授权闭集。."""

    @override
    def __str__(self) -> str:
        """返回不回显画像内容的中文错误。."""
        return "Blog gRPC 运行时画像无效"


@dataclass(frozen=True, slots=True)
class RpcDeadline:
    """保存经过精确类型和有限上界解析的 RPC 截止期。."""

    seconds: float

    def __post_init__(self) -> None:
        """拒绝绕过解析工厂直接构造的无效值。."""
        if (
            type(self.seconds) is not float
            or not isfinite(self.seconds)
            or not MIN_DEADLINE_SECONDS <= self.seconds <= MAX_DEADLINE_SECONDS
        ):
            raise InvalidDeadlineError

    @classmethod
    def parse(cls, value: float) -> "RpcDeadline":
        """在创建通道前解析严格浮点截止期。."""
        return cls(value)


@dataclass(frozen=True, slots=True)
class AuthorizationSnapshot:
    """保存构造时画像身份和 T14 权限的不可变快照。."""

    selection: RuntimeSelection
    permissions: tuple[ToolPermission, ...]

    @classmethod
    def capture(cls, profile: RuntimeProfile) -> "AuthorizationSnapshot":
        """拒绝画像子类并复制安全关键字段。."""
        if type(profile) is not RuntimeProfile:
            raise InvalidAuthorizationError
        return cls(
            RuntimeSelection(profile.selection.kind, profile.selection.version),
            tuple(sorted(profile.tool_permissions, key=lambda permission: permission.value)),
        )


@dataclass(frozen=True, slots=True)
class SecurityConfiguration:
    """组合一次性初始化的唯一授权与截止期信任根。."""

    authorization: AuthorizationSnapshot
    deadline: RpcDeadline

    @classmethod
    def parse(cls, profile: RuntimeProfile, deadline_seconds: float) -> "SecurityConfiguration":
        """在创建通道前构造无可变别名的安全配置。."""
        return cls(AuthorizationSnapshot.capture(profile), RpcDeadline.parse(deadline_seconds))
