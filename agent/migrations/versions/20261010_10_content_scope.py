"""Freeze content authorization and the first public execution deadline."""

from alembic import op

revision = "20261010_10"
down_revision = "20261007_09"
branch_labels = None
depends_on = None


def upgrade() -> None:
    """Backfill historical Runs as author without retaining an implicit default."""
    op.execute("ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS content_scope VARCHAR(16)")
    op.execute("UPDATE agent_runs SET content_scope = 'author' WHERE content_scope IS NULL")
    op.execute("ALTER TABLE agent_runs ALTER COLUMN content_scope SET NOT NULL")
    op.execute(
        "ALTER TABLE agent_runs ADD COLUMN IF NOT EXISTS public_execution_deadline TIMESTAMPTZ"
    )
    op.execute("""DO $$ BEGIN
        IF NOT EXISTS (SELECT 1 FROM pg_constraint
            WHERE conrelid = 'agent_runs'::regclass
                AND conname = 'ck_agent_runs_content_scope_valid') THEN
            ALTER TABLE agent_runs ADD CONSTRAINT ck_agent_runs_content_scope_valid
                CHECK (content_scope IN ('author','public'));
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint
            WHERE conrelid = 'agent_runs'::regclass
                AND conname = 'ck_agent_runs_public_deadline_scope') THEN
            ALTER TABLE agent_runs ADD CONSTRAINT ck_agent_runs_public_deadline_scope
                CHECK (public_execution_deadline IS NULL OR content_scope = 'public');
        END IF;
        IF NOT EXISTS (SELECT 1 FROM pg_constraint
            WHERE conrelid = 'agent_runs'::regclass
                AND conname = 'ck_agent_runs_public_search_only') THEN
            ALTER TABLE agent_runs ADD CONSTRAINT ck_agent_runs_public_search_only
                CHECK (content_scope <> 'public'
                    OR (capability IS NOT NULL AND capability = 'search'));
        END IF;
    END $$""")


def downgrade() -> None:
    """Remove only this migration's additions."""
    for name in ("public_search_only", "public_deadline_scope", "content_scope_valid"):
        op.drop_constraint(op.f(f"ck_agent_runs_{name}"), "agent_runs", type_="check")
    op.drop_column("agent_runs", "public_execution_deadline")
    op.drop_column("agent_runs", "content_scope")
