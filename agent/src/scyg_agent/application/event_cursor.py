"""Run-bound, versioned opaque cursors for durable event subscriptions."""

import base64
import binascii
import json
import re
from typing import cast

from scyg_agent.domain.ports.event_store import EventCursor
from scyg_agent.domain.runs import RunId

MAX_CURSOR_LENGTH = 256
_CURSOR_PATTERN = re.compile(r"[A-Za-z0-9_-]+")


class InvalidEventCursorError(ValueError):
    """Reject malformed or cross-Run cursors without reflecting their contents."""


def encode_event_cursor(run_id: RunId, cursor: EventCursor) -> str:
    """Encode an unpadded base64url cursor scoped to one Run."""
    data = json.dumps(
        {"v": 1, "runId": str(run_id), "seq": cursor.sequence}, separators=(",", ":")
    ).encode("utf-8")
    encoded = base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")
    if len(encoded) > MAX_CURSOR_LENGTH:
        raise InvalidEventCursorError
    return encoded


def decode_event_cursor(run_id: RunId, raw: str | None) -> EventCursor | None:
    """Reject malformed, unversioned, negative or cross-Run cursors before opening."""
    if raw is None:
        return None
    if len(raw) > MAX_CURSOR_LENGTH or _CURSOR_PATTERN.fullmatch(raw) is None:
        raise InvalidEventCursorError
    try:
        data = base64.b64decode(raw + "=" * (-len(raw) % 4), altchars=b"-_", validate=True)
        value = cast("object", json.loads(data))
    except (ValueError, UnicodeDecodeError, binascii.Error) as error:
        raise InvalidEventCursorError from error
    if not isinstance(value, dict):
        raise InvalidEventCursorError
    fields = cast("dict[str, object]", value)
    version, sequence = fields.get("v"), fields.get("seq")
    if (
        set(fields) != {"v", "runId", "seq"}
        or type(version) is not int
        or version != 1
        or fields["runId"] != str(run_id)
        or type(sequence) is not int
        or sequence < 0
    ):
        raise InvalidEventCursorError
    cursor = EventCursor(sequence)
    if encode_event_cursor(run_id, cursor) != raw:
        raise InvalidEventCursorError
    return cursor
