"""HTTP Authorization 与 gRPC metadata 的共享 Bearer 边界。."""

from collections.abc import Sequence
from dataclasses import dataclass
from typing import Final, override

from .errors import AuthenticationError, AuthenticationErrorCode

BEARER_PREFIX: Final = "Bearer "
MAX_TOKEN_CHARACTERS: Final = 8192


@dataclass(frozen=True, slots=True)
class EncodedJwt:
    """封装尚未验证且禁止默认回显的紧凑 JWT。."""

    value: str

    @override
    def __repr__(self) -> str:
        """隐藏原始令牌, 避免诊断与日志意外泄露."""
        return "EncodedJwt(***)"


def parse_bearer_values(values: Sequence[str]) -> EncodedJwt:
    """要求 HTTP 头或 gRPC metadata 中恰好一个规范 Bearer 值。."""
    if not values:
        raise AuthenticationError(AuthenticationErrorCode.MISSING_CREDENTIAL)
    if len(values) != 1:
        raise AuthenticationError(AuthenticationErrorCode.MALFORMED_CREDENTIAL)
    value = values[0]
    if not value.startswith(BEARER_PREFIX):
        raise AuthenticationError(AuthenticationErrorCode.MALFORMED_CREDENTIAL)
    token = value.removeprefix(BEARER_PREFIX)
    if (
        not token
        or len(token) > MAX_TOKEN_CHARACTERS
        or token.strip() != token
        or any(character.isspace() for character in token)
    ):
        raise AuthenticationError(AuthenticationErrorCode.MALFORMED_CREDENTIAL)
    return EncodedJwt(token)
