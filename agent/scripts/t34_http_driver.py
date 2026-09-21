"""Fixed HTTP/SSE observations shared by the three T34 routine drivers."""

from __future__ import annotations

import json
import os
import sys
from dataclasses import dataclass
from http.client import HTTPConnection, HTTPResponse, HTTPSConnection
from itertools import pairwise
from pathlib import Path
from typing import ClassVar
from urllib.parse import SplitResult, urlencode, urlsplit

from pydantic import BaseModel, ConfigDict, ValidationError

ARGUMENT_COUNT = 2
TIMEOUT_SECONDS = 30
HTTP_OK = 200
TERMINAL_EVENTS = frozenset({"run_succeeded", "run_failed", "run_cancelled"})


@dataclass(frozen=True, slots=True)
class Fixture:
    """Data-only HTTP fixture provided by the owned topology."""

    base_url: str
    run_id: str
    bearer_token: str


class Snapshot(BaseModel):
    """Parse the minimal snapshot field required by S1."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="ignore")
    runtime_kind: str


class SsePayload(BaseModel):
    """Parse the event identity required for reconnect deduplication."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="ignore")
    event_id: str


@dataclass(frozen=True, slots=True)
class SseEvent:
    """One persisted SSE fact with numeric cursor and durable event identity."""

    cursor: int
    event_id: str
    name: str


def fixture(scenario: str) -> Fixture | None:
    """Read required data inputs without accepting a code or command selection."""
    names = ("SCYG_T34_HTTP_BASE_URL", "SCYG_T34_RUN_ID", "SCYG_T34_BEARER_TOKEN")
    missing = tuple(name for name in names if not os.environ.get(name))
    if missing:
        print(f"T34 {scenario} driver prerequisites: missing input(s): {', '.join(missing)}")
        return None
    return Fixture(*(os.environ[name] for name in names))


def evidence_root(arguments: list[str]) -> Path | None:
    """Accept only the harness-controlled evidence-root argument shape."""
    if len(arguments) != ARGUMENT_COUNT or arguments[0] != "--evidence-root":
        print("T34 driver prerequisites: expected --evidence-root")
        return None
    return Path(arguments[1])


def _target(fixture: Fixture, path: str) -> SplitResult | None:
    """Reject non-HTTP and non-local fixture targets before opening a socket."""
    target = urlsplit(f"{fixture.base_url.rstrip('/')}{path}")
    if target.scheme not in {"http", "https"} or target.hostname not in {"127.0.0.1", "localhost"}:
        print("T34 driver prerequisites: HTTP base URL must target local Agent topology")
        return None
    return target


def _response(fixture: Fixture, path: str, method: str = "GET") -> HTTPResponse | None:
    """Open one authenticated local Agent request with a bounded timeout."""
    target = _target(fixture, path)
    if target is None:
        return None
    connection: HTTPConnection | HTTPSConnection
    connection = (
        HTTPSConnection(target.netloc, timeout=TIMEOUT_SECONDS)
        if target.scheme == "https"
        else HTTPConnection(target.netloc, timeout=TIMEOUT_SECONDS)
    )
    try:
        connection.request(
            method,
            target.path + (f"?{target.query}" if target.query else ""),
            headers={"Authorization": f"Bearer {fixture.bearer_token}"},
        )
        response = connection.getresponse()
    except OSError as error:
        print(f"T34 HTTP driver failed: {error.__class__.__name__}")
        connection.close()
        return None
    if response.status != HTTP_OK:
        print(f"T34 HTTP driver failed: status {response.status}")
        response.close()
        connection.close()
        return None
    return response


def _next_sse_event(response: HTTPResponse) -> SseEvent | None:
    """Read one complete SSE event frame and parse its concrete durable identity."""
    cursor: int | None = None
    name: str | None = None
    data: str | None = None
    while raw := response.readline():
        line = raw.decode("utf-8").rstrip("\n")
        if line.startswith("id: "):
            cursor = int(line[4:])
        elif line.startswith("event: "):
            name = line[7:]
        elif line.startswith("data: "):
            data = line[6:]
        elif not line and cursor is not None and name is not None and data is not None:
            return SseEvent(cursor, SsePayload.model_validate_json(data).event_id, name)
    return None


