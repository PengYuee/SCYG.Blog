"""HTTP Authorization 到单 Run 主体的严格边界。."""

from dataclasses import dataclass
from typing import Protocol

from fastapi import HTTPException, Request, status

from scyg_agent.adapters.auth import (
    AuthenticationError,
    EncodedJwt,
    Principal,
    WebRunPrincipal,
    parse_bearer_values,
)
from scyg_agent.application import OwnerContext
from scyg_agent.domain.runs import RunId
from scyg_agent.domain.runs.errors import InvalidIdentifierError


class PrincipalVerifier(Protocol):
    """声明 HTTP 所需的最小 JWT 验证能力。."""

    def verify(self, token: EncodedJwt) -> Principal:
        """验证编码令牌。."""
        ...

    def require_exact_principal(self, principal: Principal) -> None:
        """拒绝主体子类。."""
        ...


@dataclass(frozen=True, slots=True)
class AuthorizedRun:
    """保存已经绑定 URL 的 Web Run 主体。."""

    owner: OwnerContext
    run_id: RunId


def authorize_run(request: Request, raw_run_id: str, verifier: PrincipalVerifier) -> AuthorizedRun:
    """认证唯一 Authorization 头并绑定 URL Run。."""
    try:
        principal = verifier.verify(parse_bearer_values(request.headers.getlist("authorization")))
        verifier.require_exact_principal(principal)
    except AuthenticationError as error:
        raise HTTPException(status.HTTP_401_UNAUTHORIZED, str(error)) from None
    if type(principal) is not WebRunPrincipal:
        raise HTTPException(status.HTTP_403_FORBIDDEN, "当前身份无权访问 Web Run")
    try:
        run_id = RunId(raw_run_id)
    except InvalidIdentifierError:
        raise HTTPException(status.HTTP_422_UNPROCESSABLE_CONTENT, "Run 标识无效") from None
    if principal.run_id != run_id:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "未找到 Run")
    return AuthorizedRun(OwnerContext(principal.user_id), run_id)
