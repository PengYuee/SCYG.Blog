"""Transactional generated-tree replacement tests."""

import shutil
from pathlib import Path

import pytest

from scyg_agent import contract_generation

GENERATION_FAILURE = "injected generation failure"
ROLLBACK_FAILURE = "injected rollback failure"
SWAP_FAILURE = "injected swap failure"


def _tree_bytes(root: Path) -> dict[str, bytes]:
    """Snapshot a fixture tree by relative path and bytes."""
    return {
        path.relative_to(root).as_posix(): path.read_bytes()
        for path in sorted(root.rglob("*"))
        if path.is_file() and "__pycache__" not in path.parts and path.suffix != ".pyc"
    }


def test_write_restores_current_tree_when_replacement_fails(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: an existing generated tree and fresh generated output.
    repository = tmp_path / "repository"
    target = repository / "agent" / "src" / "scyg_agent" / "generated" / "proto"
    target.mkdir(parents=True)
    _ = (target / "current.py").write_bytes(b"current-generated-tree\n")
    before = _tree_bytes(target)

    def generate(destination: Path) -> None:
        """Create deterministic fresh output without invoking the network."""
        _ = (destination / "fresh.py").write_bytes(b"fresh-generated-tree\n")

    original_rename = Path.rename

    def fail_replacement(source: Path, destination: Path) -> Path:
        """Fail staged-to-target rename after the current tree becomes backup."""
        if source.name.startswith(".generated-staged-") and destination == target:
            raise OSError(SWAP_FAILURE)
        return original_rename(source, destination)

    monkeypatch.setattr(contract_generation, "repository_root", lambda: repository)
    monkeypatch.setattr(contract_generation, "generate_contracts", generate)
    monkeypatch.setattr(Path, "rename", fail_replacement)

    # When: the actual write transaction fails at the replacement boundary.
    with pytest.raises(OSError, match=SWAP_FAILURE):
        _ = contract_generation.write_contracts()

    # Then: the prior tree remains byte-identical and no transaction residue remains.
    assert target.is_dir()
    assert _tree_bytes(target) == before
    assert not tuple(target.parent.glob(".generated-staged-*"))
    assert not tuple(target.parent.glob(".generated-backup-*"))


def test_write_leaves_current_tree_when_generation_fails(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: an existing tree and a generator that fails before staging.
    repository = tmp_path / "repository"
    target = repository / "agent" / "src" / "scyg_agent" / "generated" / "proto"
    target.mkdir(parents=True)
    _ = (target / "current.py").write_bytes(b"current-generated-tree\n")
    before = _tree_bytes(target)

    def fail_generation(destination: Path) -> None:
        """Fail before the write transaction creates sibling directories."""
        assert destination.is_dir()
        raise OSError(GENERATION_FAILURE)

    monkeypatch.setattr(contract_generation, "repository_root", lambda: repository)
    monkeypatch.setattr(contract_generation, "generate_contracts", fail_generation)

    # When: fresh generation fails before staging.
    with pytest.raises(OSError, match=GENERATION_FAILURE):
        _ = contract_generation.write_contracts()

    # Then: the current tree remains exact and no transaction residue exists.
    assert _tree_bytes(target) == before
    assert not tuple(target.parent.glob(".generated-staged-*"))
    assert not tuple(target.parent.glob(".generated-backup-*"))


def test_write_preserves_backup_when_swap_and_rollback_fail(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: an existing tree and failures for both swap and rollback renames.
    repository = tmp_path / "repository"
    target = repository / "agent" / "src" / "scyg_agent" / "generated" / "proto"
    target.mkdir(parents=True)
    _ = (target / "current.py").write_bytes(b"current-generated-tree\n")
    before = _tree_bytes(target)

    def generate(destination: Path) -> None:
        """Create deterministic fresh output without invoking the network."""
        _ = (destination / "fresh.py").write_bytes(b"fresh-generated-tree\n")

    original_rename = Path.rename

    def fail_swap_and_rollback(source: Path, destination: Path) -> Path:
        """Fail staged commit and the subsequent backup restoration."""
        if source.name.startswith(".generated-staged-") and destination == target:
            raise OSError(SWAP_FAILURE)
        if source.name.startswith(".generated-backup-") and destination == target:
            raise OSError(ROLLBACK_FAILURE)
        return original_rename(source, destination)

    monkeypatch.setattr(contract_generation, "repository_root", lambda: repository)
    monkeypatch.setattr(contract_generation, "generate_contracts", generate)
    monkeypatch.setattr(Path, "rename", fail_swap_and_rollback)

    # When: commit and rollback both fail at filesystem rename boundaries.
    with pytest.raises(contract_generation.GeneratedTreeRollbackError) as caught:
        _ = contract_generation.write_contracts()

    # Then: the sole old-tree backup remains byte-exact and discoverable.
    backups = tuple(target.parent.glob(".generated-backup-*"))
    assert not target.exists()
    assert len(backups) == 1
    assert _tree_bytes(backups[0]) == before
    assert caught.value.backup == backups[0]
    assert str(caught.value.swap_error) == SWAP_FAILURE
    assert str(caught.value.rollback_error) == ROLLBACK_FAILURE
    assert str(backups[0]) in str(caught.value)
    assert not tuple(target.parent.glob(".generated-staged-*"))


def test_write_commits_complete_generated_tree(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    # Given: an existing tree and the complete generated Python/typing artifacts.
    repository = tmp_path / "repository"
    target = repository / "agent" / "src" / "scyg_agent" / "generated" / "proto"
    target.mkdir(parents=True)
    _ = (target / "current.py").write_bytes(b"current-generated-tree\n")
    source = Path(contract_generation.__file__).parent / "generated" / "proto"
    expected = _tree_bytes(source)

    def generate(destination: Path) -> None:
        """Copy the real generated fixture into Buf's temporary destination."""
        _ = shutil.copytree(source, destination, dirs_exist_ok=True)

    monkeypatch.setattr(contract_generation, "repository_root", lambda: repository)
    monkeypatch.setattr(contract_generation, "generate_contracts", generate)

    # When: the actual write transaction commits successfully.
    exit_code = contract_generation.write_contracts()

    # Then: the complete fixture is current and transaction siblings are absent.
    assert exit_code == 0
    assert "current.py" not in _tree_bytes(target)
    assert _tree_bytes(target) == expected
    assert contract_generation.report_drift(_tree_bytes(target), expected) == 0
    assert not tuple(target.parent.glob(".generated-staged-*"))
    assert not tuple(target.parent.glob(".generated-backup-*"))
