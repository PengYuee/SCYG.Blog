"""Deterministic Python protobuf generation and drift checking."""

from __future__ import annotations

import hashlib
import shutil
import subprocess
import sys
import tempfile
import uuid
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING, Final

if TYPE_CHECKING:
    from collections.abc import Mapping, Sequence

BUF_PACKAGE: Final = "@bufbuild/buf@1.47.2"
PACKAGE_IMPORT: Final = "from scyg."
OWNED_PACKAGE_IMPORT: Final = "from scyg_agent.generated.scyg."
VALIDATE_IMPORT: Final = "from buf."
OWNED_VALIDATE_IMPORT: Final = "from scyg_agent.generated.buf."
PACKAGE_INITIALIZER: Final = '"""Generated protobuf package."""\n'


class GeneratedTreeRollbackError(OSError):
    """Report a failed generated-tree swap whose backup could not be restored."""

    # backup identifies the preserved old generated tree for manual recovery.
    backup: Path
    # swap_error is the original staged-to-target rename failure.
    swap_error: OSError
    # rollback_error is the subsequent backup-to-target rename failure.
    rollback_error: OSError

    def __init__(self, backup: Path, swap_error: OSError, rollback_error: OSError) -> None:
        """Preserve both filesystem failures and the recoverable backup path."""
        super().__init__(f"generated tree rollback failed; backup preserved at {backup}")
        self.backup = backup
        self.swap_error = swap_error
        self.rollback_error = rollback_error


@dataclass(frozen=True, slots=True)
class GeneratedTrees:
    """Paths participating in one isolated generation operation."""

    repository: Path
    expected: Path
    generated: Path


def repository_root() -> Path:
    """Return the repository root from the installed Agent source tree."""
    return Path(__file__).resolve().parents[3]


def generate_contracts(destination: Path) -> None:
    """Generate and package Python bindings in an isolated destination."""
    repository = repository_root()
    contracts = repository / "contracts"
    npx = shutil.which("npx")
    if npx is None:
        _ = sys.stderr.write("npx is required for pinned Buf generation\n")
        raise SystemExit(2)
    command = (
        npx,
        "--yes",
        BUF_PACKAGE,
        "generate",
        "--template",
        "buf.gen.yaml",
        "--output",
        str(destination),
        "--include-imports",
        "--timeout",
        "2m",
    )
    # The executable is PATH-resolved and every argument is repository-owned or constant.
    _ = subprocess.run(command, cwd=contracts, check=True, timeout=150)  # noqa: S603

    for path in sorted(path for path in destination.rglob("*") if path.suffix in {".py", ".pyi"}):
        source = path.read_text(encoding="utf-8")
        _ = path.write_text(
            source.replace(PACKAGE_IMPORT, OWNED_PACKAGE_IMPORT).replace(
                VALIDATE_IMPORT, OWNED_VALIDATE_IMPORT
            ),
            encoding="utf-8",
            newline="\n",
        )
    packages = (destination, *sorted(path for path in destination.rglob("*") if path.is_dir()))
    for package in packages:
        _ = (package / "__init__.py").write_text(
            PACKAGE_INITIALIZER,
            encoding="utf-8",
            newline="\n",
        )


def _file_bytes(root: Path) -> Mapping[str, bytes]:
    """Read a generated tree as stable POSIX-relative byte entries."""
    return {
        path.relative_to(root).as_posix(): path.read_bytes()
        for path in sorted(root.rglob("*"))
        if path.is_file() and "__pycache__" not in path.parts and path.suffix != ".pyc"
    }


def _trees(temporary_root: Path) -> GeneratedTrees:
    """Resolve expected and fresh generated trees."""
    repository = repository_root()
    return GeneratedTrees(
        repository=repository,
        expected=repository / "agent" / "src" / "scyg_agent" / "generated",
        generated=temporary_root,
    )


def report_drift(expected: Mapping[str, bytes], actual: Mapping[str, bytes]) -> int:
    """Report every missing, extra, or byte-changed generated file."""
    expected_paths = set(expected)
    actual_paths = set(actual)
    missing = sorted(actual_paths - expected_paths)
    extra = sorted(expected_paths - actual_paths)
    changed = sorted(
        path for path in expected_paths & actual_paths if expected[path] != actual[path]
    )
    for category, paths in (("missing", missing), ("extra", extra), ("changed", changed)):
        for path in paths:
            _ = sys.stderr.write(f"{category}: {path}\n")
    if missing or extra or changed:
        return 1
    _ = sys.stdout.write(f"generated contracts are current ({len(actual)} files)\n")
    return 0


def _check() -> int:
    """Generate fresh bindings and byte-compare them with the committed tree."""
    with tempfile.TemporaryDirectory(prefix="scyg-contract-check-") as temporary:
        trees = _trees(Path(temporary))
        generate_contracts(Path(temporary))
        return report_drift(_file_bytes(trees.expected), _file_bytes(trees.generated))


def write_contracts() -> int:
    """Atomically replace the owned generated tree from fresh Buf output."""
    with tempfile.TemporaryDirectory(prefix="scyg-contract-generate-") as temporary:
        trees = _trees(Path(temporary))
        target = trees.expected
        generate_contracts(Path(temporary))
        transaction_id = uuid.uuid4().hex
        staged = target.parent / f".generated-staged-{transaction_id}"
        backup = target.parent / f".generated-backup-{transaction_id}"
        _ = shutil.copytree(trees.generated, staged)
        target_existed = target.exists()
        backup_created = False
        commit_succeeded = False
        rollback_succeeded = False
        try:
            if target_existed:
                _ = target.rename(backup)
                backup_created = True
            try:
                _ = staged.rename(target)
                commit_succeeded = True
            except OSError as swap_error:
                if backup_created:
                    try:
                        _ = backup.rename(target)
                        rollback_succeeded = True
                    except OSError as rollback_error:
                        raise GeneratedTreeRollbackError(
                            backup,
                            swap_error,
                            rollback_error,
                        ) from rollback_error
                raise
        finally:
            if staged.exists():
                shutil.rmtree(staged)
            if backup.exists() and (commit_succeeded or rollback_succeeded):
                shutil.rmtree(backup)
        manifest = _file_bytes(target)
        aggregate = hashlib.sha256(
            b"".join(path.encode() + b"\0" + manifest[path] for path in sorted(manifest))
        ).hexdigest()
        _ = sys.stdout.write(f"generated {len(manifest)} files (sha256 {aggregate})\n")
    return 0


def run(arguments: Sequence[str]) -> int:
    """Run the bounded generation command."""
    normalized = tuple(arguments)
    if normalized == ("generate",):
        return write_contracts()
    if normalized == ("check",):
        return _check()
    _ = sys.stderr.write("usage: scyg-agent-contracts {generate|check}\n")
    return 2


def main() -> None:
    """Run contract generation using process arguments."""
    raise SystemExit(run(sys.argv[1:]))
