"""Real local BlogContent gRPC test service."""

from collections.abc import AsyncIterator, Awaitable, Callable
from typing import override

import anyio
import grpc
import pytest
from google.protobuf.message import Message
from google.protobuf.timestamp_pb2 import Timestamp
from grpc import aio

from scyg_agent.generated.proto.scyg.blog.v1 import article_pb2, common_pb2
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2 as service_pb2
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2_grpc as service_grpc


async def dispatch_call(start: Callable[[], Awaitable[object]]) -> object:
    """Test-only Call starter; production must supply its database Run-lock fence."""
    return await start()


class FakeBlog(service_grpc.BlogContentServiceServicer):
    def __init__(self) -> None:
        self.calls: list[str] = []
        self.operation_ids: list[str] = []
        self.requests: list[Message] = []
        self.status: grpc.StatusCode | None = None
        self.malformed: bool = False
        self.article_status: common_pb2.ArticleStatus = common_pb2.ARTICLE_STATUS_DRAFT
        self.search_articles: list[article_pb2.Article] | None = None
        self.block: bool = False
        self.started: anyio.Event = anyio.Event()
        self.drained: anyio.Event = anyio.Event()

    async def _before[Request: Message, Response: Message](
        self,
        name: str,
        request: Request,
        context: aio.ServicerContext[Request, Response],
    ) -> None:
        self.calls.append(name)
        if isinstance(
            request,
            (
                service_pb2.CreateArticleRequest,
                service_pb2.UpdateArticleRequest,
                service_pb2.PublishArticleRequest,
                service_pb2.ArchiveArticleRequest,
            ),
        ):
            self.operation_ids.append(request.operation_id)
        self.requests.append(request)
        if self.status is not None:
            await context.abort(self.status, "server-secret-token")
        if self.block:
            self.started.set()
            try:
                await anyio.sleep_forever()
            finally:
                self.drained.set()

    @override
    async def GetArticle(
        self,
        request: service_pb2.GetArticleRequest,
        context: aio.ServicerContext[service_pb2.GetArticleRequest, service_pb2.GetArticleResponse],
    ) -> service_pb2.GetArticleResponse:
        await self._before("get", request, context)
        return (
            service_pb2.GetArticleResponse()
            if self.malformed
            else service_pb2.GetArticleResponse(
                article=article(request.article_id, self.article_status),
            )
        )

    @override
    async def SearchArticles(
        self,
        request: service_pb2.SearchArticlesRequest,
        context: aio.ServicerContext[
            (
                service_pb2.SearchArticlesRequest,
                service_pb2.SearchArticlesResponse,
            )
        ],
    ) -> service_pb2.SearchArticlesResponse:
        await self._before("search", request, context)
        return service_pb2.SearchArticlesResponse(
            articles=(
                self.search_articles
                if self.search_articles is not None
                else [article(0 if self.malformed else 1, self.article_status)]
            ),
            page=request.page,
            page_size=request.page_size,
            total_items=len(self.search_articles) if self.search_articles is not None else 11,
            total_pages=(
                (len(self.search_articles) + request.page_size - 1) // request.page_size
                if self.search_articles is not None
                else (11 + request.page_size - 1) // request.page_size
            ),
        )

    @override
    async def ListTags(
        self,
        request: service_pb2.ListTagsRequest,
        context: aio.ServicerContext[service_pb2.ListTagsRequest, service_pb2.ListTagsResponse],
    ) -> service_pb2.ListTagsResponse:
        await self._before("tags", request, context)
        return service_pb2.ListTagsResponse(
            tags=[article_pb2.Tag(id=2, name="Tag")],
            page=request.page,
            page_size=request.page_size,
            total_items=1,
            total_pages=1,
        )

    @override
    async def ListArticleTypes(
        self,
        request: service_pb2.ListArticleTypesRequest,
        context: aio.ServicerContext[
            (
                service_pb2.ListArticleTypesRequest,
                service_pb2.ListArticleTypesResponse,
            )
        ],
    ) -> service_pb2.ListArticleTypesResponse:
        await self._before("types", request, context)
        return service_pb2.ListArticleTypesResponse(
            article_types=[article_pb2.ArticleType(id=3, name="Type")],
            page=request.page,
            page_size=request.page_size,
            total_items=1,
            total_pages=1,
        )

    @override
    async def CreateArticle(
        self,
        request: service_pb2.CreateArticleRequest,
        context: aio.ServicerContext[
            (
                service_pb2.CreateArticleRequest,
                service_pb2.CreateArticleResponse,
            )
        ],
    ) -> service_pb2.CreateArticleResponse:
        await self._before("create", request, context)
        return service_pb2.CreateArticleResponse(article=article(1))

    @override
    async def UpdateArticle(
        self,
        request: service_pb2.UpdateArticleRequest,
        context: aio.ServicerContext[
            (
                service_pb2.UpdateArticleRequest,
                service_pb2.UpdateArticleResponse,
            )
        ],
    ) -> service_pb2.UpdateArticleResponse:
        await self._before("update", request, context)
        return service_pb2.UpdateArticleResponse(article=article(request.article_id))

    @override
    async def PublishArticle(
        self,
        request: service_pb2.PublishArticleRequest,
        context: aio.ServicerContext[
            (
                service_pb2.PublishArticleRequest,
                service_pb2.PublishArticleResponse,
            )
        ],
    ) -> service_pb2.PublishArticleResponse:
        await self._before("publish", request, context)
        return service_pb2.PublishArticleResponse(
            article=article(request.article_id, common_pb2.ARTICLE_STATUS_PUBLISHED),
        )

    @override
    async def ArchiveArticle(
        self,
        request: service_pb2.ArchiveArticleRequest,
        context: aio.ServicerContext[
            (
                service_pb2.ArchiveArticleRequest,
                service_pb2.ArchiveArticleResponse,
            )
        ],
    ) -> service_pb2.ArchiveArticleResponse:
        await self._before("archive", request, context)
        return service_pb2.ArchiveArticleResponse(
            article=article(request.article_id, common_pb2.ARTICLE_STATUS_ARCHIVED),
        )


def article(
    article_id: int,
    status: common_pb2.ArticleStatus = common_pb2.ARTICLE_STATUS_DRAFT,
) -> article_pb2.Article:
    return article_pb2.Article(
        id=article_id,
        title="标题",
        content="正文",
        digest="摘要",
        slug="article",
        article_type_id=3,
        status=status,
        version=1,
        tags=[article_pb2.Tag(id=2, name="Tag")],
        created_at=Timestamp(seconds=1),
        updated_at=Timestamp(seconds=2),
        support=4,
        comment=5,
        visited=6,
    )


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.fixture
async def blog_server() -> AsyncIterator[tuple[str, FakeBlog]]:
    fake = FakeBlog()
    server = aio.server()
    service_grpc.add_BlogContentServiceServicer_to_server(fake, server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}", fake
    finally:
        await server.stop(None)
