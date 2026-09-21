"""Minimal T34 harness contract tests; no test claims live E2E success."""

import json
from pathlib import Path

import pytest
from scripts import t34_s1_driver, t34_s2_driver
from scripts.t34_e2e import (
    DRIVER_FILES,
    TOPOLOGY_INPUTS,
    Evidence,
    Scenario,
    driver_command,
    load_scenarios,
    preflight,
    run,
)
from scripts.t34_http_driver import Fixture, SseEvent, reconnect_observation
from scripts.t34_s1_driver import main as run_s1

from scyg_agent.redaction import sanitize


def test_manifest_contains_only_ordered_routine_scenarios() -> None:
    # Given: the versioned routine manifest.
    scenarios = load_scenarios()

    # When: its identities and evidence roots are inspected.
    identifiers = tuple(item.identifier for item in scenarios)

    # Then: only S1-S3 are required in order.
    assert identifiers == ("S1", "S2", "S3")
    assert tuple(item.evidence for item in scenarios) == (
        "scenario-S1",
        "scenario-S2",
        "scenario-S3",
    )


def test_preflight_requires_only_agent_fixture_data() -> None:
    # Given: Docker is available but one routine fixture input is absent.
    environment: dict[str, str] = dict.fromkeys(TOPOLOGY_INPUTS, "configured")
    del environment["SCYG_T34_BEARER_TOKEN"]

    # When: the minimal prerequisite boundary is checked.
    diagnostic = preflight(environment, lambda _name: "docker")

    # Then: the missing data input is named and no driver command is configurable.
    assert diagnostic == "T34 E2E prerequisites: missing approved input(s): SCYG_T34_BEARER_TOKEN"


def test_preflight_fails_before_driver_when_docker_is_missing() -> None:
    # Given: all minimal inputs but no Docker executable.
    environment: dict[str, str] = dict.fromkeys(TOPOLOGY_INPUTS, "configured")

    # When: preflight runs.
    diagnostic = preflight(environment, lambda name: None if name == "docker" else "tool")

    # Then: Docker is the first named blocker.
    assert diagnostic == "T34 E2E prerequisites: missing tool(s): docker"


def test_fixed_driver_command_ignores_environment_argv_and_is_repository_owned(
    tmp_path: Path,
) -> None:
    # Given: the three fixed driver files under the Agent scripts directory.
    scripts = tmp_path / "agent/scripts"
    scripts.mkdir(parents=True)
    for name in DRIVER_FILES.values():
        _ = (scripts / name).touch()
    evidence = Evidence(tmp_path / "evidence")

    # When: S1 command construction receives hostile environment-like data.
    command = driver_command(tmp_path, Scenario("S1", "scenario-S1"), evidence)

    # Then: only the fixed interpreter and owned S1 entrypoint are selected.
    assert command[0] != "hostile.exe"
    assert command[1] == "-m"
    assert command[2:4] == ("scripts.t34_s1_driver", "--evidence-root")
    assert command[4] == str(evidence.root / "scenario-S1")


