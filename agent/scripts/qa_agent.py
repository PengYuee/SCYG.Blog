"""Cross-platform Agent QA orchestration with fail-fast and bounded cleanup."""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import uuid
from collections.abc import Callable, Sequence
from dataclasses import dataclass
from enum import StrEnum
from pathlib import Path
from typing import Final, Protocol, final

from scyg_agent.redaction import sanitize


class Mode(StrEnum):
    """Select the complete local gate or the secret-free CI gate."""

    FULL = "full"
    STATIC = "static"


@dataclass(frozen=True, slots=True)
class CommandResult:
    """Carry a command exit code and bounded diagnostic output."""

    returncode: int
    output: str


class CommandRunner(Protocol):
    """Run one labeled command in a selected directory."""

    def __call__(self, label: str, command: tuple[str, ...], cwd: Path) -> CommandResult:
        """Run one command and return its observable result."""
        ...


class Cleanup(Protocol):
    """Release resources owned by one QA invocation."""

    def __call__(self, mode: str) -> None:
        """Release resources owned by the selected mode."""
        ...


AGENT_ROOT: Final[str] = "agent"
MODE_ARGUMENT_COUNT: Final[int] = 2
USAGE: Final[str] = "usage: qa_agent.py [--mode full|static]"
REQUIRED_TOPOLOGY_ENV: Final[tuple[str, ...]] = (
    "SCYG_POSTGRES_PASSWORD",
    "SCYG_POSTGRES_ADMIN_DATABASE_URL",
    "SCYG_AGENT_DB_PASSWORD",
    "SCYG_AGENT_DATABASE_URL",
    "SCYG_AGENT_JWT_PUBLIC_KEY_PATH",
    "SCYG_AGENT_JWT_ISSUER",
    "SCYG_AGENT_JWT_AUDIENCE",
    "SCYG_AGENT_JWT_SERVICE_SUBJECT",
    "SCYG_AGENT_PROVIDER_BASE_URL",
    "SCYG_AGENT_PROVIDER_API_KEY",
    "SCYG_AGENT_PROVIDER_MODEL",
    "SCYG_AGENT_BLOG_GRPC_TARGET",
)


def _subprocess_runner(label: str, command: tuple[str, ...], cwd: Path) -> CommandResult:
    """Run a bounded command and retain only its emitted diagnostics."""
    print(f"[agent QA] {label}")
    try:
        completed = subprocess.run(
            command,
            cwd=cwd,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
            timeout=900,
            check=False,
        )
    except subprocess.TimeoutExpired:
        output = sanitize(f"{label} timed out after 900 seconds")
        print(output)
        return CommandResult(124, output)
    except OSError as error:
        output = sanitize(f"{label} could not start: {error}")
        print(output)
        return CommandResult(127, output)
    output = sanitize((completed.stdout or "") + (completed.stderr or "")).strip()
    if output:
        encoding = sys.stdout.encoding or "utf-8"
        print(output.encode(encoding, errors="replace").decode(encoding, errors="replace"))
    return CommandResult(completed.returncode, output)


