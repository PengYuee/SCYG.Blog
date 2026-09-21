"""注册 Blog gRPC 本地服务夹具。"""

from .support import anyio_backend, blog_server

__all__ = ["anyio_backend", "blog_server"]
