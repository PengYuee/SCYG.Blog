"""AgentControl gRPC 元数据认证边界."""

from typing import NoReturn

import grpc
from grpc import aio

from scyg_agent.adapters.auth import (
    AuthenticationError,
    BlogServicePrincipal,
    JwtVerifier,
    WebRunPrincipal,
    parse_bearer_values,
)

AUTHORIZATION_KEY = "authorization"
AUTHENTICATION_DETAIL = "身份认证失败"
AUTHORIZATION_DETAIL = "服务主体无权调用此接口"


async def authenticate_blog_service[RequestT, ResponseT](
    context: aio.ServicerContext[RequestT, ResponseT],
    verifier: JwtVerifier,
) -> BlogServicePrincipal:
    """要求恰好一个 Bearer 值且主体必须是受信任 Blog 服务."""
    metadata = context.invocation_metadata() or ()
    values = tuple(value for key, value in metadata if key.lower() == AUTHORIZATION_KEY)
    try:
        principal = verifier.verify(parse_bearer_values(values))
        verifier.require_exact_principal(principal)
    except AuthenticationError:
        await _abort(context, grpc.StatusCode.UNAUTHENTICATED, AUTHENTICATION_DETAIL)
    match principal:  # noqa: RUF100  # noqa: MATCH_OK - 主体联合已完整映射。
        case BlogServicePrincipal():
            return principal
        case WebRunPrincipal():
            return await _abort(context, grpc.StatusCode.PERMISSION_DENIED, AUTHORIZATION_DETAIL)


async def _abort[RequestT, ResponseT](
    context: aio.ServicerContext[RequestT, ResponseT],
    code: grpc.StatusCode,
    detail: str,
) -> NoReturn:
    """以稳定中文状态终止 RPC 且不回显底层异常."""
    await context.abort(code, detail)
