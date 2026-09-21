"""Deterministic tests for the root Agent QA orchestration contract."""

import subprocess
import sys
from pathlib import Path

import pytest
from scripts.qa_agent import (
    REQUIRED_TOPOLOGY_ENV,
    AgentQa,
    CommandResult,
    Mode,
)

ROOT_TASKFILE = Path(__file__).resolve().parents[2] / "Taskfile.yml"
QA_ENTRYPOINTS = ("qa:agent", "qa:agent:static")


def _root_qa_command(task_name: str) -> tuple[str, ...]:
    """Parse one root QA command from the Taskfile structure."""
    parser = (
        "import sys, yaml; "
        "document = yaml.safe_load(open(sys.argv[1], encoding='utf-8')); "
        "print(document['tasks'][sys.argv[2]]['cmds'][2]['cmd'])"
    )
    result = subprocess.run(  # noqa: S603 - fixed local YAML parser command.
        [sys.executable, "-c", parser, str(ROOT_TASKFILE), task_name],
        capture_output=True,
        check=False,
        text=True,
    )
    assert result.returncode == 0, result.stderr
    return tuple(result.stdout.strip().split())


def _assert_locked_root_qa_command(command: tuple[str, ...], mode: str) -> None:
    """Require the root QA command to enter uv in locked project mode."""
    assert command[:3] == ("uv", "run", "--locked")
    assert command[3:5] == ("--project", "agent")
    assert command[5:8] == ("python", "agent/scripts/qa_agent.py", "--mode")
    assert command[8] == mode


class FakeRunner:
    """Record commands and return configured exit codes."""

    def __init__(self, failure_label: str | None = None) -> None:
        self.failure_label: str | None = failure_label
        self.commands: list[str] = []

    def __call__(self, label: str, command: tuple[str, ...], cwd: Path) -> CommandResult:
        self.commands.append(label)
        if label == self.failure_label:
            return CommandResult(7, f"{label} failed")
        return CommandResult(0, f"{label} passed")


@pytest.mark.parametrize(
    ("task_name", "mode"),
    [("qa:agent", "full"), ("qa:agent:static", "static")],
)
def test_root_qa_entrypoints_are_locked_and_structurally_complete(
    task_name: str, mode: str
) -> None:
    # Given: the root Taskfile command for one public QA entrypoint.
    command = _root_qa_command(task_name)

    # When: the command contract is validated.
    _assert_locked_root_qa_command(command, mode)

    # Then: removing the lock assertion fails deterministically.
    lockless_command = tuple(argument for argument in command if argument != "--locked")
    with pytest.raises(AssertionError):
        _assert_locked_root_qa_command(lockless_command, mode)


