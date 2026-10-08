"""Strict transport validation and public snapshot serialization."""

import json
import re
from datetime import UTC, datetime
from typing import TYPE_CHECKING, cast

from scyg_agent.agents.contracts import Capability
from scyg_agent.generated.proto.scyg.agent.v1 import common_pb2

if TYPE_CHECKING:
    from scyg_agent.application.control import ControlSnapshot


class InvalidGrpcRequestError(ValueError):
    """A request violates the public transport contract."""


MAX_ID_BYTES = 128
MAX_CURSOR_BYTES = 256
MAX_JSON_BYTES = 1_048_576

CAPABILITIES: dict[int, Capability] = {
    common_pb2.AGENT_CAPABILITY_SEARCH: Capability.SEARCH,
    common_pb2.AGENT_CAPABILITY_WRITE: Capability.WRITE,
    common_pb2.AGENT_CAPABILITY_POLISH: Capability.POLISH,
    common_pb2.AGENT_CAPABILITY_CHAT: Capability.CHAT,
}
CAPABILITY_PROTO = {
    Capability.SEARCH.value: common_pb2.AGENT_CAPABILITY_SEARCH,
    Capability.WRITE.value: common_pb2.AGENT_CAPABILITY_WRITE,
    Capability.POLISH.value: common_pb2.AGENT_CAPABILITY_POLISH,
    Capability.CHAT.value: common_pb2.AGENT_CAPABILITY_CHAT,
}
STATUS_PROTO = {
    "pending": common_pb2.RUN_STATUS_QUEUED,
    "pending_resume": common_pb2.RUN_STATUS_QUEUED,
    "queued": common_pb2.RUN_STATUS_QUEUED,
    "running": common_pb2.RUN_STATUS_RUNNING,
    "waiting_input": common_pb2.RUN_STATUS_WAITING_FOR_APPROVAL,
    "waiting_for_approval": common_pb2.RUN_STATUS_WAITING_FOR_APPROVAL,
    "succeeded": common_pb2.RUN_STATUS_SUCCEEDED,
    "failed": common_pb2.RUN_STATUS_FAILED,
    "cancelled": common_pb2.RUN_STATUS_CANCELLED,
}
INTERACTION_PROTO = {
    "confirmation": common_pb2.INTERACTION_TYPE_CONFIRMATION,
    "selection": common_pb2.INTERACTION_TYPE_SELECTION,
    "text_input": common_pb2.INTERACTION_TYPE_TEXT_INPUT,
}
_RUN_ID = re.compile(r"[A-Za-z0-9._~-]{1,128}")
_KEY = re.compile(
    r"[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}",
    flags=re.IGNORECASE | re.ASCII,
)


def parse_capability(value: int) -> Capability:
    """Accept exactly the four externally selectable capability enum values."""
    try:
        return CAPABILITIES[value]
    except KeyError:
        raise InvalidGrpcRequestError from None


def validate_user(value: str) -> str:
    """Validate the opaque owner identity's UTF-8 length."""
    if not 1 <= len(value.encode("utf-8")) <= MAX_ID_BYTES:
        raise InvalidGrpcRequestError
    return value


def validate_run_id(value: str) -> str:
    """Validate the public Run ID grammar without imposing generated prefixes."""
    if _RUN_ID.fullmatch(value) is None or value in {".", ".."}:
        raise InvalidGrpcRequestError
    return value


def validate_key(value: str) -> str:
    """Require the UUIDv4 idempotency-key syntax."""
    if _KEY.fullmatch(value) is None:
        raise InvalidGrpcRequestError
    return value


def validate_interaction(value: str) -> str:
    """Validate an opaque interaction identity without exposing internal prefixes."""
    if not 1 <= len(value.encode("utf-8")) <= MAX_ID_BYTES:
        raise InvalidGrpcRequestError
    return value


def validate_cursor(value: str | None) -> str | None:
    """Keep optional cursors opaque while bounding their transport representation."""
    if value is not None and (not 1 <= len(value) <= MAX_CURSOR_BYTES or not value.isascii()):
        raise InvalidGrpcRequestError
    return value


def _invalid_constant(_value: str) -> None:
    raise InvalidGrpcRequestError


def validate_json(value: bytes) -> bytes:
    """Validate syntax only; keep original bytes and defer business interpretation."""
    if not 1 <= len(value) <= MAX_JSON_BYTES:
        raise InvalidGrpcRequestError
    try:
        _ = cast("object", json.loads(value.decode("utf-8"), parse_constant=_invalid_constant))
    except (ValueError, UnicodeError, RecursionError):
        raise InvalidGrpcRequestError from None
    return value


def _json_bytes(value: object) -> bytes:
    return json.dumps(value, ensure_ascii=False, allow_nan=False, separators=(",", ":")).encode(
        "utf-8"
    )


def _timestamp(value: datetime) -> str:
    return value.astimezone(UTC).isoformat().replace("+00:00", "Z")


def run_to_proto(snapshot: "ControlSnapshot") -> common_pb2.Run:
    """Serialize one consistent application snapshot, preserving optional JSON."""
    run = common_pb2.Run(
        run_id=snapshot.run_id,
        status=STATUS_PROTO[snapshot.status],
        capability=CAPABILITY_PROTO[snapshot.capability],
        recipe_id=snapshot.recipe_id,
        recipe_version=snapshot.recipe_version,
        created_at=_timestamp(snapshot.created_at),
        updated_at=_timestamp(snapshot.updated_at),
    )
    if snapshot.failure_code is not None:
        run.failure.CopyFrom(
            common_pb2.PublicError(
                code=snapshot.failure_code,
                message=snapshot.failure_message or "Run failed",
            )
        )
    if snapshot.result_present:
        run.result_json = _json_bytes(snapshot.result)
    if snapshot.interaction is not None:
        interaction = snapshot.interaction
        run.pending_interaction.CopyFrom(
            common_pb2.PendingInteraction(
                interaction_id=interaction.interaction_id,
                type=INTERACTION_PROTO[interaction.kind],
                payload_json=_json_bytes(interaction.payload),
            )
        )
    return run
