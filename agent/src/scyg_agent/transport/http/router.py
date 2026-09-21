"""FastAPI 快照、SSE、命令与取消路由。."""

from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import UTC, datetime
from typing import Annotated, Final

from fastapi import APIRouter, HTTPException, Query, Request, Response, status
from fastapi.responses import StreamingResponse

from scyg_agent.application import (
    ApplicationFacade,
    CancelRequest,
    FacadeConflict,
    FacadeInternal,
    FacadeNotFound,
    FacadePrecondition,
    FacadeSuccess,
    FollowOpened,
    OwnedCommand,
    ReplaySuccess,
    SnapshotSuccess,
    SubmitInputRequest,
)
from scyg_agent.domain.ports.command_store import CommandSubmission
from scyg_agent.domain.ports.event_store import EventCursor, MalformedCursor, parse_event_cursor
from scyg_agent.domain.ports.idempotency import AuditMetadata, RequestDigest, ResultReference
from scyg_agent.domain.ports.interaction_store import InteractionResolution
from scyg_agent.domain.runs import CancelRun, CommandId, EventId, InteractionId, SubmitInput

from .auth import PrincipalVerifier, authorize_run
from .schemas import CommandRequest, InputRequest, MutationResponse, SnapshotResponse
from .sse import StreamPolicy, buffered_sse

REPLAY_HEADER: Final = "Idempotency-Replayed"
SSE_HEADERS: Final = {
    "Cache-Control": "no-cache",
    "Connection": "keep-alive",
    "X-Accel-Buffering": "no",
}
CursorQuery = Annotated[str | None, Query()]
MutationOutcome = (
    FacadeSuccess | FacadeNotFound | FacadeConflict | FacadePrecondition | FacadeInternal
)


@dataclass(frozen=True, slots=True)
class HTTPDependencies:
    """绑定 HTTP 路由所需的门面、验证器和时钟。."""

    facade: ApplicationFacade
    verifier: PrincipalVerifier
    now: Callable[[], datetime] = field(default=lambda: datetime.now(tz=UTC))
    stream_policy: StreamPolicy = field(default_factory=StreamPolicy)


def _error(code: int, detail: str) -> HTTPException:
    """构造不携带内部值的中文 HTTP 错误。."""
    return HTTPException(code, detail)


def map_mutation_outcome(result: MutationOutcome, response: Response) -> MutationResponse:
    """穷尽映射门面 mutation 结果并保留重放响应头。."""
    match result:  # noqa: RUF100  # noqa: MATCH_OK - 闭合门面结果已穷尽。
        case FacadeSuccess(replayed=replayed):
            response.headers[REPLAY_HEADER] = "true" if replayed else "false"
            return MutationResponse(replayed=replayed)
        case FacadeNotFound(message=message):
            raise _error(status.HTTP_404_NOT_FOUND, message)
        case FacadeConflict(message=message) | FacadePrecondition(message=message):
            raise _error(status.HTTP_409_CONFLICT, message)
        case FacadeInternal(message=message):
            raise _error(status.HTTP_500_INTERNAL_SERVER_ERROR, message)


def _cursor(request: Request, query: str | None) -> EventCursor:
    """解析非负游标并拒绝头与查询冲突。."""
    values = request.headers.getlist("last-event-id")
    if len(values) > 1:
        raise _error(status.HTTP_400_BAD_REQUEST, "事件游标请求头重复")
    header = values[0] if values else None
    parsed_query, parsed_header = parse_event_cursor(query), parse_event_cursor(header)
    if isinstance(parsed_query, MalformedCursor) or isinstance(parsed_header, MalformedCursor):
        raise _error(status.HTTP_400_BAD_REQUEST, "事件游标必须是非负整数")
    if query is not None and header is not None and parsed_query != parsed_header:
        raise _error(status.HTTP_400_BAD_REQUEST, "事件游标相互冲突")
    return parsed_header if header is not None else parsed_query


