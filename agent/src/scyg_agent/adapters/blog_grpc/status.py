"""将 gRPC 状态映射到封闭且无秘密的失败。."""

from typing import Final

import grpc

from .contracts import BlogFailure, FailureKind

_STATUS_MAP: Final[dict[grpc.StatusCode, tuple[FailureKind, bool]]] = {
    grpc.StatusCode.INVALID_ARGUMENT: (FailureKind.INVALID_ARGUMENT, False),
    grpc.StatusCode.NOT_FOUND: (FailureKind.NOT_FOUND, False),
    grpc.StatusCode.ALREADY_EXISTS: (FailureKind.DUPLICATE, False),
    grpc.StatusCode.FAILED_PRECONDITION: (FailureKind.FAILED_PRECONDITION, False),
    grpc.StatusCode.ABORTED: (FailureKind.FAILED_PRECONDITION, False),
    grpc.StatusCode.PERMISSION_DENIED: (FailureKind.AUTHORIZATION, False),
    grpc.StatusCode.UNAUTHENTICATED: (FailureKind.AUTHORIZATION, False),
    grpc.StatusCode.RESOURCE_EXHAUSTED: (FailureKind.RESOURCE_EXHAUSTED, True),
    grpc.StatusCode.UNAVAILABLE: (FailureKind.UNAVAILABLE, True),
    grpc.StatusCode.DEADLINE_EXCEEDED: (FailureKind.DEADLINE_EXCEEDED, True),
    grpc.StatusCode.CANCELLED: (FailureKind.CANCELLED, False),
    grpc.StatusCode.OK: (FailureKind.INTERNAL, False),
    grpc.StatusCode.UNKNOWN: (FailureKind.INTERNAL, False),
    grpc.StatusCode.UNIMPLEMENTED: (FailureKind.INTERNAL, False),
    grpc.StatusCode.INTERNAL: (FailureKind.INTERNAL, False),
    grpc.StatusCode.DATA_LOSS: (FailureKind.INTERNAL, False),
    grpc.StatusCode.OUT_OF_RANGE: (FailureKind.INTERNAL, False),
}


def map_rpc_status(status: grpc.StatusCode) -> BlogFailure:
    """丢弃远端详情并返回稳定类别和重试属性。."""
    kind, retryable = _STATUS_MAP.get(status, (FailureKind.INTERNAL, False))
    return BlogFailure(kind, retryable=retryable)
