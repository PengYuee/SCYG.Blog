"""Health-only HTTP listener; Blog owns every browser business endpoint."""

from fastapi import FastAPI


def create_http_app() -> FastAPI:
    """Create the application onto which composition mounts health probes."""
    return FastAPI(title="SCYG Agent Health", openapi_url=None, docs_url=None, redoc_url=None)
