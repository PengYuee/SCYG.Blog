"""Small control facade fake shared by transport and listener tests."""

from collections.abc import AsyncIterator
from dataclasses import dataclass, field
from datetime import UTC, datetime

import anyio

from scyg_agent.agents.contracts import Capability
from scyg_agent.application.control import ControlSnapshot, ResumeCommand


class OpenedStream:
    def __init__(
        self,
        frames: tuple[str, ...] = (": heartbeat\n\n",),
        ready_to_emit: anyio.Event | None = None,
    ) -> None:
        self.values: tuple[str, ...] = frames
        self.closed: bool = False
        self.ready_to_emit: anyio.Event | None = ready_to_emit

    async def frames(self) -> AsyncIterator[str]:
        if self.ready_to_emit is not None:
            await self.ready_to_emit.wait()
        for frame in self.values:
            yield frame

    async def aclose(self) -> None:
        self.closed = True


@dataclass
class ScenarioFacade:
    calls: list[tuple[object, ...]] = field(default_factory=list)
    error: Exception | None = None
    opened: OpenedStream = field(default_factory=OpenedStream)
    snapshot: ControlSnapshot = field(
        default_factory=lambda: ControlSnapshot(
            run_id="safe-run",
            status="pending",
            capability="chat",
            recipe_id="chat",
            recipe_version="v1",
            created_at=datetime(2026, 1, 1, tzinfo=UTC),
            updated_at=datetime(2026, 1, 1, tzinfo=UTC),
            failure_code=None,
            failure_message=None,
            result_present=False,
            result=None,
            interaction=None,
        )
    )

    def _record(self, operation: str, *args: object) -> ControlSnapshot:
        self.calls.append((operation, *args))
        if self.error is not None:
            raise self.error
        return self.snapshot

    async def create(
        self,
        user_id: str,
        key: str,
        capability: Capability,
        raw_json: bytes,
    ) -> ControlSnapshot:
        return self._record("create", user_id, key, capability, raw_json)

    async def get(self, user_id: str, run_id: str) -> ControlSnapshot:
        return self._record("get", user_id, run_id)

    async def resume(
        self,
        user_id: str,
        key: str,
        command: ResumeCommand,
    ) -> ControlSnapshot:
        return self._record("resume", user_id, key, command)

    async def cancel(self, user_id: str, key: str, run_id: str) -> ControlSnapshot:
        return self._record("cancel", user_id, key, run_id)

    async def open_events(self, user_id: str, run_id: str, cursor: str | None) -> OpenedStream:
        _ = self._record("open_events", user_id, run_id, cursor)
        return self.opened