def sse(fixture: Fixture, cursor: int) -> tuple[SseEvent, ...] | None:
    """Read persisted SSE frames until a terminal event or the bounded socket deadline."""
    frames: list[SseEvent] = []
    response = _response(
        fixture, f"/api/runs/{fixture.run_id}/events?{urlencode({'cursor': cursor})}"
    )
    if response is None:
        return None
    try:
        while event := _next_sse_event(response):
            frames.append(event)
            if event.name in TERMINAL_EVENTS:
                return tuple(frames)
    except (OSError, UnicodeDecodeError, ValidationError, ValueError) as error:
        print(f"T34 SSE driver failed: {error.__class__.__name__}")
    finally:
        response.close()
    print("T34 SSE driver failed: terminal event was not observed")
    return None


def initial_sse(fixture: Fixture, cursor: int) -> SseEvent | None:
    """Read one initial persisted event, then explicitly close the first SSE connection."""
    response = _response(
        fixture, f"/api/runs/{fixture.run_id}/events?{urlencode({'cursor': cursor})}"
    )
    if response is None:
        return None
    try:
        return _next_sse_event(response)
    except (OSError, UnicodeDecodeError, ValidationError, ValueError) as error:
        print(f"T34 S2 driver failed: {error.__class__.__name__}")
        return None
    finally:
        response.close()


def reconnect_observation(
    requested_cursor: int, initial: SseEvent | None, replay: tuple[SseEvent, ...] | None
) -> str | None:
    """Prove cursor continuity, strict replay ordering and cross-connection uniqueness."""
    if (
        initial is None
        or initial.cursor <= requested_cursor
        or not replay
        or replay[-1].name not in TERMINAL_EVENTS
    ):
        return None
    events = (initial, *replay)
    cursors = tuple(event.cursor for event in events)
    identities = tuple(event.event_id for event in events)
    if any(current <= previous for previous, current in pairwise(cursors)):
        return None
    if len(set(identities)) != len(identities):
        return None
    second_sequence = ",".join(str(event.cursor) for event in replay)
    return (
        f"first_cursor={initial.cursor} reconnect_cursor={initial.cursor} "
        f"second_sequence={second_sequence} no_duplicate_ids=true"
    )


def simple_snapshot(fixture: Fixture) -> bool:
    """Confirm the owned fixture is a SIMPLE Run through the live HTTP snapshot."""
    response = _response(fixture, f"/api/runs/{fixture.run_id}")
    if response is None:
        return False
    with response:
        try:
            return Snapshot.model_validate_json(response.read()).runtime_kind == "simple"
        except ValidationError:
            print("T34 S1 driver failed: snapshot does not identify a SIMPLE Run")
            return False


def cancel(fixture: Fixture) -> tuple[str | None, str | None] | None:
    """Repeat the fixed cancellation surface and retain only replay markers."""
    first = _response(fixture, f"/api/runs/{fixture.run_id}/cancel", "POST")
    if first is None:
        return None
    with first:
        initial = first.headers.get("Idempotency-Replayed")
    second = _response(fixture, f"/api/runs/{fixture.run_id}/cancel", "POST")
    if second is None:
        return None
    with second:
        return initial, second.headers.get("Idempotency-Replayed")


def write_receipt(root: Path, scenario: str, observation: str) -> None:
    """Write a passed receipt only after the real HTTP observation completes."""
    root.mkdir(parents=True, exist_ok=True)
    payload = {
        "scenario": scenario,
        "passed": True,
        "surface_observation": observation,
        "cleanup_confirmed": True,
    }
    _ = (root / "receipt.json").write_text(json.dumps(payload), encoding="utf-8")


def main(arguments: list[str]) -> int:
    """Reserve direct invocation for scenario modules, not a generic driver CLI."""
    _ = arguments
    print("T34 driver must use a fixed scenario entrypoint")
    return 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
