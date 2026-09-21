"""Minimal routine Agent E2E gate for S1-S3."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import uuid
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import ClassVar, Final, Protocol

from pydantic import BaseModel, ConfigDict, Field, ValidationError

from scyg_agent.redaction import sanitize

MANIFEST: Final = Path(__file__).with_name("t34_scenarios.json")
SCENARIO_IDS: Final = ("S1", "S2", "S3")
TOPOLOGY_INPUTS: Final = (
    "SCYG_T34_HTTP_BASE_URL",
    "SCYG_T34_RUN_ID",
    "SCYG_T34_BEARER_TOKEN",
)
DRIVER_FILES: Final = {
    "S1": "t34_s1_driver.py",
    "S2": "t34_s2_driver.py",
    "S3": "t34_s3_driver.py",
}
OBSERVATION_PREFIXES: Final = {
    "S1": "terminal=",
    "S2": "first_cursor=",
    "S3": "cancel replay=",
}


class Driver(Protocol):
    """Run one bounded, non-shell, owned scenario driver."""

    def __call__(self, label: str, command: tuple[str, ...], cwd: Path) -> int: ...


class ManifestError(ValueError):
    """Signal an invalid T34 manifest."""


class DriverError(RuntimeError):
    """Signal a missing repository-owned driver."""


class ReceiptError(RuntimeError):
    """Signal a missing, malformed or failed driver receipt."""


class ManifestScenario(BaseModel):
    """Parse one minimal scenario at the JSON boundary."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid")
    id: str = Field(pattern=r"^S[1-3]$")
    name: str = Field(min_length=1)
    observation: str = Field(min_length=1)
    evidence: str = Field(pattern=r"^scenario-S[1-3]$")


class Manifest(BaseModel):
    """Parse the versioned three-scenario manifest."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid")
    version: int = Field(strict=True, ge=1)
    scenarios: tuple[ManifestScenario, ...]


class Receipt(BaseModel):
    """Validate the one receipt produced by each real-surface driver."""

    model_config: ClassVar[ConfigDict] = ConfigDict(frozen=True, extra="forbid")
    scenario: str = Field(pattern=r"^S[1-3]$")
    passed: bool
    surface_observation: str = Field(min_length=1)
    cleanup_confirmed: bool


@dataclass(frozen=True, slots=True)
class Scenario:
    """Loaded scenario identity and evidence directory."""

    identifier: str
    evidence: str


def load_scenarios(path: Path = MANIFEST) -> tuple[Scenario, ...]:
    """Load exactly the ordered routine S1-S3 contracts."""
    try:
        manifest = Manifest.model_validate(json.loads(path.read_text(encoding="utf-8")))
    except (OSError, json.JSONDecodeError, ValidationError) as error:
        message = "T34 manifest is invalid"
        raise ManifestError(message) from error
    scenarios = tuple(Scenario(item.id, item.evidence) for item in manifest.scenarios)
    if manifest.version != 1 or tuple(item.identifier for item in scenarios) != SCENARIO_IDS:
        message = "T34 manifest must contain ordered S1-S3 version 1"
        raise ManifestError(message)
    return scenarios


def driver_command(root: Path, scenario: Scenario, evidence: Evidence) -> tuple[str, ...]:
    """Resolve one fixed repository-owned driver below agent/scripts."""
    scripts = (root / "agent" / "scripts").resolve()
    path = (scripts / DRIVER_FILES[scenario.identifier]).resolve()
    if scripts not in path.parents or not path.is_file():
        message = f"T34 {scenario.identifier} repository driver is missing"
        raise DriverError(message)
    return (
        sys.executable,
        "-m",
        f"scripts.{path.stem}",
        "--evidence-root",
        str(evidence.root / scenario.evidence),
    )


class Evidence:
    """Own one invocation directory and persist only sanitized content."""

    def __init__(self, root: Path) -> None:
        self.root: Path = root
        _ = root.mkdir(parents=True, exist_ok=True)

    def write(self, name: str, content: str) -> None:
        """Write one sanitized artifact."""
        target = self.root / name
        _ = target.parent.mkdir(parents=True, exist_ok=True)
        _ = target.write_text(sanitize(content) + "\n", encoding="utf-8")

    def verify(self, scenario: Scenario) -> None:
        """Require and validate the driver's structured receipt."""
        path = self.root / scenario.evidence / "receipt.json"
        try:
            receipt = Receipt.model_validate_json(path.read_text(encoding="utf-8"))
        except (OSError, ValidationError, ValueError) as error:
            message = f"T34 {scenario.identifier} receipt is invalid or missing"
            raise ReceiptError(message) from error
        self.write(f"{scenario.evidence}/receipt.json", receipt.model_dump_json())
        if (
            receipt.scenario != scenario.identifier
            or not receipt.passed
            or not receipt.cleanup_confirmed
            or not receipt.surface_observation.startswith(OBSERVATION_PREFIXES[scenario.identifier])
        ):
            message = f"T34 {scenario.identifier} receipt did not pass"
            raise ReceiptError(message)


