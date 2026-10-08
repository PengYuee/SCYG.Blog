"""Exercise real Blog JWT -> Agent RPC -> provider -> durable result and SSE."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import time
from pathlib import Path
from typing import TYPE_CHECKING, cast
from uuid import uuid4

import httpx

if TYPE_CHECKING:
    from collections.abc import Callable


def request_json(
    client: httpx.Client,
    path: str,
    *,
    body: bytes | None = None,
    key: str = "",
    method: str | None = None,
) -> dict[str, object]:
    headers = {"Content-Type": "application/json"}
    if key:
        headers["Idempotency-Key"] = key
    response = client.request(
        method or ("GET" if body is None else "POST"),
        path,
        content=body,
        headers=headers,
        timeout=30,
    )
    response.raise_for_status()
    value = cast("object", response.json())
    if not isinstance(value, dict):
        message = "Expected a JSON object from the real service"
        raise TypeError(message)
    return cast("dict[str, object]", value)


def expect_error(status: int, operation: Callable[[], object]) -> None:
    try:
        _ = operation()
    except httpx.HTTPStatusError as error:
        if error.response.status_code != status:
            message = f"Expected HTTP {status}, received {error.response.status_code}"
            raise RuntimeError(message) from None
        return
    message = f"Expected HTTP {status}, but the request succeeded"
    raise RuntimeError(message)


def collect_frames(
    client: httpx.Client,
    path: str,
    cursor: str = "",
) -> list[tuple[str, bytes]]:
    headers = {"Accept": "text/event-stream"}
    if cursor:
        headers["Last-Event-ID"] = cursor
    frames: list[tuple[str, bytes]] = []
    with client.stream("GET", path, headers=headers) as response:
        response.raise_for_status()
        if response.headers.get("content-type", "").split(";", 1)[0] != "text/event-stream":
            message = "Expected the real SSE response"
            raise RuntimeError(message)
        pending = bytearray()
        frame = bytearray()
        event_id = ""
        for chunk in response.iter_raw():
            pending.extend(chunk)
            while b"\n" in pending:
                line, _, remaining = pending.partition(b"\n")
                pending = bytearray(remaining)
                line += b"\n"
                frame.extend(line)
                if line.startswith(b"id:"):
                    event_id = line[3:].strip().decode("ascii")
                if line in (b"\n", b"\r\n"):
                    if event_id:
                        frames.append((event_id, bytes(frame)))
                    frame.clear()
                    event_id = ""
    return frames


def wait_for_success(client: httpx.Client, run_path: str) -> None:
    deadline = time.monotonic() + 180
    while True:
        snapshot = request_json(client, run_path)
        if snapshot["status"] in {"succeeded", "failed", "cancelled"}:
            break
        if time.monotonic() >= deadline:
            message = "Real provider run did not reach a terminal state"
            raise RuntimeError(message)
        time.sleep(0.5)
    if snapshot["status"] != "succeeded" or snapshot.get("result") is None:
        message = "Real provider run did not persist a successful result"
        raise RuntimeError(message)


def verify_frames(client: httpx.Client, events_path: str) -> None:
    frames = collect_frames(client, events_path)
    if not frames or not any(b"event: run_succeeded" in frame for _, frame in frames):
        message = "Real SSE replay did not include the terminal frame"
        raise RuntimeError(message)
    if len(frames) > 1:
        replayed = collect_frames(client, events_path, frames[0][0])
        if replayed != frames[1:]:
            message = "Last-Event-ID replay lost or duplicated durable frames"
            raise RuntimeError(message)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--compose-project")
    arguments = parser.parse_args()
    base = os.environ.get("SCYG_BLOG_HTTP_BASE_URL")
    if arguments.compose_project:
        root = Path(__file__).resolve().parents[2]
        endpoint = subprocess.run(
            (
                "docker",
                "compose",
                "-p",
                arguments.compose_project,
                "-f",
                "compose.yaml",
                "port",
                "blog",
                "8080",
            ),
            cwd=root,
            capture_output=True,
            text=True,
            check=True,
            timeout=30,
        ).stdout.strip()
        if not endpoint or "\n" in endpoint:
            message = "Expected one published Blog HTTP endpoint"
            raise RuntimeError(message)
        base = f"http://{endpoint}"
    if base is None:
        base = f"http://127.0.0.1:{os.environ.get('SCYG_API_PORT', '8080')}"
    base = base.rstrip("/")
    credentials = json.dumps(
        {
            "username": os.environ.get("SCYG_QA_USERNAME", "admin"),
            "password": os.environ.get("SCYG_QA_PASSWORD", "666666"),
        }
    ).encode()
    with httpx.Client(base_url=base, timeout=30) as client:
        login = request_json(client, "/api/v1/auth/login", body=credentials)
        token = str(login["accessToken"])
        key = str(uuid4())
        expect_error(
            401,
            lambda: request_json(
                client,
                "/api/v1/ai/chat",
                body=b'{"message":"unauthenticated"}',
                key=key,
            ),
        )
        client.headers["Authorization"] = f"Bearer {token}"
        # Business validation failure must not consume the UUID key.
        expect_error(
            400,
            lambda: request_json(
                client,
                "/api/v1/ai/chat",
                body=b"null",
                key=key,
            ),
        )
        created = request_json(
            client,
            "/api/v1/ai/chat",
            body=b'{"message":"Reply briefly with a greeting."}',
            key=key,
        )
        run_id = str(created["runId"])
        run_path = f"/api/v1/runs/{run_id}"
        # Replay precedes interpretation of business-invalid input.
        replay = request_json(client, "/api/v1/ai/chat", body=b"null", key=key)
        if replay["runId"] != run_id:
            message = "Idempotent create returned another resource"
            raise RuntimeError(message)
        wait_for_success(client, run_path)
        replay = request_json(client, "/api/v1/ai/chat", body=b"null", key=key)
        if (
            replay["runId"] != run_id
            or replay["status"] != "succeeded"
            or replay.get("result") is None
        ):
            message = "Replay did not return the current durable resource"
            raise RuntimeError(message)
        # Successful keys are shared with bodyless cancellation and resume.
        cancelled_replay = request_json(client, run_path + "/cancel", key=key, method="POST")
        resumed_replay = request_json(
            client,
            run_path + "/resume",
            body=b'{"interactionId":"probe-interaction","decision":"approve","payload":null}',
            key=key,
        )
        if any(
            value["runId"] != run_id or value["status"] != "succeeded"
            for value in (cancelled_replay, resumed_replay)
        ):
            message = "Cross-RPC successful-key replay did not retain its current resource"
            raise RuntimeError(message)
        expect_error(
            409,
            lambda: request_json(
                client,
                run_path + "/cancel",
                key=str(uuid4()),
                method="POST",
            ),
        )
        verify_frames(client, run_path + "/events")
    print("Blog/Agent real business smoke passed: provider result, key replay and SSE")


if __name__ == "__main__":
    main()
