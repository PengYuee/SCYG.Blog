"""Run domain public API contract tests."""

from scyg_agent.domain import runs


def test_t08_run_domain_public_api_is_importable() -> None:
    # Given: T08 defines a package-owned pure Run domain.
    # When: a consumer imports the package through its public boundary.
    # Then: the state transition entry point is exported.
    assert callable(runs.transition)
