"""认证边界的稳定公开错误。."""

from enum import StrEnum
from typing import Final, final, override


class AuthenticationErrorCode(StrEnum):
    """封闭认证失败类别, 避免携带不可信输入."""

    MISSING_CREDENTIAL = "missing_credential"
    MALFORMED_CREDENTIAL = "malformed_credential"
    REJECTED = "rejected"
    INVALID_PRINCIPAL = "invalid_principal"


ERROR_MESSAGES: Final[dict[AuthenticationErrorCode, str]] = {
    AuthenticationErrorCode.MISSING_CREDENTIAL: "缺少身份凭据",
    AuthenticationErrorCode.MALFORMED_CREDENTIAL: "身份凭据格式无效",
    AuthenticationErrorCode.REJECTED: "身份令牌无效",
    AuthenticationErrorCode.INVALID_PRINCIPAL: "身份主体无效",
}


@final
class AuthenticationError(Exception):
    """向传输层提供不含令牌、声明和配置的类型化错误。."""

    __slots__ = ("_code",)

    def __init__(self, code: AuthenticationErrorCode) -> None:
        """仅保存封闭错误码, 不接受不可信上下文."""
        super().__init__()
        self._code: AuthenticationErrorCode = code

    @property
    def code(self) -> AuthenticationErrorCode:
        """返回稳定错误类别."""
        return self._code

    @override
    def __str__(self) -> str:
        """返回稳定中文消息。."""
        return ERROR_MESSAGES[self.code]
