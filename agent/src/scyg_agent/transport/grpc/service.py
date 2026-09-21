"""Implement legacy and capability-based AgentControl unary RPCs."""

from collections.abc import Awaitable
from datetime import UTC, datetime
from typing import NoReturn, Protocol, final, override

import anyio
import grpc
from grpc import aio

from scyg_agent.adapters.auth import JwtVerifier
from scyg_agent.agents.contracts import (
    INPUT_SCHEMA_VERSION,
    Capability,
    CapabilityInput,
    ChatInput,
    PolishInput,
    SearchInput,
    WritingInput,
    input_digest,
    input_payload,
    recipe_for_capability,
)
from scyg_agent.application import (
    CreateRunInput,
    FacadeCancelled,
    FacadeConflict,
    FacadeInternal,
    FacadeNotFound,
    FacadePrecondition,
    FacadeSuccess,
    FacadeValidation,
    OwnerContext,
    SnapshotSuccess,
)
from scyg_agent.domain.runs import (
    InvalidEnumValueError,
    InvalidIdentifierError,
    InvalidRuntimeVersionError,
    Run,
    RunId,
    RuntimeKind,
    RuntimeSelection,
    TaskType,
)
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc

from .authentication import authenticate_blog_service
from .conversion import (
    InvalidGrpcRequestError,
    parse_create_agent_run_request,
    parse_create_request,
    parse_get_request,
    run_to_proto,
)


class AgentControlApplications(Protocol):
    """声明两个控制面 RPC 使用的完成门面能力."""

    async def create_run(
        self, request: CreateRunInput
    ) -> (
        FacadeSuccess
        | FacadeConflict
        | FacadePrecondition
        | FacadeValidation
        | FacadeCancelled
        | FacadeInternal
    ):
        """创建或重放 Run."""
        ...

    async def get_snapshot(
        self, owner: OwnerContext, run_id: RunId
    ) -> (
        SnapshotSuccess
        | FacadeNotFound
        | FacadePrecondition
        | FacadeValidation
        | FacadeCancelled
        | FacadeInternal
    ):
        """读取稳定 Run 快照."""
        ...


class Clock(Protocol):
    """提供创建请求使用的可注入 UTC 时钟."""

    def now(self) -> datetime:
        """返回当前 UTC 时刻."""
        ...


class UtcClock:
    """提供生产环境 UTC 当前时间."""

    def now(self) -> datetime:
        """返回时区明确的当前时间."""
        return datetime.now(tz=UTC)


