"""Bounded command surface tests."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - 仅构造同步 policy 测试替身。
from collections.abc import Awaitable, Callable
from pathlib import Path
from typing import final

import pytest

from scyg_agent.__main__ import configure_event_loop_policy, run
from scyg_agent.config import ApplicationSettings
from scyg_agent.local_setup import LocalSetupStageError


@pytest.fixture(autouse=True)
def isolate_default_config(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """Prevent a developer's ignored agent.toml from changing CLI tests."""
    monkeypatch.chdir(tmp_path)


def test_check_reports_stable_secret_free_readiness(
    configured_environment: None,
    capsys: pytest.CaptureFixture[str],
) -> None:
    # Given: a valid environment and the bounded check command.
    assert configured_environment is None
    # When: the command composes the application.
    exit_code = run(("--check",))

    # Then: it exits successfully with stable, secret-free output.
    assert exit_code == 0
    assert capsys.readouterr().out == "SCYG Agent configuration ready\n"


def test_unknown_command_is_rejected_without_starting_service(
    capsys: pytest.CaptureFixture[str],
) -> None:
    # Given: an unsupported command that must never start a listener.
    # When: the bounded command parser receives it.
    exit_code = run(("--serve",))

    # Then: the command fails deterministically without configuration parsing.
    assert exit_code == 2
    assert capsys.readouterr().err == (
        "用法: scyg-agent [--help|--check|setup [--admin-url URL]|run]\n"
    )


