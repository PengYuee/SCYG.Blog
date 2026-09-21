"""Extend the T12 tool truth row for fenced T17 execution.

Revision ID: 20260712_03
Revises: 20260712_02
"""

from collections.abc import Sequence

from alembic import op

revision: str = "20260712_03"
down_revision: str | None = "20260712_02"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def _sql(*parts: str) -> str:
    """组合可审阅的固定 SQL 片段。."""
    return " ".join(parts)


def upgrade() -> None:
    """添加意图身份、审批绑定和执行围栏字段。."""
    op.execute(
        _sql(
            "ALTER TABLE agent_tool_calls DROP CONSTRAINT IF EXISTS",
            "ck_agent_tool_calls_completion_invariants",
        )
    )
    op.execute(
        "ALTER TABLE agent_tool_calls DROP CONSTRAINT IF EXISTS ck_agent_tool_calls_status_valid"
    )
    for constraint in (
        "fk_agent_tool_calls_approval_interaction_id_agent_interactions",
        "ck_agent_tool_calls_intent_invariants",
        "ck_agent_tool_calls_rpc_start_invariants",
        "ck_agent_tool_calls_claim_version_nonnegative",
        "ck_agent_tool_calls_claim_invariants",
    ):
        op.execute(f"ALTER TABLE agent_tool_calls DROP CONSTRAINT IF EXISTS {constraint}")
    op.execute("ALTER TABLE agent_tool_calls ALTER COLUMN status TYPE VARCHAR(32)")
    op.execute(
        "ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS intent_semantic_digest VARCHAR(64)"
    )
    op.execute(
        "ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS approval_interaction_id VARCHAR(68)"
    )
    op.execute(
        _sql(
            "ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS",
            "approval_resolution_digest VARCHAR(64)",
        )
    )
    op.execute("ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS claim_token UUID")
    op.execute(
        _sql(
            "ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS claim_version",
            "INTEGER NOT NULL DEFAULT 0",
        )
    )
    op.execute("ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS claim_expires_at TIMESTAMPTZ")
    op.execute("ALTER TABLE agent_tool_calls ADD COLUMN IF NOT EXISTS rpc_started_at TIMESTAMPTZ")
    op.execute(
        "ALTER TABLE agent_tool_calls ADD CONSTRAINT ck_agent_tool_calls_status_valid CHECK "
        "(status IN ('pending','in_flight','succeeded','failed',"
        "'external_outcome_unknown'))"
    )
    op.execute(
        _sql(
            "ALTER TABLE agent_tool_calls ADD CONSTRAINT",
            "ck_agent_tool_calls_completion_invariants CHECK",
            "((status IN ('pending','in_flight') AND completed_at IS NULL) OR",
            "(status NOT IN ('pending','in_flight') AND completed_at IS NOT NULL))",
        )
    )
    op.execute(
        "ALTER TABLE agent_tool_calls ADD CONSTRAINT ck_agent_tool_calls_claim_invariants CHECK "
        "((status = 'in_flight') = "
        "(claim_token IS NOT NULL AND claim_expires_at IS NOT NULL))"
    )
    op.execute(
        "ALTER TABLE agent_tool_calls ADD CONSTRAINT ck_agent_tool_calls_claim_version_nonnegative "
        "CHECK (claim_version >= 0)"
    )
    op.execute(
        _sql(
            "ALTER TABLE agent_tool_calls ADD CONSTRAINT",
            "ck_agent_tool_calls_rpc_start_invariants CHECK",
            "(rpc_started_at IS NULL OR status IN",
            "('in_flight','succeeded','failed','external_outcome_unknown'))",
        )
    )
    op.execute(
        "ALTER TABLE agent_tool_calls ADD CONSTRAINT ck_agent_tool_calls_intent_invariants "
        "CHECK ((intent_semantic_digest IS NULL AND approval_interaction_id IS NULL "
        "AND approval_resolution_digest IS NULL AND status IN ('succeeded','failed')) OR "
        "(intent_semantic_digest IS NOT NULL AND approval_interaction_id IS NOT NULL "
        "AND approval_resolution_digest IS NOT NULL))"
    )
    op.execute(
        "ALTER TABLE agent_tool_calls ADD CONSTRAINT "
        "fk_agent_tool_calls_approval_interaction_id_agent_interactions FOREIGN KEY "
        "(approval_interaction_id) REFERENCES agent_interactions (interaction_id) "
        "ON DELETE RESTRICT"
    )


def downgrade() -> None:
    """恢复 T12 终态表结构, 拒绝降级仍开放的意图。."""
    op.execute(
        "DO $$ BEGIN IF EXISTS (SELECT 1 FROM agent_tool_calls WHERE status IN "
        "('pending','in_flight','external_outcome_unknown')) THEN "
        "RAISE EXCEPTION 'open or unknown tool operations block downgrade'; "
        "END IF; END $$"
    )
    op.execute(
        "ALTER TABLE agent_tool_calls DROP CONSTRAINT "
        "fk_agent_tool_calls_approval_interaction_id_agent_interactions"
    )
    for constraint in (
        "ck_agent_tool_calls_intent_invariants",
        "ck_agent_tool_calls_rpc_start_invariants",
        "ck_agent_tool_calls_claim_version_nonnegative",
        "ck_agent_tool_calls_claim_invariants",
        "ck_agent_tool_calls_completion_invariants",
        "ck_agent_tool_calls_status_valid",
    ):
        op.execute(f"ALTER TABLE agent_tool_calls DROP CONSTRAINT {constraint}")
    for column in (
        "rpc_started_at",
        "claim_expires_at",
        "claim_version",
        "claim_token",
        "approval_resolution_digest",
        "approval_interaction_id",
        "intent_semantic_digest",
    ):
        op.execute(f"ALTER TABLE agent_tool_calls DROP COLUMN {column}")
    op.execute("ALTER TABLE agent_tool_calls ALTER COLUMN status TYPE VARCHAR(16)")
    op.execute(
        "ALTER TABLE agent_tool_calls ADD CONSTRAINT ck_agent_tool_calls_status_valid CHECK "
        "(status IN ('pending','succeeded','failed'))"
    )
    op.execute(
        _sql(
            "ALTER TABLE agent_tool_calls ADD CONSTRAINT",
            "ck_agent_tool_calls_completion_invariants CHECK",
            "((status = 'pending' AND completed_at IS NULL) OR",
            "(status <> 'pending' AND completed_at IS NOT NULL))",
        )
    )
