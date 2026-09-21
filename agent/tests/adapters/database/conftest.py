"""数据库适配器测试夹具导出。"""

from .t12_postgres_support import anyio_backend, t12_database

__all__ = ["anyio_backend", "t12_database"]
