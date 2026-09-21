"""Reject accidental root Go/frontend product scope from Agent QA wiring."""

from __future__ import annotations

import sys
from pathlib import Path
from typing import Final

ROOT: Final[Path] = Path(__file__).resolve().parents[2]
FORBIDDEN_ROOT_MODULES: Final[tuple[str, ...]] = ("go.mod", "go.work")
FORBIDDEN_PRODUCT_TOKENS: Final[tuple[str, ...]] = (
    "go run ./backend",
    "pnpm --dir frontend",
    "npm --prefix frontend",
)


def main() -> int:
    """Check only the QA contract's static scope boundaries."""
    violations = [name for name in FORBIDDEN_ROOT_MODULES if (ROOT / name).exists()]
    for relative in ("Taskfile.yml", ".github/workflows/agent-quality.yml"):
        path = ROOT / relative
        if path.exists():
            text = path.read_text(encoding="utf-8")
            violations.extend(
                f"{relative}: {token}" for token in FORBIDDEN_PRODUCT_TOKENS if token in text
            )
    if violations:
        _ = sys.stderr.write("Agent QA scope violation: " + "; ".join(violations) + "\n")
        return 1
    _ = sys.stdout.write("Agent QA scope scan: clean\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
