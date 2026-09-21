"""HTTP 路由表的外部契约测试。"""

from fastapi.routing import APIRoute

from scyg_agent.adapters.auth import (
    AuthenticationError,
    AuthenticationErrorCode,
    EncodedJwt,
    Principal,
)
from scyg_agent.transport.http import HTTPDependencies, create_http_router
from tests.application.test_facade import facade


class UnusedVerifier:
    """提供路由构造所需但不会被调用的验证接口。"""

    def verify(self, token: EncodedJwt) -> Principal:
        del token
        raise AuthenticationError(AuthenticationErrorCode.REJECTED)

    def require_exact_principal(self, principal: Principal) -> None:
        raise AssertionError(principal)


def test_route_table_exposes_no_create_route() -> None:
    # Given: T20 HTTP 路由器。
    application, _, _ = facade()
    router = create_http_router(HTTPDependencies(application, UnusedVerifier()))
    # When: 收集公开方法与路径。
    routes: set[tuple[str, str]] = set()
    for route in router.routes:
        if isinstance(route, APIRoute):
            methods = route.methods or set()
            routes.update((method, route.path) for method in methods)
    # Then: 只存在快照、事件、输入、命令和取消,绝无创建入口。
    assert ("POST", "/api/runs") not in routes
    assert routes == {
        ("GET", "/api/runs/{run_id}"),
        ("GET", "/api/runs/{run_id}/events"),
        ("POST", "/api/runs/{run_id}/input"),
        ("POST", "/api/runs/{run_id}/commands"),
        ("POST", "/api/runs/{run_id}/cancel"),
    }
