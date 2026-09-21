"""静态允许列表 BlogTool 异步 gRPC 客户端。."""

from collections.abc import Awaitable, Callable
from types import TracebackType
from typing import Final, Protocol, Self, final, override

from grpc import aio

from scyg_agent.generated.scyg.blog.v1 import blog_tool_service_pb2 as service_pb2
from scyg_agent.generated.scyg.blog.v1 import blog_tool_service_pb2_grpc as service_grpc
from scyg_agent.generated.scyg.blog.v1 import common_pb2
from scyg_agent.runtimes.profiles import RUNTIME_VERSION_V1, RuntimeProfile, ToolPermission

from .contracts import (
    AddArticleTags,
    BlogCommand,
    BlogFailure,
    BlogResult,
    CreateArticleDraft,
    FailureKind,
    GetPublishedArticle,
    RequestIdentity,
    SearchArticles,
    UpdateArticleDraft,
)
from .mapping import article_response, search_response
from .security import SecurityConfiguration
from .status import map_rpc_status


class BlogStub(Protocol):
    """描述生成桩中五个获批的一元调用。."""

    GetPublishedArticle: Callable[..., Awaitable[service_pb2.GetPublishedArticleResponse]]
    SearchArticles: Callable[..., Awaitable[service_pb2.SearchArticlesResponse]]
    CreateArticleDraft: Callable[..., Awaitable[service_pb2.CreateArticleDraftResponse]]
    UpdateArticleDraft: Callable[..., Awaitable[service_pb2.UpdateArticleDraftResponse]]
    AddArticleTags: Callable[..., Awaitable[service_pb2.AddArticleTagsResponse]]


