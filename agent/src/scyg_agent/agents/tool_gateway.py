"""从可信运行上下文授权现有 Blog 只读 RPC, 不读取 checkpoint 权限."""

import hashlib
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import asdict
from typing import Annotated, ClassVar, Protocol, cast, override
from uuid import uuid4

from langchain.agents.middleware import AgentMiddleware
from langchain.agents.middleware.types import AgentState
from langchain_core.messages import ToolMessage
from langchain_core.tools import BaseTool, InjectedToolArg, StructuredTool
from langgraph.prebuilt import ToolRuntime
from langgraph.prebuilt.tool_node import ToolCallRequest
from langgraph.types import Command
from pydantic import BaseModel, ConfigDict, Field, ValidationError

from scyg_agent.adapters.blog_grpc.contracts import (
    ArticleId,
    ArticleSucceeded,
    BlogCommand,
    BlogFailure,
    BlogResult,
    BlogUserId,
    CorrelationId,
    GetArticle,
    RequestId,
    RequestIdentity,
    RpcDispatch,
    SearchArticles,
    SearchSucceeded,
)
from scyg_agent.domain.runs import RunId, ToolCallId, UserId
from scyg_agent.domain.runs.errors import InvalidIdentifierError

from .context import AgentRequestContext, Quality
from .recipes import AgentRecipe, RecipeRegistry
from .tool_catalog import READ_TOOL_CATALOG, ReadToolName

_MAX_RESOURCE_ID = 2**63 - 1


class _RuntimeView(Protocol):
    """仅投影授权需要的 SDK 注入字段, 不依赖动态 state 类型."""

    @property
    def context(self) -> object:
        """返回 SDK 注入的运行上下文."""
        ...

    @property
    def config(self) -> Mapping[str, object]:
        """返回 SDK 执行配置."""
        ...

    @property
    def tool_call_id(self) -> object:
        """返回 SDK 注入的实际调用身份."""
        ...


class _ToolRequestView(Protocol):
    """限制 SDK 请求动态字段的静态边界."""

    @property
    def tool_call(self) -> Mapping[str, object]:
        """返回尚未注入控制字段的模型调用."""
        ...

    @property
    def runtime(self) -> object:
        """返回 SDK 运行对象."""
        ...

    @property
    def tool(self) -> BaseTool | None:
        """返回实际注册的工具实例."""
        ...


class BlogReadClient(Protocol):
    """复用应用拥有的 Blog 客户端和既有截止期."""

    async def invoke(
        self, tool_name: str, version: str, command: BlogCommand, *, dispatch: RpcDispatch
    ) -> BlogResult:
        """Start the final RPC inside the supplied cancellation fence."""
        ...


class ToolAccessDeniedError(Exception):
    """稳定、无请求值的工具授权失败."""

    def __init__(self) -> None:
        """构造不包含上下文值的拒绝异常."""
        super().__init__("Agent tool access denied")


class BlogToolFailureError(Exception):
    """稳定、无底层服务详情的工具依赖失败."""

    retryable: bool

    def __init__(self, *, retryable: bool) -> None:
        """仅携带稳定的依赖重试策略."""
        self.retryable = retryable
        super().__init__("Agent Blog tool failed")


class _PageArgs(BaseModel):
    """Strict management pagination for article queries."""

    model_config: ClassVar[ConfigDict] = ConfigDict(extra="forbid", strict=True, frozen=True)
    query: Annotated[str, Field(max_length=500)] = ""
    page: Annotated[int, Field(ge=1, le=2**32 - 1)] = 1
    page_size: Annotated[int, Field(ge=1, le=100)] = 20
    sort: Annotated[str, Field(max_length=100)] = ""


class _SearchArgs(_PageArgs):
    status: Annotated[int, Field(ge=0, le=3)] = 0
    article_type_id: Annotated[int, Field(ge=0, le=_MAX_RESOURCE_ID)] = 0
    tag_id: Annotated[int, Field(ge=0, le=_MAX_RESOURCE_ID)] = 0


class _ArticleArgs(BaseModel):
    model_config: ClassVar[ConfigDict] = ConfigDict(extra="forbid", strict=True, frozen=True)
    article_id: Annotated[int, Field(ge=1, le=_MAX_RESOURCE_ID)]


class _SearchRuntimeArgs(_SearchArgs):
    # SDK validates the post-injection mapping. Keep the trusted collaborator
    # opaque here; _authorize checks the actual ToolRuntime and server context.
    runtime: Annotated[object, InjectedToolArg]


