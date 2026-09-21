"""Worker 借用外部持久化资源时的取消边界."""

from collections.abc import Awaitable

import anyio


async def finish_persistence[PersistenceResult](
    operation: Awaitable[PersistenceResult],
) -> PersistenceResult:
    """屏蔽 Worker 取消直到资源所有者完成事务关闭或回滚."""
    with anyio.CancelScope(shield=True):
        return await operation
    reason = "持久化事务未返回结果"
    raise RuntimeError(reason)