@final
class AgentQa:
    """Execute Agent gates in one deterministic order."""

    def __init__(
        self,
        root: Path,
        *,
        runner: CommandRunner = _subprocess_runner,
        tool_exists: Callable[[str], bool] = lambda name: shutil.which(name) is not None,
        cleanup: Cleanup | None = None,
        environment: dict[str, str] | None = None,
    ) -> None:
        self.root: Path = root
        self.agent: Path = root / AGENT_ROOT
        self.runner: CommandRunner = runner
        self.tool_exists: Callable[[str], bool] = tool_exists
        self.cleanup: Cleanup = cleanup or self._cleanup
        self.environment: dict[str, str] = environment or dict(os.environ)
        self.compose_project = f"scyg-agent-qa-{uuid.uuid4().hex[:12]}"

    def _cleanup(self, mode: str) -> None:
        """Remove Compose resources when the full topology owns them."""
        if mode != Mode.FULL or not self.tool_exists("docker"):
            return
        result = self.runner(
            "cleanup",
            (
                "docker",
                "compose",
                "-p",
                self.compose_project,
                "-f",
                "compose.yaml",
                "down",
                "--volumes",
                "--remove-orphans",
            ),
            self.agent,
        )
        if result.returncode != 0:
            raise OSError(result.output or "docker compose cleanup failed")

    def _preflight(self, mode: Mode) -> bool:
        """Reject missing tools or topology inputs before a false success."""
        required_tools = ("uv",) if mode is Mode.STATIC else ("uv", "docker")
        missing_tools = tuple(tool for tool in required_tools if not self.tool_exists(tool))
        if missing_tools:
            print(f"T32 topology prerequisites: missing tool(s): {', '.join(missing_tools)}")
            return False
        if mode is Mode.FULL:
            has_environment = all(self.environment.get(name) for name in REQUIRED_TOPOLOGY_ENV)
            has_env_file = (self.agent / ".env").is_file()
            if not has_environment and not has_env_file:
                message = (
                    "T32 topology prerequisites: Docker plus Compose secrets/approved endpoint "
                    "(.env or required environment) are required"
                )
                print(message)
                return False
        return True

    def _commands(self) -> tuple[tuple[str, tuple[str, ...]], ...]:
        """Return the static-safe Agent gate sequence."""
        uv = ("uv", "run", "--locked")
        return (
            ("agent-contracts", (*uv, "scyg-agent-contracts", "check")),
            ("ruff-check", (*uv, "ruff", "check", ".")),
            ("ruff-format", (*uv, "ruff", "format", "--check", ".")),
            ("basedpyright", (*uv, "basedpyright")),
            (
                "pytest-coverage",
                (*uv, "pytest", "-q", "--cov=src/scyg_agent", "--cov-branch"),
            ),
            ("no-excuse", (*uv, "python", "scripts/check_no_excuse.py", "src", "tests")),
            ("scope-scan", (*uv, "python", "scripts/check_qa_scope.py")),
            ("secret-scan", (*uv, "python", "scripts/check_agent_secrets.py")),
            (
                "dependency-audit",
                ("uv", "audit", "--preview-features", "audit-command", "--locked", "--no-dev"),
            ),
            ("deployment-contracts", (*uv, "pytest", "tests/deployment", "-q")),
        )

    def _run_sequence(self, commands: tuple[tuple[str, tuple[str, ...]], ...]) -> str | None:
        """Run commands in order and return the first failed label."""
        for label, command in commands:
            result = self.runner(label, command, self.agent)
            if result.returncode != 0:
                print(f"agent QA failed: {label}")
                return label
        return None

    def run(self, mode: Mode) -> int:
        """Run gates and always attempt cleanup without hiding the primary failure."""
        primary_failure: str | None = None
        try:
            if not self._preflight(mode):
                primary_failure = "T32 topology prerequisites"
            else:
                primary_failure = self._run_sequence(self._commands())
                if primary_failure is None and mode is Mode.FULL:
                    primary_failure = self._run_sequence(
                        (
                            (
                                "compose-config",
                                (
                                    "docker",
                                    "compose",
                                    "-p",
                                    self.compose_project,
                                    "-f",
                                    "compose.yaml",
                                    "config",
                                ),
                            ),
                            (
                                "t32-topology",
                                (
                                    "docker",
                                    "compose",
                                    "-p",
                                    self.compose_project,
                                    "-f",
                                    "compose.yaml",
                                    "up",
                                    "--build",
                                    "--detach",
                                    "--wait",
                                    "--wait-timeout",
                                    "120",
                                ),
                            ),
                        )
                    )
                    if primary_failure is None:
                        primary_failure = self._run_sequence(
                            (
                                (
                                    "t34-e2e",
                                    ("uv", "run", "--locked", "python", "scripts/t34_e2e.py"),
                                ),
                            )
                        )
        finally:
            try:
                self.cleanup(mode.value)
            except OSError as error:
                print(f"agent QA cleanup failed: {sanitize(str(error))}")
                if primary_failure is None:
                    primary_failure = "cleanup"
        if primary_failure is not None:
            if primary_failure != "T32 topology prerequisites":
                print(f"agent QA primary failure: {primary_failure}")
            return 1
        if mode is Mode.STATIC:
            print("agent QA: STATIC PASS")
        else:
            print("agent QA: PASS")
        return 0


def _arguments(arguments: Sequence[str]) -> Mode:
    """Parse the small public CLI surface."""
    normalized = tuple(arguments)
    if not normalized:
        return Mode.FULL
    if len(normalized) == MODE_ARGUMENT_COUNT and normalized[0] == "--mode":
        return Mode(normalized[1])
    raise SystemExit(USAGE)


def main(arguments: Sequence[str] | None = None) -> int:
    """Run the root-invoked Agent QA gate."""
    mode = _arguments(arguments or sys.argv[1:])
    return AgentQa(Path(__file__).resolve().parents[2]).run(mode)


if __name__ == "__main__":
    raise SystemExit(main())