@final
class AgentControlServicer(service_grpc.AgentControlServiceServicer):
    """认证 Blog 服务并把两个控制面 RPC 委托给共享门面."""

    def __init__(
        self, facade: AgentControlApplications, verifier: JwtVerifier, clock: Clock | None = None
    ) -> None:
        """绑定完成的应用门面、JWT 验证器与可测试时钟."""
        self._facade = facade
        self._verifier = verifier
        self._clock = clock or UtcClock()

    @override
    async def CreateRun(
        self,
        request: service_pb2.CreateRunRequest,
        context: aio.ServicerContext[service_pb2.CreateRunRequest, service_pb2.CreateRunResponse],
    ) -> service_pb2.CreateRunResponse:
        """认证、解析并执行一次规范幂等 Run 创建."""
        _ = await authenticate_blog_service(context, self._verifier)
        await _require_live_context(context)
        try:
            owner, operation_id, run_id, task_type, runtime, article_id, message = (
                parse_create_request(request)
            )
            create = CreateRunInput(
                OwnerContext(owner),
                operation_id,
                run_id,
                task_type,
                runtime,
                article_id,
                message,
                self._clock.now(),
            )
        except (
            InvalidGrpcRequestError,
            InvalidIdentifierError,
            InvalidEnumValueError,
            InvalidRuntimeVersionError,
            ValueError,
        ):
            await _abort(context, grpc.StatusCode.INVALID_ARGUMENT, "创建 Run 的请求参数无效")
        try:
            result = await _within_deadline(context, self._facade.create_run(create))
        except TimeoutError:
            await _abort(context, grpc.StatusCode.DEADLINE_EXCEEDED, "请求处理超过截止时间")
        except Exception:  # noqa: BLE001  # noqa: BROAD_EXCEPT_OK - 传输最外层必须隐藏未知基础设施异常。
            await _abort(context, grpc.StatusCode.INTERNAL, "服务内部错误")
        return await _create_outcome(context, result, article_id)

    @override
    async def CreateAgentRun(
        self,
        request: service_pb2.CreateAgentRunRequest,
        context: aio.ServicerContext[
            service_pb2.CreateAgentRunRequest, service_pb2.CreateAgentRunResponse
        ],
    ) -> service_pb2.CreateAgentRunResponse:
        """Authenticate and adapt the public capability request to the current Run facade."""
        _ = await authenticate_blog_service(context, self._verifier)
        await _require_live_context(context)
        try:
            owner, operation, run_id, capability, value, locale = parse_create_agent_run_request(
                request
            )
            task_type, runtime, message = _legacy_route(capability, value)
            create = CreateRunInput(
                OwnerContext(owner),
                operation,
                run_id,
                task_type,
                runtime,
                "",
                message,
                self._clock.now(),
                capability=capability.value,
                recipe_id=recipe_for_capability(capability).value,
                recipe_version="v1",
                input_schema_version=INPUT_SCHEMA_VERSION,
                input_payload=input_payload(value),
                input_digest=input_digest(value),
                locale=locale,
            )
        except (InvalidGrpcRequestError, InvalidIdentifierError, InvalidEnumValueError, ValueError):
            await _abort(context, grpc.StatusCode.INVALID_ARGUMENT, "创建 Agent Run 的请求参数无效")
        try:
            result = await _within_deadline(context, self._facade.create_run(create))
        except TimeoutError:
            await _abort(context, grpc.StatusCode.DEADLINE_EXCEEDED, "请求处理超过截止时间")
        except Exception:  # noqa: BLE001
            await _abort(context, grpc.StatusCode.INTERNAL, "服务内部错误")
        return await _create_agent_outcome(context, result)

    @override
    async def GetRun(
        self,
        request: service_pb2.GetRunRequest,
        context: aio.ServicerContext[service_pb2.GetRunRequest, service_pb2.GetRunResponse],
    ) -> service_pb2.GetRunResponse:
        """认证、解析并返回不含内部执行字段的稳定 Run 快照."""
        _ = await authenticate_blog_service(context, self._verifier)
        await _require_live_context(context)
        try:
            owner, run_id = parse_get_request(request)
        except (
            InvalidGrpcRequestError,
            InvalidIdentifierError,
            InvalidEnumValueError,
            InvalidRuntimeVersionError,
            ValueError,
        ):
            await _abort(context, grpc.StatusCode.INVALID_ARGUMENT, "读取 Run 的请求参数无效")
        try:
            result = await _within_deadline(
                context, self._facade.get_snapshot(OwnerContext(owner), run_id)
            )
        except TimeoutError:
            await _abort(context, grpc.StatusCode.DEADLINE_EXCEEDED, "请求处理超过截止时间")
        except Exception:  # noqa: BLE001  # noqa: BROAD_EXCEPT_OK - 传输最外层必须隐藏未知基础设施异常。
            await _abort(context, grpc.StatusCode.INTERNAL, "服务内部错误")
        return await _get_outcome(context, result)


async def _create_outcome(
    context: aio.ServicerContext[service_pb2.CreateRunRequest, service_pb2.CreateRunResponse],
    result: FacadeSuccess
    | FacadeConflict
    | FacadePrecondition
    | FacadeValidation
    | FacadeCancelled
    | FacadeInternal,
    article_id: str,
) -> service_pb2.CreateRunResponse:
    """映射创建门面闭合结果为稳定 RPC 结果."""
    match result:
        case FacadeSuccess(value=Run() as run):
            response_run = run_to_proto(run)
            if article_id:
                response_run.article_id.value = article_id
            return service_pb2.CreateRunResponse(run=response_run)
        case FacadeConflict():
            return await _abort(context, grpc.StatusCode.ALREADY_EXISTS, "创建请求与已有事实冲突")
        case FacadePrecondition():
            return await _abort(
                context, grpc.StatusCode.FAILED_PRECONDITION, "Run 当前状态不允许此操作"
            )
        case FacadeValidation():
            return await _abort(context, grpc.StatusCode.INVALID_ARGUMENT, "请求参数无效")
        case FacadeCancelled():
            return await _abort(context, grpc.StatusCode.CANCELLED, "请求已取消")
        case FacadeInternal() | FacadeSuccess():
            return await _abort(context, grpc.StatusCode.INTERNAL, "服务内部错误")


