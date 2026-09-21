"""Composition and lifecycle seam tests."""

from fastapi import FastAPI

from scyg_agent.bootstrap import create_app
from scyg_agent.config import load_settings


def test_create_app_composes_without_starting_real_services(
    configured_environment: None,
) -> None:
    # Given: valid typed settings exist.
    assert configured_environment is None
    settings = load_settings()

    # When: the application composition seam is invoked.
    app = create_app(settings)

    # Then: an importable FastAPI app exists without listeners or I/O.
    assert isinstance(app, FastAPI)
    assert app.title == "SCYG Agent"