@dataclass(frozen=True, slots=True)
class HTTPHandlers:
    """实现固定路由并把业务处理委托给应用门面。."""

    dependencies: HTTPDependencies

    async def snapshot(self, request: Request, run_id: str) -> SnapshotResponse:
        """读取属于当前 Web 主体的 Run 快照。."""
        bound = authorize_run(request, run_id, self.dependencies.verifier)
        result = await self.dependencies.facade.get_snapshot(bound.owner, bound.run_id)
        match result:  # noqa: RUF100  # noqa: MATCH_OK - 闭合门面结果已穷尽。  # noqa: RUF100  # noqa: MATCH_OK - 闭合门面结果已穷尽。
            case SnapshotSuccess(run=run, cursor=cursor):
                pending = str(run.pending_interaction_id) if run.pending_interaction_id else None
                return SnapshotResponse(
                    run_id=str(run.id),
                    owner_user_id=str(run.owner_user_id),
                    task_type=run.task_type.value,
                    runtime_kind=run.runtime.kind.value,
                    runtime_version=run.runtime.version,
                    revision=run.revision,
                    status=run.status.value,
                    created_at=run.created_at,
                    updated_at=run.updated_at,
                    attempt=run.attempt,
                    pending_interaction_id=pending,
                    cursor=cursor.sequence,
                )
            case FacadeNotFound(message=message):
                raise _error(status.HTTP_404_NOT_FOUND, message)
            case FacadeInternal(message=message):
                raise _error(status.HTTP_500_INTERNAL_SERVER_ERROR, message)

    async def events(
        self, request: Request, run_id: str, cursor: CursorQuery = None
    ) -> StreamingResponse:
        """验证回放边界后跟随 T11 持久化事件。."""
        bound = authorize_run(request, run_id, self.dependencies.verifier)
        start = _cursor(request, cursor)
        checked = await self.dependencies.facade.replay_events(bound.owner, bound.run_id, start, 1)
        match checked:  # noqa: RUF100  # noqa: MATCH_OK - 闭合回放结果已穷尽。
            case ReplaySuccess():
                followed = await self.dependencies.facade.follow(bound.owner, bound.run_id, start)
            case FacadeNotFound(message=message):
                raise _error(status.HTTP_404_NOT_FOUND, message)
            case FacadeConflict(message=message):
                raise _error(status.HTTP_409_CONFLICT, message)
            case FacadeInternal(message=message):
                raise _error(status.HTTP_500_INTERNAL_SERVER_ERROR, message)
        match followed:  # noqa: RUF100  # noqa: MATCH_OK - 闭合跟随结果已穷尽。
            case FollowOpened(events=stream):
                body = buffered_sse(stream, self.dependencies.stream_policy)
                return StreamingResponse(body, media_type="text/event-stream", headers=SSE_HEADERS)
            case FacadeNotFound(message=message):
                raise _error(status.HTTP_404_NOT_FOUND, message)
            case FacadeInternal(message=message):
                raise _error(status.HTTP_500_INTERNAL_SERVER_ERROR, message)

    async def submit_input(
        self, request: Request, response: Response, run_id: str, body: InputRequest
    ) -> MutationResponse:
        """提交一次严格绑定交互的用户输入。."""
        bound = authorize_run(request, run_id, self.dependencies.verifier)
        interaction_id = InteractionId(body.interaction_id)
        digest = RequestDigest.parse(body.response_digest)
        command = SubmitInput(
            CommandId(body.command_id),
            EventId(body.event_id),
            body.expected_revision,
            body.occurred_at,
            interaction_id,
        )
        submission = CommandSubmission(
            command.command_id,
            bound.run_id,
            body.expected_revision,
            body.expected_sequence,
            "submit_input",
            digest,
            body.occurred_at,
            command,
            AuditMetadata("source", "http"),
        )
        resolution = InteractionResolution(
            interaction_id, digest, ResultReference(body.result_reference), submission
        )
        result = await self.dependencies.facade.submit_input(
            SubmitInputRequest(bound.owner, resolution)
        )
        return map_mutation_outcome(result, response)

    async def submit_command(
        self, request: Request, response: Response, run_id: str, body: CommandRequest
    ) -> MutationResponse:
        """提交公共取消命令且拒绝内部命令变体。."""
        bound = authorize_run(request, run_id, self.dependencies.verifier)
        command = CancelRun(
            CommandId(body.command_id),
            EventId(body.event_id),
            body.expected_revision,
            body.occurred_at,
        )
        submission = CommandSubmission(
            command.command_id,
            bound.run_id,
            body.expected_revision,
            body.expected_sequence,
            body.kind,
            RequestDigest.parse(body.request_digest),
            body.occurred_at,
            command,
            AuditMetadata("source", "http"),
        )
        result = await self.dependencies.facade.submit_command(
            OwnedCommand(bound.owner, submission)
        )
        return map_mutation_outcome(result, response)

    async def cancel(self, request: Request, response: Response, run_id: str) -> MutationResponse:
        """独立持久化取消意图,不依赖 SSE 生命周期。."""
        bound = authorize_run(request, run_id, self.dependencies.verifier)
        result = await self.dependencies.facade.cancel(
            CancelRequest(bound.owner, bound.run_id, self.dependencies.now())
        )
        return map_mutation_outcome(result, response)


def create_http_router(dependencies: HTTPDependencies) -> APIRouter:
    """创建不含 Run 创建入口的固定 Web 路由表。."""
    handlers = HTTPHandlers(dependencies)
    router = APIRouter(prefix="/api")
    router.add_api_route("/runs/{run_id}", handlers.snapshot, methods=["GET"])
    router.add_api_route("/runs/{run_id}/events", handlers.events, methods=["GET"])
    router.add_api_route("/runs/{run_id}/input", handlers.submit_input, methods=["POST"])
    router.add_api_route("/runs/{run_id}/commands", handlers.submit_command, methods=["POST"])
    router.add_api_route("/runs/{run_id}/cancel", handlers.cancel, methods=["POST"])
    return router
