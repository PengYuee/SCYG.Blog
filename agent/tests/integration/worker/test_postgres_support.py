"""双 Worker PostgreSQL 辅助边界的纯测试."""

from scyg_agent.domain.runs import RunId

from .postgres_support import persisted_run_matches


def test_persisted_run_string_matches_exact_branded_id() -> None:
    """Given 持久化字符串, When 解析为 RunId, Then 只匹配精确身份."""
    expected = RunId("run_workerpg000")

    assert persisted_run_matches("run_workerpg000", expected) is True
    assert persisted_run_matches("run_workerpg001", expected) is False