class _ArticleRuntimeArgs(_ArticleArgs):
    runtime: Annotated[object, InjectedToolArg]


def _schema(name: str) -> type[BaseModel]:
    if name == ReadToolName.SEARCH_ARTICLES:
        return _SearchArgs
    if name == ReadToolName.GET_ARTICLE:
        return _ArticleArgs
    raise ToolAccessDeniedError


class _ReadTool(StructuredTool):
    """SDK 子集 schema 不保留 extra 配置, 显式提供公开参数模型."""

    @property
    @override
    def tool_call_schema(self) -> type[BaseModel]:
        """返回无 SDK 控制字段、禁止额外参数的模型 schema."""
        return _schema(self.name)


class _ReadAuthorization(AgentMiddleware[AgentState[object], AgentRequestContext, object]):
    """即使 checkpoint 包含未注册工具, 也先拒绝再调用 handler."""

    _gateway: "BlogReadGateway"

    def __init__(self, gateway: "BlogReadGateway") -> None:
        """绑定应用拥有的授权网关."""
        self._gateway = gateway

    @override
    async def awrap_tool_call(
        self,
        request: ToolCallRequest,
        handler: Callable[[ToolCallRequest], Awaitable[ToolMessage | Command[object]]],
    ) -> ToolMessage | Command[object]:
        """在 SDK handler 注入工具参数之前验证原始调用."""
        self._gateway.validate_tool_call(request)
        return await handler(request)


