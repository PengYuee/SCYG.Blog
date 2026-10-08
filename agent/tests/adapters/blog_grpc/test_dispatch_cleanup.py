"""Transaction exit failures retain ownership of an already-started real Call."""

from collections.abc import AsyncIterator, Awaitable
from contextlib import asynccontextmanager
from inspect import getcoroutinestate

import anyio
import pytest
from grpc import aio
from sqlalchemy.ext.asyncio import AsyncSession

from scyg_agent.agents.execution import PostgreSQLExecutionDispatch
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2 as service_pb2
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2_grpc as service_grpc
from tests.adapters.database.scripted_session_support import (
    NOW,
    FakeResult,
    ScriptedSession,
    session,
)
from tests.worker.test_dispatch import Transactions, lease_guard, live_run

from .support import FakeBlog


class FailedCommit(Transactions):
    error: BaseException
    started: anyio.Event | None
    active: bool

    def __init__(
        self, value: AsyncSession, error: BaseException, started: anyio.Event | None = None
    ) -> None:
        super().__init__(value)
        self.error = error
        self.started = started

    @asynccontextmanager
    async def begin(self) -> AsyncIterator[AsyncSession]:
        self.active = True
        try:
            yield self.value
        finally:
            self.active = False
        if self.started is not None:
            with anyio.fail_after(5):
                await self.started.wait()
        raise self.error


@pytest.mark.anyio
@pytest.mark.parametrize("cancelled", [False, True])
async def test_failed_commit_cancels_started_grpc_call(
    blog_server: tuple[str, FakeBlog],
    monkeypatch: pytest.MonkeyPatch,
    *,
    cancelled: bool,
) -> None:
    target, fake = blog_server
    error_type = anyio.get_cancelled_exc_class() if cancelled else RuntimeError
    fake.block = True
    transaction = FailedCommit(
        session(monkeypatch, ScriptedSession([FakeResult(live_run())])), error_type(), fake.started
    )
    dispatch = PostgreSQLExecutionDispatch(transaction.factory(), lease_guard(), lambda: NOW)
    async with aio.insecure_channel(target) as channel:
        stub = service_grpc.BlogContentServiceStub(channel)
        calls: list[
            aio.UnaryUnaryCall[service_pb2.GetArticleRequest, service_pb2.GetArticleResponse]
        ] = []

        def start() -> Awaitable[service_pb2.GetArticleResponse]:
            assert transaction.active
            call = stub.GetArticle(service_pb2.GetArticleRequest(user_id="admin", article_id=1))
            calls.append(call)
            return call

        with pytest.raises(error_type):
            _ = await dispatch.dispatch(start)
        assert calls[0].cancelled()
        with anyio.fail_after(5):
            await fake.drained.wait()


@pytest.mark.anyio
@pytest.mark.parametrize("cancelled", [False, True])
async def test_failed_commit_closes_unawaited_coroutine(
    monkeypatch: pytest.MonkeyPatch, *, cancelled: bool
) -> None:
    error_type = anyio.get_cancelled_exc_class() if cancelled else RuntimeError
    transaction = FailedCommit(
        session(monkeypatch, ScriptedSession([FakeResult(live_run())])), error_type()
    )
    dispatch = PostgreSQLExecutionDispatch(transaction.factory(), lease_guard(), lambda: NOW)

    async def response_body() -> str:
        transaction.events.append("await")
        return "response"

    response = response_body()
    with pytest.raises(error_type):
        _ = await dispatch.dispatch(lambda: response)
    assert getcoroutinestate(response) == "CORO_CLOSED"
    assert "await" not in transaction.events
