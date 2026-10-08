"""Allowlisted asynchronous BlogContent client with fenced Call creation."""

from collections.abc import Awaitable, Callable
from types import TracebackType
from typing import TYPE_CHECKING, Final, Self, cast, final, override
from uuid import UUID

from google.protobuf.message import Message
from grpc import aio

from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2 as service_pb2
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2_grpc as service_grpc

from .contracts import (
    ArchiveArticle,
    BlogCommand,
    BlogFailure,
    BlogResult,
    CreateArticle,
    FailureKind,
    GetArticle,
    ListArticleTypes,
    ListTags,
    PublishArticle,
    RpcDispatch,
    SearchArticles,
    UpdateArticle,
)
from .mapping import article_response, article_types_response, search_response, tags_response
from .security import RpcDeadline
from .status import map_rpc_status

if TYPE_CHECKING:
    from scyg_agent.generated.proto.scyg.blog.v1 import common_pb2

_COMMANDS: Final = {
    "get_article": GetArticle,
    "search_articles": SearchArticles,
    "list_tags": ListTags,
    "list_article_types": ListArticleTypes,
    "create_article": CreateArticle,
    "update_article": UpdateArticle,
    "publish_article": PublishArticle,
    "archive_article": ArchiveArticle,
}
_UUID_VERSION: Final = 4


class ImmutableBlogConfigurationError(AttributeError):
    """Reject mutation of the client channel, deadline and generated stub."""


type PreparedCall = tuple[Callable[[], Awaitable[object]], Callable[[object], BlogResult]]


def _bind[RequestT: Message, ResponseT: Message](
    request: RequestT,
    method: aio.UnaryUnaryMultiCallable[RequestT, ResponseT],
    mapper: Callable[[ResponseT], BlogResult],
    deadline: float,
) -> PreparedCall:
    def start() -> Awaitable[object]:
        # Calling grpc.aio's callable synchronously starts the Call under the fence.
        return method(request, timeout=deadline)

    def map_response(response: object) -> BlogResult:
        # This Call's protobuf deserializer returns exactly its declared response.
        return mapper(cast("ResponseT", response))

    return start, map_response


def _update_request(command: UpdateArticle) -> service_pb2.UpdateArticleRequest:
    return service_pb2.UpdateArticleRequest(
        user_id=command.user_id,
        operation_id=command.operation_id.value,
        article_id=command.article_id,
        expected_version=command.expected_version,
        article_type_id=command.article_type_id,
        title=command.title,
        slug=command.slug,
        digest=command.digest,
        content=command.content,
        tag_ids=(
            service_pb2.TagIds(values=command.tag_ids) if command.tag_ids is not None else None
        ),
    )