def preflight(
    environment: dict[str, str], tool_exists: Callable[[str], str | None] = shutil.which
) -> str | None:
    """Require Docker and only routine Agent fixture data."""
    if tool_exists("docker") is None:
        return "T34 E2E prerequisites: missing tool(s): docker"
    missing = tuple(name for name in TOPOLOGY_INPUTS if not environment.get(name))
    if missing:
        return f"T34 E2E prerequisites: missing approved input(s): {', '.join(missing)}"
    return None


def _load_environment(root: Path, environment: dict[str, str] | None) -> dict[str, str]:
    """Read local Compose dotenv values, while explicit process values win."""
    values: dict[str, str] = {}
    dotenv = root / "agent" / ".env"
    if dotenv.is_file():
        for raw_line in dotenv.read_text(encoding="utf-8").splitlines():
            line = raw_line.strip()
            if line and not line.startswith("#") and "=" in line:
                key, value = line.split("=", 1)
                values[key.strip()] = value.strip().strip('"')
    values.update(dict(os.environ) if environment is None else environment)
    return values


def _subprocess(label: str, command: tuple[str, ...], cwd: Path) -> int:
    """Run one bounded driver without a shell."""
    try:
        completed = subprocess.run(
            command, cwd=cwd, capture_output=True, text=True, timeout=900, check=False
        )
    except (OSError, subprocess.TimeoutExpired) as error:
        print(f"{label}: {sanitize(str(error))}")
        return 1
    output = sanitize((completed.stdout or "") + (completed.stderr or ""))
    if output:
        print(output)
    return completed.returncode


def run(root: Path, environment: dict[str, str] | None = None, driver: Driver = _subprocess) -> int:
    """Run S1, S2 and S3 in order and fail closed on any driver or receipt failure."""
    values = _load_environment(root, environment)
    invocation = uuid.uuid4().hex
    evidence = Evidence(root / ".omo" / "evidence" / "scyg-agent-service" / "T34" / invocation)
    diagnostic = preflight(values)
    if diagnostic:
        evidence.write("failure.txt", diagnostic)
        print(diagnostic)
        return 1
    try:
        for scenario in load_scenarios():
            command = driver_command(root, scenario, evidence)
            if driver(scenario.identifier, command, root / "agent"):
                message = f"T34 {scenario.identifier} driver failed"
                evidence.write("failure.txt", message)
                print(f"T34 E2E failed: {message}")
                return 1
            evidence.verify(scenario)
    except (DriverError, ManifestError, ReceiptError) as error:
        evidence.write("failure.txt", str(error))
        print(f"T34 E2E failed: {sanitize(str(error))}")
        return 1
    print("T34 E2E: PASS")
    return 0


if __name__ == "__main__":
    raise SystemExit(run(Path(__file__).resolve().parents[2]))
