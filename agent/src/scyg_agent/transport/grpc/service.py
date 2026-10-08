"""Internal five-RPC control plane; browser authentication belongs to Blog."""

from collections.abc import AsyncIterator, Awaitable, Callable
from typing import Final, NoReturn, Protocol, TypeVar, final, override

import anyio
import grpc
from google.protobuf.any_pb2 import Any
from google.rpc import status_pb2
from grpc_status import rpc_status

from scyg_agent.adapters.database.event_subscription import SubscriptionCursorError
from scyg_agent.agents.contracts import Capability
from scyg_agent.application.control import ControlError, ControlSnapshot, ResumeCommand
from scyg_agent.generated.proto.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.proto.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc
from scyg_agent.generated.proto.scyg.agent.v1 import common_pb2

from .conversion import (
    InvalidGrpcRequestError,
    parse_capability,
    run_to_proto,
    validate_cursor,
    validate_interaction,
    validate_json,
    validate_key,
    validate_run_id,
    validate_user,
)

T = TypeVar("T")
_CURSOR_REJECTED: Final = ("FAILED_PRECONDITION", "Event cursor is outside the retained journal")


class Context(Protocol):
    """Expose the actual grpc.aio lifecycle methods used across RPC message types."""

    def time_remaining(self) -> float | None:
        """Return the remaining deadline, or None when no deadline was specified."""
        ...

    async def send_initial_metadata(
        self, initial_metadata: tuple[tuple[str, str | bytes], ...]
    ) -> None:
        """Send the subscription readiness metadata before yielding frames."""
        ...

    async def abort(
        self,
        code: grpc.StatusCode,
        details: str = "",
        trailing_metadata: tuple[tuple[str, str | bytes], ...] = (),
    ) -> NoReturn:
        """Terminate the RPC with the canonical serialized rich-status metadata."""
        ...


class OpenedFrames(Protocol):
    """An already registered subscription whose resources can be released."""

    def frames(self) -> AsyncIterator[str]:
        """Iterate complete frames from an already registered subscription."""
        ...

    async def aclose(self) -> None:
        """Release the subscription, including before iteration begins."""
        ...


class ControlFacade(Protocol):
    """The complete control application contract, without transport credentials."""

    async def create(
        self, user_id: str, key: str, capability: Capability, raw_json: bytes
    ) -> ControlSnapshot:
        """Create or replay a Run in one successful-key transaction."""
        ...

    async def get(self, user_id: str, run_id: str) -> ControlSnapshot:
        """Read one owner-authorized public snapshot."""
        ...

    async def resume(self, user_id: str, key: str, command: ResumeCommand) -> ControlSnapshot:
        """Resolve the current interaction or replay a successful key."""
        ...

    async def cancel(self, user_id: str, key: str, run_id: str) -> ControlSnapshot:
        """Set cancellation admission fences or replay a successful key."""
        ...

    async def open_events(self, user_id: str, run_id: str, cursor: str | None) -> OpenedFrames:
        """Authorize and open a cursor-validated subscription before readiness."""
        ...


