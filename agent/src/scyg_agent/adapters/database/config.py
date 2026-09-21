"""Typed async SQLAlchemy configuration seam."""

from dataclasses import dataclass

from pydantic import SecretStr
from sqlalchemy.ext.asyncio import AsyncEngine, create_async_engine


@dataclass(frozen=True, slots=True)
class AsyncDatabaseConfig:
    """Hold a secret DSN and bounded pool settings without printable disclosure."""

    database_url: SecretStr
    pool_size: int = 5
    max_overflow: int = 5
    pool_timeout_seconds: float = 10.0

    def create_engine(self) -> AsyncEngine:
        """Create a pre-pinging async engine from the secret at the I/O boundary."""
        return create_async_engine(
            self.database_url.get_secret_value(),
            pool_pre_ping=True,
            pool_size=self.pool_size,
            max_overflow=self.max_overflow,
            pool_timeout=self.pool_timeout_seconds,
        )
