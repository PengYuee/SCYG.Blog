"""Small repository-local no-excuse scan for authored Agent Python."""

from __future__ import annotations

import re
import sys
from pathlib import Path
from typing import Final

PATTERNS: Final[tuple[tuple[str, re.Pattern[str]], ...]] = (
    ("type-ignore", re.compile(r"#\s*(?:type|pyright):\s*ignore")),
    ("bare-except", re.compile(r"^\s*except\s*:\s*$")),
    ("asyncio", re.compile(r"^\s*(?:import asyncio|from asyncio import)")),
    ("pandas", re.compile(r"^\s*(?:import pandas|from pandas import)")),
    ("broad-except", re.compile(r"except\s+(?:Exception|BaseException)\b")),
)


def _scan_file(path: Path) -> list[str]:
    """Return no-excuse findings for one authored Python file."""
    violations: list[str] = []
    in_docstring = False
    for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        triple_quotes = line.count('"""') + line.count("'''")
        if in_docstring:
            in_docstring = triple_quotes % 2 == 0
            continue
        if triple_quotes % 2 == 1:
            in_docstring = True
            continue
        for name, pattern in PATTERNS:
            exemptions = {f"{name.upper().replace('-', '_')}_OK"}
            if name == "asyncio":
                exemptions.add("ANYIO_OK")
            if pattern.search(line) and not any(token in line for token in exemptions):
                violations.append(f"{path}:{line_number}: {name}")
    return violations


def main(arguments: list[str]) -> int:
    """Scan supplied directories while honoring generated and explicit exemptions."""
    violations: list[str] = []
    for argument in arguments:
        for path in sorted(Path(argument).rglob("*.py")):
            if "generated" in path.parts or path.name == "check_no_excuse.py":
                continue
            violations.extend(_scan_file(path))
    if violations:
        _ = sys.stderr.write("\n".join(violations) + "\n")
        return 1
    _ = sys.stdout.write("Agent no-excuse scan: clean\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