@final
class BlogGrpcClient:
    """在一个可复用通道上调用画像明确授权的 Blog RPC。."""

    __slots__: Final = ("_channel", "_closed", "_security", "_stub")

    def __init__(self, target: str, profile: RuntimeProfile, deadline_seconds: float) -> None:
        """构造具有固定画像和有限截止期的客户端。."""
        security = SecurityConfiguration.parse(profile, deadline_seconds)
        self._security: SecurityConfiguration = security
        self._channel: aio.Channel = aio.insecure_channel(target)
        self._stub: BlogStub = service_grpc.BlogToolServiceStub(self._channel)
        self._closed: bool = False

    @override
    def __setattr__(
        self,
        name: str,
        value: Self | aio.Channel | BlogStub | SecurityConfiguration | bool,
    ) -> None:
        """仅允许生命周期闭合标记在初始化后发生变化。."""
        if name == "_closed" and hasattr(self, "_closed"):
            if type(value) is not bool:
                msg = "Blog gRPC 生命周期状态必须为 bool"
                raise AttributeError(msg)
            object.__setattr__(self, name, value)
            return
        if hasattr(self, name):
            msg = "Blog gRPC 安全配置初始化后不可修改"
            raise AttributeError(msg)
        object.__setattr__(self, name, value)

    @override
    def __delattr__(self, _name: str) -> None:
        """禁止删除安全或生命周期字段后重建。."""
        msg = "Blog gRPC 客户端字段不可删除"
        raise AttributeError(msg)

    async def __aenter__(self) -> "BlogGrpcClient":
        """进入客户端资源作用域。."""
        return self

    async def __aexit__(
        self,
        _exception_type: type[BaseException] | None,
        _exception: BaseException | None,
        _traceback: TracebackType | None,
    ) -> None:
        """离开作用域时关闭通道并取消遗留调用。."""
        await self.close()

    async def close(self) -> None:
        """幂等关闭通道并取消遗留调用。."""
        await self._channel.close()
        self._closed = True

    async def invoke(self, tool_name: str, version: str, command: BlogCommand) -> BlogResult:
        """在网络调用前验证版本、授权和命令精确类型。."""
        boundary_failure = self._validate_boundary(tool_name, version, command)
        if boundary_failure is not None:
            return boundary_failure
        try:
            return await self._dispatch(command)
        except aio.AioRpcError as error:
            return map_rpc_status(error.code())

    def _validate_boundary(
        self, tool_name: str, version: str, command: BlogCommand
    ) -> BlogFailure | None:
        """把不可信工具选择解析为 T14 权限并校验命令绑定。."""
        state_failure = self._state_failure()
        if state_failure is not None:
            return state_failure
        if version != RUNTIME_VERSION_V1:
            return BlogFailure(FailureKind.INVALID_VERSION, retryable=False)
        try:
            permission = ToolPermission(tool_name)
        except ValueError:
            return BlogFailure(FailureKind.INVALID_TOOL, retryable=False)
        if permission not in self._security.authorization.permissions:
            return BlogFailure(FailureKind.INVALID_TOOL, retryable=False)
        if not _command_matches(permission, command):
            return BlogFailure(FailureKind.WRONG_COMMAND, retryable=False)
        return None

    def _state_failure(self) -> BlogFailure | None:
        """在业务边界前关闭生命周期和安全状态失败。."""
        if self._closed:
            return BlogFailure(FailureKind.CHANNEL_CLOSED, retryable=False)
        return None

    async def _dispatch(self, command: BlogCommand) -> BlogResult:
        """对已验证命令闭集执行唯一显式 RPC。."""
        match command:  # noqa: RUF100  # noqa: MATCH_OK - 五个精确分支已静态穷尽。
            case GetPublishedArticle():
                response = await self._stub.GetPublishedArticle(
                    service_pb2.GetPublishedArticleRequest(
                        metadata=_request_metadata(command.identity),
                        article_id=common_pb2.ArticleId(value=command.article_id),
                    ),
                    timeout=self._security.deadline.seconds,
                )
                return article_response(response)
            case SearchArticles():
                response = await self._stub.SearchArticles(
                    service_pb2.SearchArticlesRequest(
                        metadata=_request_metadata(command.identity),
                        query=command.query,
                        page_size=command.page_size,
                        page_token=command.page_token,
                    ),
                    timeout=self._security.deadline.seconds,
                )
                return search_response(response)
            case CreateArticleDraft():
                response = await self._stub.CreateArticleDraft(
                    service_pb2.CreateArticleDraftRequest(
                        metadata=_write_metadata(command.identity, command.operation_id.value),
                        author_user_id=common_pb2.UserId(value=command.author_user_id),
                        title=command.title,
                        body_markdown=command.body_markdown,
                        summary=command.summary,
                    ),
                    timeout=self._security.deadline.seconds,
                )
                return article_response(response)
            case UpdateArticleDraft():
                response = await self._stub.UpdateArticleDraft(
                    service_pb2.UpdateArticleDraftRequest(
                        metadata=_write_metadata(command.identity, command.operation_id.value),
                        article_id=common_pb2.ArticleId(value=command.article_id),
                        expected_version=command.expected_version,
                        title=command.title,
                        body_markdown=command.body_markdown,
                        summary=command.summary,
                    ),
                    timeout=self._security.deadline.seconds,
                )
                return article_response(response)
            case AddArticleTags():
                response = await self._stub.AddArticleTags(
                    service_pb2.AddArticleTagsRequest(
                        metadata=_write_metadata(command.identity, command.operation_id.value),
                        article_id=common_pb2.ArticleId(value=command.article_id),
                        expected_version=command.expected_version,
                        tag_ids=tuple(common_pb2.TagId(value=value) for value in command.tag_ids),
                    ),
                    timeout=self._security.deadline.seconds,
                )
                return article_response(response)


def _command_matches(permission: ToolPermission, command: BlogCommand) -> bool:
    """检查 T14 权限名称与精确命令类型的一一绑定。."""
    expected_types: Final = {
        ToolPermission.GET_PUBLISHED_ARTICLE: GetPublishedArticle,
        ToolPermission.SEARCH_ARTICLES: SearchArticles,
        ToolPermission.CREATE_ARTICLE_DRAFT: CreateArticleDraft,
        ToolPermission.UPDATE_ARTICLE_DRAFT: UpdateArticleDraft,
        ToolPermission.ADD_ARTICLE_TAGS: AddArticleTags,
    }
    expected_type = expected_types.get(permission)
    return expected_type is not None and type(command) is expected_type


def _request_metadata(identity: RequestIdentity) -> common_pb2.ToolRequestMetadata:
    """精确构造调用追踪元数据。."""
    return common_pb2.ToolRequestMetadata(
        request_id=identity.request_id,
        correlation_id=identity.correlation_id,
        causation_id=identity.causation_id,
        run_id=identity.run_id.value,
        tool_call_id=identity.tool_call_id.value,
    )


def _write_metadata(
    identity: RequestIdentity, operation_id: str
) -> common_pb2.WriteOperationMetadata:
    """仅在契约字段中传播一次规范幂等身份。."""
    return common_pb2.WriteOperationMetadata(
        request=_request_metadata(identity), operation_id=operation_id
    )
