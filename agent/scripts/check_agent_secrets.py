"""Scan deployment contracts for embedded credentials or private key material."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from typing import Final

ROOT: Final[Path] = Path(__file__).resolve().parents[1]
FILES: Final[tuple[str, ...]] = (
    "Dockerfile",
    "compose.yaml",
    ".env.example",
    "src/scyg_agent/deployment.py",
    "src/scyg_agent/deployment_contracts.py",
)
SECRET_PATTERNS: Final[tuple[re.Pattern[str], ...]] = (
    re.compile(r"BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY"),
    re.compile(r"(?i)(?:password|api[_-]?key|token)\s*[:=]\s*['\"][^$?\s]"),
)


def main() -> int:
    """Reject usable secret literals while allowing variable placeholders."""
    violations: list[str] = []
    for relative in FILES:
        path = ROOT / relative
        text = path.read_text(encoding="utf-8")
        if any(pattern.search(text) for pattern in SECRET_PATTERNS):
            violations.append(relative)
        if relative == ".env.example" and ("postgresql://" in text or "PRIVATE KEY" in text):
            violations.append(relative)
    if violations:
        _ = sys.stderr.write(
            "Agent secret scan violation: " + ", ".join(sorted(set(violations))) + "\n"
        )
        return 1
    _ = sys.stdout.write("Agent secret scan: clean\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
