"""Isolated migration-database fixtures for real PostgreSQL and grpc.aio tests."""

from collections.abc import AsyncIterator

import pytest
from tests.integration.t20.postgres_support import T20Database, migrate, migration_environment


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


@pytest.fixture
async def database() -> AsyncIterator[T20Database]:
    environment = migration_environment()
    migrate(["upgrade", "head"], environment)
    database = T20Database.create(
        environment["SCYG_AGENT_DATABASE_URL"], environment["SCYG_T20_LISTENER_DSN"]
    )
    try:
        await database.reset()
        yield database
    finally:
        try:
            await database.reset()
        finally:
            await database.close()
        migrate(["downgrade", "base"], environment)