def test_static_mode_orders_all_safe_gates_and_uses_distinct_marker(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: every static command succeeds and cleanup is observable.
    runner = FakeRunner()
    cleaned: list[str] = []

    def cleanup(mode: str) -> None:
        cleaned.append(mode)

    qa = AgentQa(tmp_path, runner=runner, tool_exists=lambda _name: True, cleanup=cleanup)

    # When: the CI-safe static orchestration runs.
    exit_code = qa.run(Mode.STATIC)

    # Then: gates are fail-fast ordered, cleanup runs, and full PASS is never emitted.
    assert exit_code == 0
    assert runner.commands == [
        "agent-contracts",
        "ruff-check",
        "ruff-format",
        "basedpyright",
        "pytest-coverage",
        "no-excuse",
        "scope-scan",
        "secret-scan",
        "dependency-audit",
        "deployment-contracts",
    ]
    assert cleaned == ["static"]
    output = capsys.readouterr().out
    assert "agent QA: STATIC PASS" in output
    assert "agent QA: PASS" not in output


def test_failure_is_fail_fast_and_cleanup_preserves_primary_label(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: the contract gate intentionally fails before later gates.
    runner = FakeRunner("agent-contracts")
    cleaned: list[str] = []

    def cleanup(mode: str) -> None:
        cleaned.append(mode)

    qa = AgentQa(tmp_path, runner=runner, tool_exists=lambda _name: True, cleanup=cleanup)

    # When: static QA runs against the negative fixture.
    exit_code = qa.run(Mode.STATIC)

    # Then: the primary label remains visible, cleanup runs, and no marker claims success.
    output = capsys.readouterr().out
    assert exit_code != 0
    assert runner.commands == ["agent-contracts"]
    assert cleaned == ["static"]
    assert "agent-contracts" in output
    assert "agent QA: PASS" not in output
    assert "agent QA: STATIC PASS" not in output


def test_cleanup_failure_sanitizes_diagnostic(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: otherwise-successful static QA has a cleanup failure containing a labeled secret.
    field_value = "f2-cleanup-value-7291"

    def fail_cleanup(mode: str) -> None:
        _ = mode
        message = f"password={field_value}"
        raise OSError(message)

    qa = AgentQa(
        tmp_path,
        runner=FakeRunner(),
        tool_exists=lambda _name: True,
        cleanup=fail_cleanup,
    )

    # When: cleanup runs after the ordered static checks.
    exit_code = qa.run(Mode.STATIC)
    output = capsys.readouterr().out

    # Then: failure remains visible without disclosing its value.
    assert exit_code != 0
    assert "agent QA cleanup failed: password=[REDACTED]" in output
    assert field_value not in output


def test_full_mode_reports_named_topology_prerequisite_and_cleans_up(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: Docker is unavailable, matching the current T32 host limitation.
    runner = FakeRunner()
    cleaned: list[str] = []

    def cleanup(mode: str) -> None:
        cleaned.append(mode)

    qa = AgentQa(
        tmp_path,
        runner=runner,
        tool_exists=lambda name: name != "docker",
        cleanup=cleanup,
    )

    # When: the normal full gate starts.
    exit_code = qa.run(Mode.FULL)

    # Then: it fails before static claims, names the prerequisite, and cleans up.
    output = capsys.readouterr().out
    assert exit_code != 0
    assert "T32 topology prerequisites" in output
    assert runner.commands == []
    assert cleaned == ["full"]
    assert "agent QA: PASS" not in output


def test_cleanup_failure_turns_success_into_failure_without_false_pass(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: all gates pass but the bounded cleanup reports a failure.
    runner = FakeRunner()

    def fail_cleanup(mode: str) -> None:
        _ = mode
        raise OSError

    qa = AgentQa(tmp_path, runner=runner, tool_exists=lambda _name: True, cleanup=fail_cleanup)

    # When: static QA completes its gates.
    exit_code = qa.run(Mode.STATIC)

    # Then: cleanup failure is visible and prevents every success marker.
    output = capsys.readouterr().out
    assert exit_code != 0
    assert "cleanup" in output
    assert "agent QA: PASS" not in output
    assert "agent QA: STATIC PASS" not in output


def test_full_orders_t34_after_healthy_topology_and_static_stays_e2e_free(
    tmp_path: Path,
) -> None:
    # Given: complete topology inputs and successful owned commands.
    runner = FakeRunner()
    cleaned: list[str] = []
    environment = dict.fromkeys(REQUIRED_TOPOLOGY_ENV, "configured")

    def cleanup(mode: str) -> None:
        cleaned.append(mode)

    qa = AgentQa(
        tmp_path,
        runner=runner,
        tool_exists=lambda _name: True,
        cleanup=cleanup,
        environment=environment,
    )

    # When: full QA executes through topology and T34.
    exit_code = qa.run(Mode.FULL)

    # Then: T34 is last, cleanup runs, and static commands contain no E2E label.
    assert exit_code == 0
    assert runner.commands[-3:] == ["compose-config", "t32-topology", "t34-e2e"]
    assert cleaned == ["full"]

    static_runner = FakeRunner()
    static_qa = AgentQa(tmp_path, runner=static_runner, tool_exists=lambda _name: True)
    assert static_qa.run(Mode.STATIC) == 0
    assert "t34-e2e" not in static_runner.commands


def test_subprocess_runner_redacts_compose_style_secrets_from_output_and_result(
    tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: a child command emits Compose-style DSN, key and JWT diagnostics on both streams.
    dsn_user = "f2-dsn-user-7291"
    dsn_credential = "f2-dsn-password-7291"
    query_credential = "f2-query-secret-7291"
    fragment_credential = "f2-fragment-secret-7291"
    provider_key = "f2-provider-key-7291"
    field_value = "f2-labeled-value-7291"
    jwt = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJmMiJ9.f2-signature-7291"
    diagnostics = "\n".join(
        (
            "service: agent",
            (
                "SCYG_AGENT_DATABASE_URL="
                f"postgresql+asyncpg://{dsn_user}:{dsn_credential}@postgres:5432/scyg_agent"
                f"?access_token={query_credential}#{fragment_credential}"
            ),
            (
                "endpoint: "
                f"postgresql+asyncpg://{dsn_user}:{dsn_credential}@postgres:5432/scyg_agent"
                f"?access_token={query_credential}#{fragment_credential}"
            ),
            f"SCYG_AGENT_PROVIDER_API_KEY={provider_key}",
            f"Authorization: Bearer {jwt}",
            f"password={field_value}",
            f"jwt={field_value}",
            f"endpoint=https://provider.example/v1?token={query_credential}#fragment",
        )
    )
    source = f"import sys; print({diagnostics!r}); print({diagnostics!r}, file=sys.stderr)"

    # When: the QA subprocess boundary captures the command's stdout and stderr.
    qa = AgentQa(tmp_path)
    result = qa.runner("compose-config", (sys.executable, "-c", source), tmp_path)
    captured = capsys.readouterr()
    surfaces = (captured.out, captured.err, result.output)

    # Then: labels and non-secret diagnostics remain while every secret is absent.
    assert result.returncode == 0
    assert "[agent QA] compose-config" in captured.out
    assert "service: agent" in captured.out
    assert (
        "endpoint: postgresql+asyncpg://[REDACTED]@postgres:5432/scyg_agent"
        "?access_token=[REDACTED]#[REDACTED]"
    ) in captured.out
    assert "Authorization: [REDACTED]" in captured.out
    assert "password=[REDACTED]" in captured.out
    assert "jwt=[REDACTED]" in captured.out
    assert "endpoint=https://provider.example/v1?token=[REDACTED]#fragment" in captured.out
    for sentinel in (
        dsn_user,
        dsn_credential,
        query_credential,
        fragment_credential,
        provider_key,
        field_value,
        jwt,
    ):
        assert all(sentinel not in surface for surface in surfaces)


def test_subprocess_runner_sanitizes_timeout_diagnostic(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: the child process times out while carrying secret-shaped diagnostics.
    field_value = "f2-timeout-value-7291"

    def raise_timeout(*_args: object, **_kwargs: object) -> None:
        timeout = subprocess.TimeoutExpired(
            ("docker", "compose"),
            900,
            output=f"password={field_value}",
            stderr=f"jwt={field_value}",
        )
        raise timeout

    monkeypatch.setattr("scripts.qa_agent.subprocess.run", raise_timeout)

    # When: the public QA runner handles the timeout.
    result = AgentQa(tmp_path).runner("compose-config", ("docker", "compose"), tmp_path)
    captured = capsys.readouterr()

    # Then: the stable label remains while neither returned nor printed output leaks the value.
    assert result.returncode == 124
    assert result.output == "compose-config timed out after 900 seconds"
    assert "[agent QA] compose-config" in captured.out
    assert result.output in captured.out
    assert field_value not in captured.out
    assert field_value not in captured.err
    assert field_value not in result.output


def test_subprocess_runner_sanitizes_os_error_diagnostic(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path, capsys: pytest.CaptureFixture[str]
) -> None:
    # Given: process startup fails with a secret-shaped diagnostic.
    field_value = "f2-oserror-value-7291"

    def raise_os_error(*_args: object, **_kwargs: object) -> None:
        message = f"password={field_value}"
        raise OSError(message)

    monkeypatch.setattr("scripts.qa_agent.subprocess.run", raise_os_error)

    # When: the public QA runner returns the failed command result.
    result = AgentQa(tmp_path).runner("compose-config", ("docker", "compose"), tmp_path)
    captured = capsys.readouterr()

    # Then: the error retains its label and redacts the value on every observable surface.
    assert result.returncode == 127
    assert result.output == "compose-config could not start: password=[REDACTED]"
    assert "[agent QA] compose-config" in captured.out
    assert result.output in captured.out
    assert field_value not in captured.out
    assert field_value not in captured.err
    assert field_value not in result.output
