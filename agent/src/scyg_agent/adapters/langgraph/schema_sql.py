"""LangGraph 检查点 schema 的固定部署与 readiness SQL."""

from typing import Final

CREATE_METADATA_SQL: Final = """CREATE TABLE IF NOT EXISTS scyg_checkpoint_metadata (
key TEXT PRIMARY KEY,
langgraph TEXT NOT NULL,
checkpoint TEXT NOT NULL,
checkpoint_postgres TEXT NOT NULL,
psycopg TEXT NOT NULL,
psycopg_pool TEXT NOT NULL
)"""
INSERT_METADATA_SQL: Final = """INSERT INTO scyg_checkpoint_metadata
(key, langgraph, checkpoint, checkpoint_postgres, psycopg, psycopg_pool)
VALUES (%s, %s, %s, %s, %s, %s)
ON CONFLICT (key) DO NOTHING"""
READINESS_SQL: Final = """SELECT
ARRAY(SELECT tablename FROM pg_tables WHERE schemaname=current_schema() ORDER BY 1) AS tables,
ARRAY(SELECT indexname FROM pg_indexes WHERE schemaname=current_schema() ORDER BY 1) AS indexes,
ARRAY(SELECT v FROM checkpoint_migrations ORDER BY v) AS migrations,
m.langgraph, m.checkpoint, m.checkpoint_postgres, m.psycopg, m.psycopg_pool
FROM scyg_checkpoint_metadata m WHERE m.key=%s"""
