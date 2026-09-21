"""No-database adapter boundary tests for T11 validation."""

import pytest
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.event_store import (
    NotificationChannel,
    PostgreSQLEventStore,
)
from scyg_agent.adapters.database.event_subscription import open_listener_connection
from scyg_agent.domain.ports.event_store import EventCursor, InvalidReplayLimitError
from scyg_agent.domain.runs import RunId


@pytest.fixture
def anyio_backend() -> str:
    """Use the backend compatible with the pinned SQLAlchemy driver."""
    return "asyncio"


def test_constructor_rejects_invalid_subscription_replay_limit_without_database() -> None:
    # Given/When/Then: invalid configured replay capacity is rejected immediately.
    with pytest.raises(InvalidReplayLimitError, match="between 1 and 1000"):
        _ = PostgreSQLEventStore(
            async_sessionmaker[AsyncSession](),
            "postgresql://unused",
            NotificationChannel.parse("scyg_t11_events"),
            open_listener_connection,
            replay_limit=0,
        )


@pytest.mark.anyio
async def test_replay_rejects_invalid_limit_before_session_acquisition() -> None:
    # Given: an adapter wired to a session factory that must never be called.
    store = PostgreSQLEventStore(
        async_sessionmaker[AsyncSession](),
        "postgresql://unused",
        NotificationChannel.parse("scyg_t11_events"),
        open_listener_connection,
    )

    # When/Then: replay rejects the boundary before touching database infrastructure.
    with pytest.raises(InvalidReplayLimitError, match="between 1 and 1000"):
        _ = await store.replay(RunId("run_00000001"), EventCursor(0), -1)
