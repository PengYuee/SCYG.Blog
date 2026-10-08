"""Persisted Recipe inputs remain executable after the public JSON cutover."""

from uuid import uuid4

import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker

from scyg_agent.adapters.database.event_store import NotificationChannel, PostgreSQLEventStore
from scyg_agent.adapters.database.event_subscription import open_listener_connection
from scyg_agent.adapters.database.run_records import RunRecord
from scyg_agent.adapters.database.run_request_source import PostgreSQLRunInputSource, map_input_row
from scyg_agent.agents.contracts import Capability, WritingInput
from scyg_agent.agents.input import decode_agent_input
from scyg_agent.agents.recipes import default_recipe_registry
from scyg_agent.application.control import ControlApplication
from scyg_agent.application.event_subscription import EventSubscriptionService
from scyg_agent.domain.runs import RunId
from scyg_agent.domain.runs.input import MissingRunInput

type DatabaseFixture = tuple[AsyncEngine, async_sessionmaker[AsyncSession]]


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("capability", "payload"),
    [
        (Capability.CHAT, b'{"message":"hello"}'),
        (Capability.SEARCH, b'{"query":"architecture"}'),
        (Capability.WRITE, b'{"topic":"article","reference_article_ids":[17,23]}'),
        (Capability.POLISH, b'{"content":"paragraph","requirements":"clear"}'),
    ],
)
async def test_created_capability_snapshot_decodes_for_real_runner(
    t12_database: DatabaseFixture,
    capability: Capability,
    payload: bytes,
) -> None:
    engine, sessions = t12_database
    store = PostgreSQLEventStore(
        sessions,
        str(engine.url),
        NotificationChannel.parse("agent_events"),
        open_listener_connection,
    )
    control = ControlApplication(sessions, "agent_events", EventSubscriptionService(store))
    created = await control.create("owner", str(uuid4()), capability, payload)
    persisted = await PostgreSQLRunInputSource(sessions).get_input(RunId(created.run_id))
    assert not isinstance(persisted, MissingRunInput)
    snapshot = decode_agent_input(persisted, default_recipe_registry())
    assert snapshot.capability is capability
    assert snapshot.locale == "und"
    if capability is Capability.WRITE:
        assert isinstance(snapshot.value, WritingInput)
        assert snapshot.value.reference_article_ids == (17, 23)
    async with sessions() as session:
        row = (
            await session.execute(
                select(RunRecord).where(RunRecord.run_id == created.run_id),
            )
        ).scalar_one()
        assert row.thread_id == created.run_id
        assert row.recipe_id == snapshot.recipe.recipe_id.value
        assert row.quality == snapshot.recipe.model_tier


@pytest.mark.anyio
async def test_nonexistent_input_is_typed_missing(t12_database: DatabaseFixture) -> None:
    _, sessions = t12_database
    result = await PostgreSQLRunInputSource(sessions).get_input(RunId("legal-but-absent"))
    assert isinstance(result, MissingRunInput)


@pytest.mark.parametrize("row", [None, (None, ""), ("message", None)])
def test_incomplete_historical_row_cannot_be_executed(
    row: tuple[str | None, str | None] | None,
) -> None:
    assert isinstance(map_input_row(RunId("historical-row"), row), MissingRunInput)
