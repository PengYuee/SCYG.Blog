"""Switch Run creation to capability-aware idempotency indexes.

Revision ID: 20260921_07
Revises: 20260921_06
"""

from collections.abc import Sequence
from typing import Final

from alembic import op

revision: str = "20260921_07"
down_revision: str | None = "20260921_06"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

TABLE_NAME: Final = "agent_runs"
OLD_CONSTRAINT: Final = "uq_agent_runs_operation_id"
CAPABILITY_INDEX: Final = "uq_agent_runs_capability_identity"
LEGACY_INDEX: Final = "uq_agent_runs_legacy_operation_id"

UPGRADE_STATEMENTS: Final = (
    "ALTER TABLE agent_runs DROP CONSTRAINT IF EXISTS uq_agent_runs_operation_id",
    "DROP INDEX IF EXISTS uq_agent_runs_operation_id",
    """
    DO $$
    BEGIN
        IF EXISTS (
            SELECT 1
            FROM agent_runs
            WHERE capability IS NOT NULL
            GROUP BY owner_user_id, capability, operation_id
            HAVING count(*) > 1
        ) THEN
            RAISE EXCEPTION 'duplicate capability Run idempotency identities block migration';
        END IF;
    END $$
    """,
    """
    CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_runs_capability_identity
        ON agent_runs (owner_user_id, capability, operation_id)
        WHERE capability IS NOT NULL
    """,
    """
    CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_runs_legacy_operation_id
        ON agent_runs (operation_id)
        WHERE capability IS NULL
    """,
)

DOWNGRADE_STATEMENTS: Final = (
    """
    DO $$
    BEGIN
        IF EXISTS (
            SELECT 1
            FROM agent_runs
            GROUP BY operation_id
            HAVING count(*) > 1
        ) THEN
            RAISE EXCEPTION
                'capability-aware Run identities cannot downgrade to global operation_id';
        END IF;
    END $$
    """,
    "DROP INDEX IF EXISTS uq_agent_runs_capability_identity",
    "DROP INDEX IF EXISTS uq_agent_runs_legacy_operation_id",
    """
    DO $$
    BEGIN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conrelid = 'agent_runs'::regclass
              AND conname = 'uq_agent_runs_operation_id'
        ) THEN
            ALTER TABLE agent_runs
                ADD CONSTRAINT uq_agent_runs_operation_id UNIQUE (operation_id);
        END IF;
    END $$
    """,
)


def upgrade() -> None:
    """Replace global operation uniqueness with capability-aware partial indexes."""
    for statement in UPGRADE_STATEMENTS:
        op.execute(statement)


def downgrade() -> None:
    """Restore global operation uniqueness only when existing rows permit it."""
    for statement in DOWNGRADE_STATEMENTS:
        op.execute(statement)
