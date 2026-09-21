"""Application composition and lifecycle seams."""

from collections.abc import AsyncIterator, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager

from fastapi import FastAPI

from scyg_agent.config import Settings


def create_lifespan() -> Callable[[FastAPI], AbstractAsyncContextManager[None]]:
    """Create an intentionally empty service lifecycle seam."""

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        """Reserve structured startup and shutdown ownership for later tasks."""
        yield

    return lifespan


def create_app(settings: Settings) -> FastAPI:
    """Compose the HTTP application without starting listeners or external resources."""
    del settings
    return FastAPI(title="SCYG Agent", lifespan=create_lifespan())
