"""Run domain dependency boundary tests."""

import ast
from pathlib import Path
from typing import Final

DOMAIN_ROOT: Final = Path(__file__).parents[3] / "src" / "scyg_agent" / "domain" / "runs"
FORBIDDEN_PREFIXES: Final = (
    "fastapi",
    "grpc",
    "sqlalchemy",
    "langgraph",
    "deepagents",
    "scyg_agent.generated",
)


def test_run_domain_has_no_runtime_persistence_or_transport_imports() -> None:
    # Given: every authored source file in the pure Run domain.
    imported_modules: list[tuple[Path, str]] = []
    for path in sorted(DOMAIN_ROOT.glob("*.py")):
        tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                imported_modules.extend((path, alias.name) for alias in node.names)
                continue
            if isinstance(node, ast.ImportFrom) and node.module is not None:
                imported_modules.append((path, node.module))

    # When: imports are compared with forbidden adapter/framework boundaries.
    violations = [
        f"{path.name}: {module}"
        for path, module in imported_modules
        if module.startswith(FORBIDDEN_PREFIXES)
    ]

    # Then: the domain remains standard-library-only and transport-independent.
    assert violations == []
