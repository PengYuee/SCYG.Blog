"""Add successful public keys and lossless interaction response presence."""

from alembic import op

revision = "20261007_09"
down_revision = "20260922_08"
branch_labels = None
depends_on = None


def upgrade() -> None:
    """Keep existing Run, event, interaction, and checkpoint data intact."""
    op.execute("""
        CREATE TABLE IF NOT EXISTS agent_successful_operations (
            user_id VARCHAR(128) NOT NULL,
            idempotency_key UUID NOT NULL,
            run_id VARCHAR(68) NOT NULL REFERENCES agent_runs(run_id) ON DELETE RESTRICT,
            succeeded_at TIMESTAMPTZ NOT NULL,
            expires_at TIMESTAMPTZ NOT NULL,
            PRIMARY KEY (user_id, idempotency_key)
        )
    """)
    op.execute("""
        CREATE INDEX IF NOT EXISTS ix_agent_successful_operations_expires_at
        ON agent_successful_operations (expires_at)
    """)
    op.execute("ALTER TABLE agent_interactions ADD COLUMN IF NOT EXISTS request_payload JSONB")
    op.execute("ALTER TABLE agent_interactions ADD COLUMN IF NOT EXISTS decision VARCHAR(32)")
    op.execute("ALTER TABLE agent_interactions ADD COLUMN IF NOT EXISTS response_payload JSONB")
    op.execute("""
        ALTER TABLE agent_interactions ADD COLUMN IF NOT EXISTS
        payload_present BOOLEAN NOT NULL DEFAULT false
    """)


def downgrade() -> None:
    """Remove only the control-plane additions, never checkpoint tables."""
    for name in ("payload_present", "response_payload", "decision", "request_payload"):
        op.execute(f"ALTER TABLE agent_interactions DROP COLUMN IF EXISTS {name}")
    op.execute("DROP INDEX IF EXISTS ix_agent_successful_operations_expires_at")
    op.execute("DROP TABLE IF EXISTS agent_successful_operations")
