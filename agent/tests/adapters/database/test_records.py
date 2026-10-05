"""PostgreSQL record metadata contract tests."""

from sqlalchemy import CheckConstraint, UniqueConstraint
from sqlalchemy.dialects import postgresql
from sqlalchemy.schema import CreateTable

from scyg_agent.adapters.database.records import Base

EXPECTED_TABLES = {
    "agent_runs",
    "agent_run_results",
    "agent_checkpoint_bindings",
    "agent_events",
    "agent_commands",
    "agent_interactions",
    "agent_tool_calls",
    "agent_audit_events",
}


def test_metadata_contains_agent_truth_tables() -> None:
    # Given/When: the adapter metadata is loaded.
    tables = set(Base.metadata.tables)

    # Then: all Agent-owned truth tables are declared.
    assert tables == EXPECTED_TABLES


def test_records_define_deterministic_constraints_and_indexes() -> None:
    # Given: the complete PostgreSQL metadata.
    event_table = Base.metadata.tables["agent_events"]
    run_table = Base.metadata.tables["agent_runs"]
    tool_table = Base.metadata.tables["agent_tool_calls"]

    # When: named constraints and indexes are inspected.
    event_uniques = {
        constraint.name
        for constraint in event_table.constraints
        if isinstance(constraint, UniqueConstraint)
    }
    run_checks = {
        constraint.name
        for constraint in run_table.constraints
        if isinstance(constraint, CheckConstraint)
    }
    tool_uniques = {
        constraint.name
        for constraint in tool_table.constraints
        if isinstance(constraint, UniqueConstraint)
    }
    run_indexes = {index.name for index in run_table.indexes}

    # Then: replay, queue, and capability-aware idempotency contracts are explicit.
    assert "uq_agent_events_run_id_seq" in event_uniques
    assert "ck_agent_runs_revision_positive" in run_checks
    assert "ck_agent_runs_state_invariants" in run_checks
    assert "uq_agent_tool_calls_operation_id" in tool_uniques
    assert "uq_agent_runs_capability_identity" in run_indexes
    assert "uq_agent_runs_legacy_operation_id" in run_indexes
    assert "ix_agent_runs_claim_queue" in run_indexes


def test_json_columns_document_sanitized_serializer_boundary() -> None:
    # Given: columns capable of carrying structured values.
    intended = {
        "agent_events": ("payload",),
        "agent_audit_events": ("metadata",),
        "agent_runs": ("terminal_metadata", "error_metadata"),
        "agent_tool_calls": ("result_metadata", "error_metadata"),
    }

    # When/Then: every such column advertises the sanitized typed boundary.
    for table_name, column_names in intended.items():
        table = Base.metadata.tables[table_name]
        for column_name in column_names:
            assert "sanitized typed serializer" in (table.c[column_name].comment or "")


def test_metadata_compiles_as_postgresql_not_sqlite() -> None:
    # Given: PostgreSQL-specific record metadata.
    dialect = postgresql.dialect()

    # When: every table is compiled with the PostgreSQL dialect.
    ddl = "\n".join(
        str(CreateTable(table).compile(dialect=dialect)) for table in Base.metadata.sorted_tables
    )

    # Then: UUID, JSONB, and timezone-aware columns are present.
    assert "UUID" in ddl
    assert "JSONB" in ddl
    assert "TIMESTAMP WITH TIME ZONE" in ddl
