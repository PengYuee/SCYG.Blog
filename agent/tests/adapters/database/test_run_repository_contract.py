"""Static contracts for the PostgreSQL Run repository."""

import ast
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID

from sqlalchemy.dialects import postgresql

from scyg_agent.adapters.database.run_queries import (
    claim_candidates_statement,
    renew_lease_statement,
)
from scyg_agent.domain.runs import ExecutionOwnerId, RunId
from scyg_agent.domain.runs.repository import LeaseGuard, LeaseToken, RenewRequest


def test_claim_query_uses_bounded_skip_locked_ordering() -> None:
    # Given: the adapter claim statement.
    compiled = str(claim_candidates_statement().compile(dialect=postgresql.dialect()))

    # When/Then: PostgreSQL locking and deterministic queue ordering are explicit.
    assert "FOR UPDATE SKIP LOCKED" in compiled
    assert "ORDER BY" in compiled
    assert "next_attempt_at" in compiled
    assert "created_at" in compiled
    assert "run_id" in compiled
    assert "LIMIT" in compiled
    assert "runtime_kind" in compiled
    assert compiled.index("runtime_kind") < compiled.index("LIMIT")


def test_renew_query_preserves_a_longer_existing_expiry() -> None:
    # Given: a candidate renewal shorter than the lease may already be.
    now = datetime(2026, 7, 12, 9, 1, tzinfo=UTC)
    guard = LeaseGuard(
        RunId("run_12345678"),
        ExecutionOwnerId("worker_12345678"),
        LeaseToken(UUID("12345678-1234-5678-9234-567812345678")),
        2,
        now,
    )

    # When: the guarded renewal statement is compiled for PostgreSQL.
    compiled = str(
        renew_lease_statement(RenewRequest(guard, timedelta(minutes=1))).compile(
            dialect=postgresql.dialect()
        )
    )

    # Then: persisted expiry and the candidate are compared monotonically in SQL.
    assert "greatest(agent_runs.lease_expires_at" in compiled.lower()
    assert "RETURNING" in compiled


def test_repository_architecture_keeps_domain_pure_and_adapter_narrow() -> None:
    # Given: both repository boundary modules.
    root = Path(__file__).parents[3] / "src" / "scyg_agent"
    domain = ast.parse((root / "domain" / "runs" / "repository.py").read_text(encoding="utf-8"))
    adapter = ast.parse(
        (root / "adapters" / "database" / "run_repository.py").read_text(encoding="utf-8")
    )

    # When: imported top-level packages are collected.
    domain_imports = _import_roots(domain)
    adapter_imports = _import_roots(adapter)

    # Then: infrastructure stays out of the port and runtime stacks stay out of persistence.
    assert "sqlalchemy" not in domain_imports
    assert adapter_imports.isdisjoint({"fastapi", "grpc", "langgraph", "deepagents"})


def _import_roots(tree: ast.Module) -> set[str]:
    """Collect absolute import roots from one parsed module."""
    roots: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            roots.update(alias.name.partition(".")[0] for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module is not None:
            roots.add(node.module.partition(".")[0])
    return roots
