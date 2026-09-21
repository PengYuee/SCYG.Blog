"""Strengthen T12 semantic identity and audit immutability.

Revision ID: 20260712_02
Revises: 20260711_01
"""

from collections.abc import Sequence

from alembic import op

revision: str = "20260712_02"
down_revision: str | None = "20260711_01"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

_ZERO_DIGEST = "0" * 64


def upgrade() -> None:
    """Add canonical identities and reject audit mutation in PostgreSQL."""
    op.execute(
        f"ALTER TABLE agent_commands ADD COLUMN IF NOT EXISTS semantic_digest "
        f"VARCHAR(64) NOT NULL DEFAULT '{_ZERO_DIGEST}'"
    )
    op.execute(
        f"ALTER TABLE agent_interactions ADD COLUMN IF NOT EXISTS request_semantic_digest "
        f"VARCHAR(64) NOT NULL DEFAULT '{_ZERO_DIGEST}'"
    )
    op.execute(
        "ALTER TABLE agent_interactions ADD COLUMN IF NOT EXISTS "
        "resolution_semantic_digest VARCHAR(64)"
    )
    op.execute(
        f"ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS semantic_digest "
        f"VARCHAR(64) NOT NULL DEFAULT '{_ZERO_DIGEST}'"
    )
    op.execute(
        "ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS request_audit_metadata "
        'JSONB NOT NULL DEFAULT \'{"source":"legacy"}\'::jsonb'
    )
    op.execute(
        "CREATE OR REPLACE FUNCTION reject_agent_audit_mutation() RETURNS trigger "
        "LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'agent audit rows are immutable' "
        "USING ERRCODE = '55000'; END; $$"
    )
    op.execute(
        "CREATE TRIGGER agent_audit_events_immutable BEFORE UPDATE OR DELETE "
        "ON agent_audit_events FOR EACH ROW EXECUTE FUNCTION reject_agent_audit_mutation()"
    )


def downgrade() -> None:
    """Remove audit trigger and canonical identity columns symmetrically."""
    op.execute("DROP TRIGGER agent_audit_events_immutable ON agent_audit_events")
    op.execute("DROP FUNCTION reject_agent_audit_mutation()")
    op.execute("ALTER TABLE agent_tool_calls DROP COLUMN request_audit_metadata")
    op.execute("ALTER TABLE agent_tool_calls DROP COLUMN semantic_digest")
    op.execute("ALTER TABLE agent_interactions DROP COLUMN resolution_semantic_digest")
    op.execute("ALTER TABLE agent_interactions DROP COLUMN request_semantic_digest")
    op.execute("ALTER TABLE agent_commands DROP COLUMN semantic_digest")