class BlogReadGateway:
    """唯一只读工具适配器; 每次调用都重新检查服务器 Recipe 和上下文."""

    _client: BlogReadClient
    _registry: RecipeRegistry
    _tools: dict[ReadToolName, BaseTool]
    middleware: AgentMiddleware[AgentState[object], AgentRequestContext, object]

    def __init__(self, client: BlogReadClient, registry: RecipeRegistry) -> None:
        """构造固定只读工具, 不接管 Blog 客户端生命周期."""
        self._client = client
        self._registry = registry

        async def search_articles(
            runtime: ToolRuntime[AgentRequestContext, object], **arguments: object
        ) -> dict[str, object]:
            context = self._authorize(ReadToolName.SEARCH_ARTICLES, runtime)
            args = _SearchArgs.model_validate(arguments)
            command = SearchArticles(
                self._identity(context, runtime),
                BlogUserId(context.owner_user_id.value),
                args.query,
                args.page,
                args.page_size,
                args.status,
                args.article_type_id,
                args.tag_id,
                args.sort,
            )
            result = await self._invoke(ReadToolName.SEARCH_ARTICLES, command, context)
            if not isinstance(result, SearchSucceeded):
                raise BlogToolFailureError(retryable=False)
            return {
                "articles": [asdict(article) for article in result.articles],
                "page": result.page,
                "page_size": result.page_size,
                "total_items": result.total_items,
                "total_pages": result.total_pages,
            }

        async def get_article(
            article_id: int, runtime: ToolRuntime[AgentRequestContext, object]
        ) -> dict[str, object]:
            context = self._authorize(ReadToolName.GET_ARTICLE, runtime)
            command = GetArticle(
                self._identity(context, runtime),
                BlogUserId(context.owner_user_id.value),
                ArticleId(article_id),
            )
            result = await self._invoke(ReadToolName.GET_ARTICLE, command, context)
            if not isinstance(result, ArticleSucceeded) or result.article.article_id != article_id:
                raise BlogToolFailureError(retryable=False)
            return asdict(result.article)

        self._tools = {
            ReadToolName.SEARCH_ARTICLES: _ReadTool.from_function(
                coroutine=search_articles,
                name=ReadToolName.SEARCH_ARTICLES.value,
                description="Search authorized Blog management articles by filters and page.",
                args_schema=_SearchRuntimeArgs,
            ),
            ReadToolName.GET_ARTICLE: _ReadTool.from_function(
                coroutine=get_article,
                name=ReadToolName.GET_ARTICLE.value,
                description=(
                    "Read an authorized draft, published or archived article by numeric ID."
                ),
                args_schema=_ArticleRuntimeArgs,
            ),
        }
        self.middleware = _ReadAuthorization(self)

    def tools_for(self, recipe: AgentRecipe) -> tuple[BaseTool, ...]:
        """只接受注册表拥有的 Recipe 实例, 模型不能扩充目录."""
        if self._registry.resolve(recipe.recipe_id, recipe.version) is not recipe:
            raise ToolAccessDeniedError
        if any(
            type(name) is not ReadToolName or name not in READ_TOOL_CATALOG
            for name in recipe.tool_names
        ):
            raise ToolAccessDeniedError
        return tuple(self._tools[name] for name in recipe.tool_names)

    def validate_tool_call(self, request: object) -> None:
        """在执行前验证 SDK 原始调用, 包括未注册 checkpoint 工具."""
        if not isinstance(request, ToolCallRequest):
            raise ToolAccessDeniedError
        incoming = cast("_ToolRequestView", request)
        raw_name = incoming.tool_call.get("name")
        if not isinstance(raw_name, str):
            raise ToolAccessDeniedError
        try:
            name = ReadToolName(raw_name)
        except ValueError:
            raise ToolAccessDeniedError from None
        _ = self._authorize(name, incoming.runtime)
        runtime = cast("_RuntimeView", incoming.runtime)
        if incoming.tool_call.get("id") != runtime.tool_call_id:
            raise ToolAccessDeniedError
        if incoming.tool is None or incoming.tool is not self._tools.get(name):
            raise ToolAccessDeniedError
        schema = _schema(name)
        try:
            _ = schema.model_validate(incoming.tool_call.get("args"))
        except ValidationError:
            raise ToolAccessDeniedError from None

    def _authorize(self, name: object, runtime_object: object) -> AgentRequestContext:
        """从 SDK 注入值校验调用, 不使用 state 或模型参数."""
        if not isinstance(runtime_object, ToolRuntime):
            raise ToolAccessDeniedError
        runtime = cast("_RuntimeView", runtime_object)
        context = runtime.context
        if not isinstance(context, AgentRequestContext) or type(context) is not AgentRequestContext:
            raise ToolAccessDeniedError
        try:
            run_id = RunId(context.run_id)
            owner = UserId(context.owner_user_id.value)
        except (InvalidIdentifierError, ValueError, TypeError, AttributeError):
            raise ToolAccessDeniedError from None
        recipe = self._registry.resolve(context.recipe_id, context.recipe_version)
        configurable = runtime.config.get("configurable")
        if not isinstance(configurable, Mapping):
            raise ToolAccessDeniedError
        thread_id = cast("Mapping[str, object]", configurable).get("thread_id")
        call_id = runtime.tool_call_id
        if (
            type(cast("object", context.owner_user_id)) is not UserId
            or owner != context.owner_user_id
            or type(cast("object", context.quality)) is not Quality
            or recipe is None
            or recipe.capability is not context.capability
            or recipe.model_tier != context.quality.value
            or name not in READ_TOOL_CATALOG
            or name not in recipe.tool_names
            or thread_id != str(run_id)
            or not isinstance(call_id, str)
            or not call_id.strip()
        ):
            raise ToolAccessDeniedError
        return context

    @staticmethod
    def _identity(
        context: AgentRequestContext, runtime: ToolRuntime[AgentRequestContext, object]
    ) -> RequestIdentity:
        """Run 与 SDK 调用 ID 决定逻辑身份, 每次 RPC 独立请求身份."""
        call_id = runtime.tool_call_id
        if call_id is None:
            raise ToolAccessDeniedError
        if ToolCallId.pattern.fullmatch(call_id):
            logical_id = ToolCallId(call_id)
        else:
            digest = hashlib.sha256(f"{context.run_id}\0{call_id}".encode()).hexdigest()
            logical_id = ToolCallId(f"tool_{digest}")
        return RequestIdentity(
            RequestId(f"req_{uuid4().hex}"),
            CorrelationId(context.run_id),
            RunId(context.run_id),
            logical_id,
        )

    async def _invoke(
        self, name: ReadToolName, command: BlogCommand, context: AgentRequestContext
    ) -> BlogResult:
        """Pass synchronous Call creation to the worker-owned Run-lock fence."""
        try:
            result = await self._client.invoke(
                name.value, "v1", command, dispatch=context.execution_guard.dispatch
            )
        except (TimeoutError, OSError):
            raise BlogToolFailureError(retryable=True) from None
        except Exception:  # noqa: BLE001  # noqa: BROAD_EXCEPT_OK - 第三方端口边界禁止泄露原始异常。
            raise BlogToolFailureError(retryable=False) from None
        if isinstance(result, BlogFailure):
            raise BlogToolFailureError(retryable=result.retryable)
        return result