def test_missing_driver_or_fixture_fails_before_receipt(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # Given: no owned driver file and no HTTP fixture values.
    evidence = Evidence(tmp_path / "evidence")
    for name in TOPOLOGY_INPUTS:
        monkeypatch.delenv(name, raising=False)

    # When: resolution and the fixed S1 entrypoint run.
    with pytest.raises(RuntimeError, match="repository driver"):
        _ = driver_command(tmp_path, Scenario("S1", "scenario-S1"), evidence)
    exit_code = run_s1(["--evidence-root", str(tmp_path / "scenario-S1")])

    # Then: both fail before any passed receipt can be written.
    assert exit_code == 1
    assert not (tmp_path / "scenario-S1/receipt.json").exists()


def test_s1_driver_writes_receipt_after_simple_snapshot_and_terminal_sse(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # Given: a SIMPLE snapshot and the current named terminal SSE event shape.
    value = Fixture("http://127.0.0.1:8080", "run-1", "test-token")
    frames = (SseEvent(1, "event-1", "run_succeeded"),)
    observations: list[str] = []

    def fake_fixture(_scenario: str) -> Fixture:
        return value

    def fake_sse(_value: Fixture, cursor: int) -> tuple[SseEvent, ...] | None:
        return frames if cursor == 0 else None

    def fake_simple_snapshot(_value: Fixture) -> bool:
        return True

    def fake_receipt(_root: Path, _scenario: str, observation: str) -> None:
        observations.append(observation)

    monkeypatch.setattr(t34_s1_driver, "fixture", fake_fixture)
    monkeypatch.setattr(t34_s1_driver, "simple_snapshot", fake_simple_snapshot)
    monkeypatch.setattr(t34_s1_driver, "sse", fake_sse)
    monkeypatch.setattr(t34_s1_driver, "write_receipt", fake_receipt)

    # When: the fixed S1 driver receives a terminal SseEvent.
    exit_code = t34_s1_driver.main(["--evidence-root", str(tmp_path)])

    # Then: it records the terminal event through named fields, not tuple indexing.
    assert exit_code == 0
    assert observations == ["terminal=run_succeeded events=1"]


def test_s2_driver_closes_first_stream_then_reconnects_from_its_cursor(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # Given: a first SSE connection that observes cursor 9 and a terminal replay after it.
    value = Fixture("http://127.0.0.1:8080", "run-1", "test-token")
    initial = SseEvent(9, "event-9", "text_delta")
    replay = (SseEvent(10, "event-10", "run_succeeded"),)
    reconnect_cursors: list[int] = []
    observations: list[str] = []

    def fake_fixture(_scenario: str) -> Fixture:
        return value

    def fake_initial_sse(_value: Fixture, cursor: int) -> SseEvent | None:
        return initial if cursor == 7 else None

    def fake_sse(_value: Fixture, cursor: int) -> tuple[SseEvent, ...]:
        reconnect_cursors.append(cursor)
        return replay

    def fake_receipt(_root: Path, _scenario: str, observation: str) -> None:
        observations.append(observation)

    monkeypatch.setenv("SCYG_T34_S2_CURSOR", "7")
    monkeypatch.setattr(t34_s2_driver, "fixture", fake_fixture)
    monkeypatch.setattr(t34_s2_driver, "initial_sse", fake_initial_sse)
    monkeypatch.setattr(t34_s2_driver, "sse", fake_sse)
    monkeypatch.setattr(t34_s2_driver, "write_receipt", fake_receipt)

    # When: the owned S2 entrypoint runs.
    exit_code = t34_s2_driver.main(["--evidence-root", str(tmp_path)])

    # Then: the second request starts exactly after the first connection's persisted cursor.
    assert exit_code == 0
    assert reconnect_cursors == [9]
    assert observations == [
        "first_cursor=9 reconnect_cursor=9 second_sequence=10 no_duplicate_ids=true"
    ]


def test_s2_driver_rejects_initial_cursor_that_does_not_advance_request(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # Given: the first returned cursor repeats the requested cursor before a valid-looking replay.
    value = Fixture("http://127.0.0.1:8080", "run-1", "test-token")
    reconnect_cursors: list[int] = []
    observations: list[str] = []

    def fake_fixture(_scenario: str) -> Fixture:
        return value

    def fake_initial_sse(_value: Fixture, cursor: int) -> SseEvent | None:
        return SseEvent(cursor, "event-7", "text_delta")

    def fake_sse(_value: Fixture, cursor: int) -> tuple[SseEvent, ...]:
        reconnect_cursors.append(cursor)
        return (SseEvent(8, "event-8", "run_succeeded"),)

    def fake_receipt(_root: Path, _scenario: str, observation: str) -> None:
        observations.append(observation)

    monkeypatch.setenv("SCYG_T34_S2_CURSOR", "7")
    monkeypatch.setattr(t34_s2_driver, "fixture", fake_fixture)
    monkeypatch.setattr(t34_s2_driver, "initial_sse", fake_initial_sse)
    monkeypatch.setattr(t34_s2_driver, "sse", fake_sse)
    monkeypatch.setattr(t34_s2_driver, "write_receipt", fake_receipt)

    # When: the owned S2 entrypoint sees 7 -> 7 -> 8.
    exit_code = t34_s2_driver.main(["--evidence-root", str(tmp_path)])

    # Then: it rejects the stale initial cursor before reconnecting or writing a receipt.
    assert exit_code == 1
    assert reconnect_cursors == []
    assert observations == []


@pytest.mark.parametrize(
    ("requested_cursor", "initial", "replay"),
    [
        (3, SseEvent(3, "event-3", "text_delta"), (SseEvent(4, "event-4", "run_succeeded"),)),
        (2, SseEvent(3, "event-3", "text_delta"), None),
        (2, SseEvent(3, "event-3", "text_delta"), (SseEvent(3, "event-4", "run_succeeded"),)),
        (2, SseEvent(3, "event-3", "text_delta"), (SseEvent(4, "event-3", "run_succeeded"),)),
        (
            2,
            SseEvent(3, "event-3", "text_delta"),
            (SseEvent(4, "event-4", "text_delta"),),
        ),
    ],
)
def test_s2_reconnect_observation_rejects_missing_or_non_monotonic_replay(
    requested_cursor: int, initial: SseEvent, replay: tuple[SseEvent, ...] | None
) -> None:
    # Given: an absent reconnect, cursor regression, duplicate identity, or incomplete replay.
    # When/Then: no passing observation can be issued.
    assert reconnect_observation(requested_cursor, initial, replay) is None


def test_receipt_and_sanitization_fail_closed(tmp_path: Path) -> None:
    # Given: a receipt with the wrong scenario identity and secret-shaped output.
    evidence = Evidence(tmp_path)
    _ = (tmp_path / "scenario-S1").mkdir()
    _ = (tmp_path / "scenario-S1/receipt.json").write_text(
        json.dumps(
            {
                "scenario": "S2",
                "passed": True,
                "surface_observation": "terminal=run_succeeded",
                "cleanup_confirmed": True,
            }
        ),
        encoding="utf-8",
    )

    # When/Then: receipt verification rejects the wrong identity.
    with pytest.raises(RuntimeError, match="receipt"):
        evidence.verify(load_scenarios()[0])
    assert sanitize("password=secret") == "password=[REDACTED]"
    assert sanitize("jwt=secret") == "jwt=[REDACTED]"
    assert "eyJheader" not in sanitize("Bearer eyJheader.payload.signature")


def test_evidence_write_sanitizes_failure_diagnostic(tmp_path: Path) -> None:
    # Given: a failure diagnostic contains labeled secret-shaped values.
    evidence = Evidence(tmp_path)
    field_value = "t34-failure-value-7291"
    jwt = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ0MzQifQ.t34-signature-7291"

    # When: the evidence boundary persists the diagnostic.
    evidence.write(
        "failure.txt",
        f"password={field_value}\nAuthorization: Bearer {jwt}\nservice: agent",
    )
    persisted = (tmp_path / "failure.txt").read_text(encoding="utf-8")

    # Then: stored evidence preserves labels and excludes every sensitive value.
    assert "password=[REDACTED]" in persisted
    assert "Authorization: [REDACTED]" in persisted
    assert "service: agent" in persisted
    assert field_value not in persisted
    assert jwt not in persisted


def test_receipt_true_without_required_scenario_observation_is_rejected(tmp_path: Path) -> None:
    # Given: a true S1 receipt that omits the terminal SSE observation.
    evidence = Evidence(tmp_path)
    _ = (tmp_path / "scenario-S1").mkdir()
    _ = (tmp_path / "scenario-S1/receipt.json").write_text(
        json.dumps(
            {
                "scenario": "S1",
                "passed": True,
                "surface_observation": "observed",
                "cleanup_confirmed": True,
            }
        ),
        encoding="utf-8",
    )

    # When/Then: the harness refuses the unsupported true receipt.
    with pytest.raises(RuntimeError, match="receipt"):
        evidence.verify(load_scenarios()[0])


def test_run_stops_at_missing_docker_and_writes_only_failure_diagnostic(tmp_path: Path) -> None:
    # Given: no Docker and a driver that must never be invoked.
    calls: list[str] = []

    def driver(label: str, command: tuple[str, ...], cwd: Path) -> int:
        _ = command, cwd
        calls.append(label)
        return 0

    # When: the routine runner starts.
    exit_code = run(tmp_path, {}, driver)

    # Then: it fails before S1 and records only the sanitized diagnostic.
    assert exit_code == 1
    assert calls == []
    failure_paths = tuple((tmp_path / ".omo/evidence/scyg-agent-service/T34").glob("*/failure.txt"))
    assert len(failure_paths) == 1