def _legacy_route(
    capability: Capability, value: CapabilityInput
) -> tuple[TaskType, RuntimeSelection, str]:
    """Resolve server-owned legacy fields until the P2 Run cutover."""
    if capability is Capability.SEARCH and isinstance(value, SearchInput):
        return TaskType.RESEARCH, RuntimeSelection(RuntimeKind.DEEP, "v1"), value.query
    if capability is Capability.WRITE and isinstance(value, WritingInput):
        return TaskType.COMPOSE, RuntimeSelection(RuntimeKind.DEEP, "v1"), value.topic
    if capability is Capability.POLISH and isinstance(value, PolishInput):
        return TaskType.POLISH, RuntimeSelection(RuntimeKind.SIMPLE, "v1"), value.content
    if capability is Capability.CHAT and isinstance(value, ChatInput):
        return TaskType.QUESTION, RuntimeSelection(RuntimeKind.SIMPLE, "v1"), value.message
    raise InvalidGrpcRequestError


async def _create_agent_outcome(
    context: aio.ServicerContext[
        service_pb2.CreateAgentRunRequest, service_pb2.CreateAgentRunResponse
    ],
    result: FacadeSuccess
    | FacadeConflict
    | FacadePrecondition
    | FacadeValidation
    | FacadeCancelled
    | FacadeInternal,
) -> service_pb2.CreateAgentRunResponse:
    """Map the shared facade result to the capability-specific response."""
    match result:
        case FacadeSuccess(value=Run() as run):
            return service_pb2.CreateAgentRunResponse(run=run_to_proto(run))
        case FacadeConflict():
            return await _abort(context, grpc.StatusCode.ALREADY_EXISTS, "创建请求与已有事实冲突")
        case FacadePrecondition():
            return await _abort(
                context, grpc.StatusCode.FAILED_PRECONDITION, "Run 当前状态不允许此操作"
            )
        case FacadeValidation():
            return await _abort(context, grpc.StatusCode.INVALID_ARGUMENT, "请求参数无效")
        case FacadeCancelled():
            return await _abort(context, grpc.StatusCode.CANCELLED, "请求已取消")
        case FacadeInternal() | FacadeSuccess():
            return await _abort(context, grpc.StatusCode.INTERNAL, "服务内部错误")


async def _get_outcome(
    context: aio.ServicerContext[service_pb2.GetRunRequest, service_pb2.GetRunResponse],
    result: SnapshotSuccess
    | FacadeNotFound
    | FacadePrecondition
    | FacadeValidation
    | FacadeCancelled
    | FacadeInternal,
) -> service_pb2.GetRunResponse:
    """映射读取门面闭合结果为稳定 RPC 结果."""
    match result:  # noqa: RUF100  # noqa: MATCH_OK - 门面闭合结果已完整映射。
        case SnapshotSuccess(run=run):
            return service_pb2.GetRunResponse(run=run_to_proto(run))
        case FacadeNotFound():
            return await _abort(context, grpc.StatusCode.NOT_FOUND, "未找到 Run")
        case FacadePrecondition():
            return await _abort(
                context, grpc.StatusCode.FAILED_PRECONDITION, "Run 当前状态不允许此操作"
            )
        case FacadeValidation():
            return await _abort(context, grpc.StatusCode.INVALID_ARGUMENT, "请求参数无效")
        case FacadeCancelled():
            return await _abort(context, grpc.StatusCode.CANCELLED, "请求已取消")
        case FacadeInternal():
            return await _abort(context, grpc.StatusCode.INTERNAL, "服务内部错误")


async def _require_live_context[RequestT, ResponseT](
    context: aio.ServicerContext[RequestT, ResponseT],
) -> None:
    """在任何数据库操作前拒绝已取消或已到期调用."""
    remaining = _time_remaining(context)
    if context.cancelled():
        await _abort(context, grpc.StatusCode.CANCELLED, "请求已取消")
    if remaining is not None and remaining <= 0:
        await _abort(context, grpc.StatusCode.DEADLINE_EXCEEDED, "请求处理超过截止时间")


async def _within_deadline[T, RequestT, ResponseT](
    context: aio.ServicerContext[RequestT, ResponseT], operation: Awaitable[T]
) -> T:
    """以客户端剩余截止期约束门面协程并传播外部取消."""
    remaining = _time_remaining(context)
    if remaining is None:
        return await operation
    with anyio.fail_after(remaining):
        return await operation


def _time_remaining[RequestT, ResponseT](
    context: aio.ServicerContext[RequestT, ResponseT],
) -> float | None:
    """兼容 grpc.aio 无截止期时的运行时 None."""
    return context.time_remaining()


async def _abort[RequestT, ResponseT](
    context: aio.ServicerContext[RequestT, ResponseT],
    code: grpc.StatusCode,
    detail: str,
) -> NoReturn:
    """以稳定公开状态终止 RPC."""
    await context.abort(code, detail)