def test_check_reports_malformed_toml_without_echoing_content(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    sentinel = "sentinel-file-secret"
    _ = (tmp_path / "agent.toml").write_text(f'provider_api_key = "{sentinel}\n', encoding="utf-8")

    exit_code = run(("--check",))
    stderr = capsys.readouterr().err

    assert exit_code == 2
    assert stderr == "SCYG Agent 配置文件无效: agent.toml: contains invalid TOML\n"
    assert sentinel not in stderr


def test_check_reports_malformed_database_without_echoing_credentials(
    configured_environment: None,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    # Given: a malformed database DSN contains unique credentials.
    assert configured_environment is None
    raw_dsn = "not-postgres://sentinel-user:sentinel-password-9472@db/scyg"
    monkeypatch.setenv("SCYG_AGENT_DATABASE_URL", raw_dsn)

    # When: the bounded configuration check parses the environment.
    exit_code = run(("--check",))
    stderr = capsys.readouterr().err

    # Then: a useful field error is emitted without any credential input.
    assert exit_code == 2
    assert stderr == ("SCYG Agent 配置无效:\n- database_url: must be a valid PostgreSQL DSN\n")
    assert "sentinel-password-9472" not in stderr
    assert "sentinel-user" not in stderr
    assert raw_dsn not in stderr


def test_check_reports_url_and_concurrency_without_echoing_environment_values(
    configured_environment: None,
    monkeypatch: pytest.MonkeyPatch,
    capsys: pytest.CaptureFixture[str],
) -> None:
    # Given: provider URL and concurrency values are malformed external input.
    assert configured_environment is None
    provider_value = "sentinel-provider-url-6381"
    concurrency_value = "0"
    monkeypatch.setenv("SCYG_AGENT_PROVIDER_BASE_URL", provider_value)
    monkeypatch.setenv("SCYG_AGENT_SIMPLE_CONCURRENCY", concurrency_value)

    # When: the bounded configuration check parses the environment.
    exit_code = run(("--check",))
    stderr = capsys.readouterr().err

    # Then: stable field reasons remain useful without raw values.
    assert exit_code == 2
    assert stderr == (
        "SCYG Agent 配置无效:\n"
        "- provider_base_url: must be a valid URL\n"
        "- simple_concurrency: must be greater than or equal to 1\n"
    )
    assert provider_value not in stderr
    assert concurrency_value not in stderr


def test_help_is_available_without_configuration(
    monkeypatch: pytest.MonkeyPatch, capsys: pytest.CaptureFixture[str]
) -> None:
    monkeypatch.delenv("SCYG_AGENT_DATABASE_URL", raising=False)
    assert run(("--help",)) == 0
    assert "run      启动 Agent 服务" in capsys.readouterr().out


def test_run_delegates_typed_settings(configured_environment: None) -> None:
    assert configured_environment is None
    called = False

    def runner(_settings: ApplicationSettings) -> None:
        nonlocal called
        called = True

    assert run(("run",), service_runner=runner) == 0
    assert called


def test_run_hides_startup_exception(
    configured_environment: None, capsys: pytest.CaptureFixture[str]
) -> None:
    assert configured_environment is None

    def failing(_settings: ApplicationSettings) -> None:
        message = "sentinel-provider-secret"
        raise RuntimeError(message)

    assert run(("run",), service_runner=failing) == 1
    stderr = capsys.readouterr().err
    assert stderr == "SCYG Agent 启动或关闭失败\n"
    assert "sentinel" not in stderr


def test_setup_reads_password_and_delegates_settings(
    configured_environment: None, capsys: pytest.CaptureFixture[str]
) -> None:
    assert configured_environment is None
    observed: tuple[str, str | None] | None = None

    def setup_runner(_settings: ApplicationSettings, password: str, admin_url: str | None) -> None:
        nonlocal observed
        observed = password, admin_url

    exit_code = run(
        ("setup",),
        setup_runner=setup_runner,
        password_reader=lambda _prompt: "admin-secret",
    )

    assert exit_code == 0
    assert observed == ("admin-secret", None)
    assert capsys.readouterr().out == "SCYG Agent database setup complete\n"


def test_setup_forwards_explicit_admin_url(configured_environment: None) -> None:
    assert configured_environment is None
    observed: str | None = None

    def setup_runner(_settings: ApplicationSettings, _password: str, admin_url: str | None) -> None:
        nonlocal observed
        observed = admin_url

    admin_url = "postgresql://postgres@db.example:5433/postgres"
    assert (
        run(
            ("setup", "--admin-url", admin_url),
            setup_runner=setup_runner,
            password_reader=lambda _prompt: "admin-secret",
        )
        == 0
    )
    assert observed == admin_url


def test_setup_hides_database_failure_and_password(
    configured_environment: None, capsys: pytest.CaptureFixture[str]
) -> None:
    assert configured_environment is None
    sentinel = "sentinel-admin-password"

    def failing(_settings: ApplicationSettings, password: str, _admin_url: str | None) -> None:
        raise RuntimeError(password)

    assert (
        run(
            ("setup",),
            setup_runner=failing,
            password_reader=lambda _prompt: sentinel,
        )
        == 1
    )
    stderr = capsys.readouterr().err
    assert stderr == "SCYG Agent 数据库初始化失败\n"
    assert sentinel not in stderr


def test_setup_reports_stage_and_sqlstate_without_credentials(
    configured_environment: None, capsys: pytest.CaptureFixture[str]
) -> None:
    assert configured_environment is None

    def failing(_settings: ApplicationSettings, _password: str, _admin_url: str | None) -> None:
        stage = "database"
        error_type = "OperationalError"
        sqlstate = "28P01"
        raise LocalSetupStageError(stage, error_type, sqlstate)

    assert (
        run(
            ("setup",),
            setup_runner=failing,
            password_reader=lambda _prompt: "admin-sentinel",
        )
        == 1
    )
    assert capsys.readouterr().err == (
        "SCYG Agent 数据库初始化失败: stage=database; error=OperationalError (SQLSTATE 28P01)\n"
    )


@final
class FakePolicy(asyncio.DefaultEventLoopPolicy):
    """表示测试中的普通或 Selector policy."""

    def __init__(self, *, selector: bool) -> None:
        super().__init__()
        self.selector: bool = selector


@final
class FakePolicyApi:
    """记录同步事件循环 policy API 调用."""

    def __init__(self, current: FakePolicy) -> None:
        self.current: asyncio.AbstractEventLoopPolicy = current
        self.created: int = 0
        self.set_values: list[asyncio.AbstractEventLoopPolicy] = []

    def get(self) -> asyncio.AbstractEventLoopPolicy:
        return self.current

    def is_selector(self, policy: asyncio.AbstractEventLoopPolicy) -> bool:
        return isinstance(policy, FakePolicy) and policy.selector

    def create_selector(self) -> asyncio.AbstractEventLoopPolicy:
        self.created += 1
        return FakePolicy(selector=True)

    def set(self, policy: asyncio.AbstractEventLoopPolicy) -> None:
        self.current = policy
        self.set_values.append(policy)


def test_windows_installs_selector_policy_before_async_runtime() -> None:
    api = FakePolicyApi(FakePolicy(selector=False))

    configure_event_loop_policy(platform="win32", api=api)

    assert api.created == 1
    assert len(api.set_values) == 1
    assert isinstance(api.current, FakePolicy)
    assert api.current.selector


def test_windows_preserves_existing_selector_policy_idempotently() -> None:
    selector = FakePolicy(selector=True)
    api = FakePolicyApi(selector)

    configure_event_loop_policy(platform="win32", api=api)
    configure_event_loop_policy(platform="win32", api=api)

    assert api.current is selector
    assert api.created == 0
    assert api.set_values == []


def test_non_windows_does_not_read_or_change_policy() -> None:
    api = FakePolicyApi(FakePolicy(selector=False))

    configure_event_loop_policy(platform="linux", api=api)

    assert api.created == 0
    assert api.set_values == []


def test_run_configures_policy_before_anyio_run(
    configured_environment: None, monkeypatch: pytest.MonkeyPatch
) -> None:
    assert configured_environment is None
    events: list[str] = []

    def configure() -> None:
        events.append("policy")

    def anyio_run(
        _function: Callable[[ApplicationSettings], Awaitable[None]],
        _settings: ApplicationSettings,
    ) -> None:
        events.append("anyio")

    monkeypatch.setattr("scyg_agent.__main__.configure_event_loop_policy", configure)
    monkeypatch.setattr("scyg_agent.__main__.anyio.run", anyio_run)

    assert run(("run",)) == 0
    assert events == ["policy", "anyio"]