@final
class BlogGrpcClient:
    """Prepare arguments before the fence; create the actual Call inside it."""

    __slots__ = ("_channel", "_closed", "_deadline", "_stub")

    def __init__(self, target: str, deadline_seconds: float) -> None:
        """Create an asynchronous channel with an immutable bounded deadline."""
        self._deadline = RpcDeadline.parse(deadline_seconds)
        self._channel = aio.insecure_channel(
            target, options=(("grpc.max_receive_message_length", -1),)
        )
        self._stub = service_grpc.BlogContentServiceStub(self._channel)
        self._closed = False

    @override
    def __setattr__(self, name: str, value: object) -> None:
        if hasattr(self, name) and (name != "_closed" or type(value) is not bool):
            raise ImmutableBlogConfigurationError
        object.__setattr__(self, name, value)

    @override
    def __delattr__(self, name: str) -> None:
        raise ImmutableBlogConfigurationError

    async def __aenter__(self) -> Self:
        """Return the same client as an owned asynchronous resource."""
        return self

    async def __aexit__(
        self,
        _kind: type[BaseException] | None,
        _error: BaseException | None,
        _traceback: TracebackType | None,
    ) -> None:
        """Close the owned channel when leaving the resource scope."""
        await self.close()

    async def close(self) -> None:
        """Close the channel and reject further dispatches."""
        await self._channel.close()
        self._closed = True

    async def invoke(
        self, tool_name: str, version: str, command: BlogCommand, *, dispatch: RpcDispatch
    ) -> BlogResult:
        """Start the real RPC inside the caller's cancellation admission fence."""
        failure = self._validate_boundary(tool_name, version, command)
        if failure is not None:
            return failure
        start, mapper = self._prepare(command)
        try:
            response = await dispatch(start)
            return mapper(response)
        except aio.AioRpcError as error:
            return map_rpc_status(error.code())

    def _validate_boundary(
        self, tool_name: str, version: str, command: BlogCommand
    ) -> BlogFailure | None:
        if self._closed:
            kind = FailureKind.CHANNEL_CLOSED
        elif version != "v1":
            kind = FailureKind.INVALID_VERSION
        elif tool_name not in _COMMANDS:
            kind = FailureKind.INVALID_TOOL
        elif _COMMANDS.get(tool_name) is not type(command):
            kind = FailureKind.WRONG_COMMAND
        elif isinstance(command, (CreateArticle, UpdateArticle, PublishArticle, ArchiveArticle)):
            try:
                operation = UUID(command.operation_id.value)
            except (ValueError, AttributeError):
                return BlogFailure(FailureKind.INVALID_ARGUMENT, retryable=False)
            if operation.version != _UUID_VERSION or str(operation) != command.operation_id.value:
                return BlogFailure(FailureKind.INVALID_ARGUMENT, retryable=False)
            return None
        else:
            return None
        return BlogFailure(kind, retryable=False)

    def _prepare(self, command: BlogCommand) -> PreparedCall:
        user = command.user_id
        deadline = self._deadline.seconds
        match command:
            case GetArticle():
                prepared = _bind(
                    service_pb2.GetArticleRequest(user_id=user, article_id=command.article_id),
                    self._stub.GetArticle,
                    article_response,
                    deadline,
                )
            case SearchArticles():
                prepared = _bind(
                    service_pb2.SearchArticlesRequest(
                        user_id=user,
                        query=command.query,
                        page=command.page,
                        page_size=command.page_size,
                        status=cast("common_pb2.ArticleStatus", command.status),
                        article_type_id=command.article_type_id,
                        tag_id=command.tag_id,
                        sort=command.sort,
                    ),
                    self._stub.SearchArticles,
                    search_response,
                    deadline,
                )
            case ListTags():
                prepared = _bind(
                    service_pb2.ListTagsRequest(
                        user_id=user,
                        page=command.page,
                        page_size=command.page_size,
                        query=command.query,
                        sort=command.sort,
                    ),
                    self._stub.ListTags,
                    tags_response,
                    deadline,
                )
            case ListArticleTypes():
                prepared = _bind(
                    service_pb2.ListArticleTypesRequest(
                        user_id=user,
                        page=command.page,
                        page_size=command.page_size,
                        query=command.query,
                        sort=command.sort,
                    ),
                    self._stub.ListArticleTypes,
                    article_types_response,
                    deadline,
                )
            case CreateArticle():
                prepared = _bind(
                    service_pb2.CreateArticleRequest(
                        user_id=user,
                        operation_id=command.operation_id.value,
                        status=cast("common_pb2.ArticleStatus", command.status),
                        article_type_id=command.article_type_id,
                        title=command.title,
                        slug=command.slug,
                        digest=command.digest,
                        content=command.content,
                        tag_ids=command.tag_ids,
                    ),
                    self._stub.CreateArticle,
                    article_response,
                    deadline,
                )
            case UpdateArticle():
                prepared = _bind(
                    _update_request(command),
                    self._stub.UpdateArticle,
                    article_response,
                    deadline,
                )
            case PublishArticle():
                prepared = _bind(
                    service_pb2.PublishArticleRequest(
                        user_id=user,
                        operation_id=command.operation_id.value,
                        article_id=command.article_id,
                        expected_version=command.expected_version,
                    ),
                    self._stub.PublishArticle,
                    article_response,
                    deadline,
                )
            case ArchiveArticle():
                prepared = _bind(
                    service_pb2.ArchiveArticleRequest(
                        user_id=user,
                        operation_id=command.operation_id.value,
                        article_id=command.article_id,
                        expected_version=command.expected_version,
                    ),
                    self._stub.ArchiveArticle,
                    article_response,
                    deadline,
                )
        return prepared