@final
class AgentControlServicer(service_grpc.AgentControlServiceServicer):
    """Validate transport syntax, then delegate transactional business behavior."""

    def __init__(self, facade: ControlFacade) -> None:
        """Bind only the shared owner-authorized application facade."""
        self._facade = facade

    async def _invoke(self, context: Context, call: Callable[[], Awaitable[T]]) -> T:
        try:
            remaining = context.time_remaining()
            if remaining is None:
                return await call()
            with anyio.fail_after(remaining):
                return await call()
        except (InvalidGrpcRequestError, UnicodeError):
            await _abort(context, grpc.StatusCode.INVALID_ARGUMENT, "Invalid request")
        except ControlError as error:
            await _abort(context, grpc.StatusCode[error.code], error.message)
        except TimeoutError:
            await _abort(context, grpc.StatusCode.DEADLINE_EXCEEDED, "Request deadline exceeded")
        except Exception:  # noqa: BLE001  # BROAD_EXCEPT_OK: sanitize all infrastructure failures at the public boundary.
            await _abort(context, grpc.StatusCode.INTERNAL, "Internal service error")

    @override
    async def CreateRun(
        self, request: service_pb2.CreateRunRequest, context: Context
    ) -> common_pb2.Run:
        """Preserve raw JSON and leave business parsing after key replay."""

        async def call() -> common_pb2.Run:
            return run_to_proto(
                await self._facade.create(
                    validate_user(request.user_id),
                    validate_key(request.idempotency_key),
                    parse_capability(request.capability),
                    validate_json(request.json_payload),
                )
            )

        return await self._invoke(context, call)

    @override
    async def GetRun(self, request: service_pb2.GetRunRequest, context: Context) -> common_pb2.Run:
        """Return one owner-authorized consistent snapshot."""

        async def call() -> common_pb2.Run:
            return run_to_proto(
                await self._facade.get(
                    validate_user(request.user_id),
                    validate_run_id(request.run_id),
                )
            )

        return await self._invoke(context, call)

    @override
    async def ResumeRun(
        self, request: service_pb2.ResumeRunRequest, context: Context
    ) -> common_pb2.Run:
        """Preserve omitted payload separately from explicit JSON null."""

        async def call() -> common_pb2.Run:
            if not request.decision:
                raise InvalidGrpcRequestError
            payload = (
                validate_json(request.payload_json) if request.HasField("payload_json") else None
            )
            return run_to_proto(
                await self._facade.resume(
                    validate_user(request.user_id),
                    validate_key(request.idempotency_key),
                    ResumeCommand(
                        validate_run_id(request.run_id),
                        validate_interaction(request.interaction_id),
                        request.decision,
                        payload,
                    ),
                )
            )

        return await self._invoke(context, call)

    @override
    async def CancelRun(
        self, request: service_pb2.CancelRunRequest, context: Context
    ) -> common_pb2.Run:
        """Delegate atomic cancellation and successful-key replay."""

        async def call() -> common_pb2.Run:
            return run_to_proto(
                await self._facade.cancel(
                    validate_user(request.user_id),
                    validate_key(request.idempotency_key),
                    validate_run_id(request.run_id),
                )
            )

        return await self._invoke(context, call)

    @override
    async def StreamRunEvents(
        self, request: service_pb2.StreamRunEventsRequest, context: Context
    ) -> AsyncIterator[service_pb2.RunEventFrame]:
        """Signal readiness only after open; relay complete frames and heartbeats."""

        async def open_stream() -> OpenedFrames:
            user = validate_user(request.user_id)
            run_id = validate_run_id(request.run_id)
            cursor = validate_cursor(
                request.after_event_id if request.HasField("after_event_id") else None
            )
            try:
                return await self._facade.open_events(user, run_id, cursor)
            except ValueError:
                raise InvalidGrpcRequestError from None
            except SubscriptionCursorError:
                raise ControlError(*_CURSOR_REJECTED) from None

        opened = await self._invoke(context, open_stream)
        try:
            await context.send_initial_metadata((("scyg-subscription-ready", "1"),))
            async for frame in opened.frames():
                yield service_pb2.RunEventFrame(frame=frame.encode("utf-8"))
        except ControlError as error:
            await _abort(context, grpc.StatusCode[error.code], error.message)
        except Exception:  # noqa: BLE001  # BROAD_EXCEPT_OK: terminate the public stream without exposing internals.
            await _abort(context, grpc.StatusCode.INTERNAL, "Internal service error")
        finally:
            with anyio.CancelScope(shield=True):
                await opened.aclose()


async def _abort(context: Context, code: grpc.StatusCode, message: str) -> NoReturn:
    """Pack a sanitized canonical public status detail for the Blog gateway."""
    public_error = common_pb2.PublicError(code=code.name, message=message)
    detail = Any(
        type_url="type.googleapis.com/scyg.agent.v1.PublicError",
        value=public_error.SerializeToString(),
    )
    status = rpc_status.to_status(
        status_pb2.Status(
            code=code.value[0],
            message=message,
            details=[detail],
        )
    )
    await context.abort(status.code, status.details, status.trailing_metadata)
