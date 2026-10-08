from collections.abc import Awaitable, Iterator
from typing import Protocol

from grpc import aio

from . import health_pb2 as pb

class HealthStub:
    def __init__(self, channel: aio.Channel) -> None: ...
    Check: aio.UnaryUnaryMultiCallable[pb.HealthCheckRequest, pb.HealthCheckResponse]
    Watch: aio.UnaryStreamMultiCallable[pb.HealthCheckRequest, pb.HealthCheckResponse]

class HealthServicer(Protocol):
    def Check(
        self,
        request: pb.HealthCheckRequest,
        context: aio.ServicerContext[pb.HealthCheckRequest, pb.HealthCheckResponse],
    ) -> pb.HealthCheckResponse | Awaitable[pb.HealthCheckResponse]: ...
    def Watch(
        self,
        request: pb.HealthCheckRequest,
        context: aio.ServicerContext[pb.HealthCheckRequest, pb.HealthCheckResponse],
    ) -> Iterator[pb.HealthCheckResponse] | Awaitable[None]: ...

def add_HealthServicer_to_server(servicer: HealthServicer, server: aio.Server) -> None: ...
