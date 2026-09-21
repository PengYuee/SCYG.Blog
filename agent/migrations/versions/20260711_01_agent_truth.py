"""Create Agent persistence truth and the shared LangGraph schema.

Revision ID: 20260711_01
Revises: None
"""

from collections.abc import Sequence
from typing import Final

from alembic import op

from scyg_agent.adapters.database.records import Base

AGENT_ROLE: Final = "scyg_agent"

revision: str = "20260711_01"
down_revision: str | None = None
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

KNOWN_AGENT_TABLES: Final = (
    "agent_runs",
    "agent_interactions",
    "agent_events",
    "agent_commands",
    "agent_tool_calls",
    "agent_audit_events",
)

DOWNGRADE_GUARD_SQL: Final = """DO $$
DECLARE
    table_name text;
    row_count bigint;
BEGIN
    FOREACH table_name IN ARRAY ARRAY[
        'agent_runs',
        'agent_interactions',
        'agent_events',
        'agent_commands',
        'agent_tool_calls',
        'agent_audit_events'
    ] LOOP
        IF to_regclass(format('public.%I', table_name)) IS NOT NULL THEN
            IF EXISTS (
                SELECT 1
                FROM pg_class c
                JOIN pg_namespace n ON n.oid = c.relnamespace
                JOIN pg_roles r ON r.oid = c.relowner
                WHERE n.nspname = 'public'
                  AND c.relname = table_name
                  AND r.rolname <> 'scyg_agent'
            ) THEN
                RAISE EXCEPTION 'unknown Agent table owner blocks downgrade';
            END IF;
            EXECUTE format('SELECT count(*) FROM public.%I', table_name)
                INTO row_count;
            IF row_count <> 0 THEN
                RAISE EXCEPTION 'Agent truth data blocks downgrade';
            END IF;
        END IF;
    END LOOP;

    IF NOT EXISTS (
        SELECT 1 FROM pg_namespace WHERE nspname = 'langgraph'
    ) THEN
        RETURN;
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM pg_namespace n
        JOIN pg_roles r ON r.oid = n.nspowner
        WHERE n.nspname = 'langgraph'
          AND r.rolname = 'scyg_agent'
    ) THEN
        RAISE EXCEPTION 'unknown checkpoint schema owner blocks downgrade';
    END IF;
    IF EXISTS (
        SELECT 1
        FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'langgraph'
          AND c.relkind IN ('r', 'p', 'S', 'v', 'm', 'f', 'c')
    ) OR EXISTS (
        SELECT 1
        FROM pg_proc p
        JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE n.nspname = 'langgraph'
    ) OR EXISTS (
        SELECT 1
        FROM pg_type t
        JOIN pg_namespace n ON n.oid = t.typnamespace
        WHERE n.nspname = 'langgraph'
          AND t.typrelid = 0
          AND t.typtype IN ('d', 'e', 'r')
    ) THEN
        RAISE EXCEPTION 'retained checkpoint objects block downgrade';
    END IF;
END $$"""


def _role_sql() -> str:
    """Return the single fixed Agent database role."""
    return f'"{AGENT_ROLE}"'


def upgrade() -> None:
    """Create Agent-owned tables and grant the shared application role checkpoint access."""
    role = _role_sql()
    bind = op.get_bind()
    Base.metadata.create_all(bind=bind, checkfirst=True)
    op.execute("CREATE SCHEMA IF NOT EXISTS langgraph")
    op.execute(f"GRANT USAGE, CREATE ON SCHEMA langgraph TO {role}")
    op.execute(f"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA langgraph TO {role}")
    op.execute(f"GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA langgraph TO {role}")
    op.execute(
        " ".join(
            (
                "ALTER DEFAULT PRIVILEGES IN SCHEMA langgraph",
                f"GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO {role}",
            )
        )
    )
    op.execute(
        " ".join(
            (
                "ALTER DEFAULT PRIVILEGES IN SCHEMA langgraph",
                f"GRANT USAGE, SELECT, UPDATE ON SEQUENCES TO {role}",
            )
        )
    )


def downgrade() -> None:
    """Revoke privileges and remove only empty objects owned by this revision."""
    op.execute(DOWNGRADE_GUARD_SQL)
    role = _role_sql()
    op.execute(
        " ".join(
            (
                "ALTER DEFAULT PRIVILEGES IN SCHEMA langgraph",
                f"REVOKE USAGE, SELECT, UPDATE ON SEQUENCES FROM {role}",
            )
        )
    )
    op.execute(
        " ".join(
            (
                "ALTER DEFAULT PRIVILEGES IN SCHEMA langgraph",
                f"REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM {role}",
            )
        )
    )
    op.execute(f"REVOKE USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA langgraph FROM {role}")
    op.execute(
        f"REVOKE SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA langgraph FROM {role}"
    )
    op.execute(f"REVOKE USAGE, CREATE ON SCHEMA langgraph FROM {role}")
    op.execute("DROP SCHEMA IF EXISTS langgraph")
    Base.metadata.drop_all(bind=op.get_bind(), checkfirst=True)
