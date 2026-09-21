"""真实 Blog gRPC 测试服务与类型化夹具。"""

from collections.abc import AsyncIterator
from typing import TYPE_CHECKING, NoReturn, Protocol, override

import anyio
import grpc
import pytest
from grpc import aio

from scyg_agent.generated.scyg.blog.v1 import article_pb2, common_pb2
from scyg_agent.generated.scyg.blog.v1 import blog_tool_service_pb2 as service_pb2
from scyg_agent.generated.scyg.blog.v1 import blog_tool_service_pb2_grpc as service_grpc

if TYPE_CHECKING:

    def add_blog_servicer(_servicer: "FakeBlog", _server: aio.Server) -> None:
        """声明生成注册函数缺失的静态类型。"""

else:
    add_blog_servicer = service_grpc.add_BlogToolServiceServicer_to_server


NEXT_PAGE_CURSOR = "page-next"


class RpcContext(Protocol):
    """描述测试服务使用的最小上下文能力。"""

    def time_remaining(self) -> float:
        """返回客户端剩余截止期秒数。"""
        ...

    async def abort(self, code: grpc.StatusCode, details: str) -> NoReturn:
        """以指定远端状态终止调用。"""
        ...


class FakeBlog(service_grpc.BlogToolServiceServicer):
    """记录真实传输请求并提供可控响应和生命周期。"""

    def __init__(self) -> None:
        """初始化每个测试独占的可变观测状态。"""
        self.calls: list[str] = []
        self.operation_ids: list[str] = []
        self.request_metadata: list[common_pb2.ToolRequestMetadata] = []
        self.deadlines: list[float] = []
        self.status: grpc.StatusCode | None = None
        self.malformed: bool = False
        self.block: bool = False
        self.started: anyio.Event = anyio.Event()
        self.drained: anyio.Event = anyio.Event()

    async def _before(self, name: str, context: RpcContext) -> None:
        self.calls.append(name)
        self.deadlines.append(context.time_remaining())
        if self.status is not None:
            await context.abort(self.status, "server-secret-token")
        if self.block:
            self.started.set()
            try:
                await anyio.sleep_forever()
            finally:
                self.drained.set()

    @override
    async def GetPublishedArticle(
        self,
        request: service_pb2.GetPublishedArticleRequest,
        context: aio.ServicerContext[
            service_pb2.GetPublishedArticleRequest,
            service_pb2.GetPublishedArticleResponse,
        ],
    ) -> service_pb2.GetPublishedArticleResponse:
        self.request_metadata.append(request.metadata)
        await self._before("get", context)
        if self.malformed:
            return service_pb2.GetPublishedArticleResponse()
        return service_pb2.GetPublishedArticleResponse(article=article(request.article_id.value))

    @override
    async def SearchArticles(
        self,
        request: service_pb2.SearchArticlesRequest,
        context: aio.ServicerContext[
            service_pb2.SearchArticlesRequest,
            service_pb2.SearchArticlesResponse,
        ],
    ) -> service_pb2.SearchArticlesResponse:
        self.request_metadata.append(request.metadata)
        await self._before("search", context)
        hit = article_pb2.ArticleSearchHit(
            id=common_pb2.ArticleId(value="hit-1"), title=request.query, summary="摘要"
        )
        if self.malformed:
            hit.ClearField("id")
        return service_pb2.SearchArticlesResponse(articles=[hit], next_page_token=NEXT_PAGE_CURSOR)

    @override
    async def CreateArticleDraft(
        self,
        request: service_pb2.CreateArticleDraftRequest,
        context: aio.ServicerContext[
            service_pb2.CreateArticleDraftRequest,
            service_pb2.CreateArticleDraftResponse,
        ],
    ) -> service_pb2.CreateArticleDraftResponse:
        self._record_write(request.metadata)
        await self._before("create", context)
        return service_pb2.CreateArticleDraftResponse(article=article("created-1"))

    @override
    async def UpdateArticleDraft(
        self,
        request: service_pb2.UpdateArticleDraftRequest,
        context: aio.ServicerContext[
            service_pb2.UpdateArticleDraftRequest,
            service_pb2.UpdateArticleDraftResponse,
        ],
    ) -> service_pb2.UpdateArticleDraftResponse:
        self._record_write(request.metadata)
        await self._before("update", context)
        return service_pb2.UpdateArticleDraftResponse(article=article(request.article_id.value))

    @override
    async def AddArticleTags(
        self,
        request: service_pb2.AddArticleTagsRequest,
        context: aio.ServicerContext[
            service_pb2.AddArticleTagsRequest,
            service_pb2.AddArticleTagsResponse,
        ],
    ) -> service_pb2.AddArticleTagsResponse:
        self._record_write(request.metadata)
        await self._before("tags", context)
        return service_pb2.AddArticleTagsResponse(article=article(request.article_id.value))

    def _record_write(self, metadata: common_pb2.WriteOperationMetadata) -> None:
        self.operation_ids.append(metadata.operation_id)
        self.request_metadata.append(metadata.request)


def article(article_id: str) -> article_pb2.Article:
    """构造语义完整的文章响应。"""
    return article_pb2.Article(
        id=common_pb2.ArticleId(value=article_id),
        title="标题",
        body_markdown="正文",
        summary="摘要",
        status=common_pb2.ARTICLE_STATUS_DRAFT,
        version=1,
    )


@pytest.fixture
def anyio_backend() -> str:
    """grpc.aio 明确运行在 asyncio 后端。"""
    return "asyncio"


@pytest.fixture
async def blog_server() -> AsyncIterator[tuple[str, FakeBlog]]:
    """启动并确定停止一个本地真实 grpc.aio 服务。"""
    fake = FakeBlog()
    server = aio.server()
    add_blog_servicer(fake, server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}", fake
    finally:
        await server.stop(None)
