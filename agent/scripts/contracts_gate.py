"""Lint contracts and enforce compatibility against an explicit accepted baseline."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SHA_LENGTH = 40
BUF_ANNOTATION_EXIT_CODE = 100


def commit_sha(value: str, label: str) -> str:
    if len(value) != SHA_LENGTH or any(character not in "0123456789abcdef" for character in value):
        message = f"{label} must be an actual full commit SHA"
        raise SystemExit(message)
    subprocess.run(("git", "rev-parse", "--verify", f"{value}^{{commit}}"), cwd=ROOT, check=True)
    return value


def main() -> int:
    npx = shutil.which("npx")
    if npx is None:
        message = "npx is required for Buf 1.47.2"
        raise SystemExit(message)
    buf = (npx, "--yes", "@bufbuild/buf@1.47.2")
    subprocess.run((*buf, "lint", "contracts"), cwd=ROOT, check=True, timeout=150)
    accepted = commit_sha(os.environ.get("SCYG_CONTRACTS_ACCEPTED_BASE", ""), "Accepted baseline")
    current = subprocess.check_output(("git", "rev-parse", "HEAD"), cwd=ROOT, text=True).strip()
    if current == accepted:
        message = "Accepted baseline must differ from HEAD"
        raise SystemExit(message)
    result = subprocess.run(
        (
            *buf,
            "breaking",
            "contracts",
            "--against",
            f".git#ref={accepted},subdir=contracts",
            "--error-format=json",
        ),
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=False,
        timeout=150,
    )
    diagnostics = (result.stdout + result.stderr).strip()
    if diagnostics:
        print(diagnostics)
    if result.returncode == 0:
        return 0
    # A single explicitly approved SHA pair cannot exempt subsequent commits or baselines.
    # Buf 1.47.2 reports breaking-change annotations with exit code 100, not 1.
    approval_base = os.environ.get("SCYG_CONTRACTS_CUTOVER_BASE", "")
    approval_head = os.environ.get("SCYG_CONTRACTS_CUTOVER_HEAD", "")
    if (
        approval_base != accepted
        or approval_head != current
        or result.returncode != BUF_ANNOTATION_EXIT_CODE
    ):
        return result.returncode
    contract_changes = subprocess.check_output(
        ("git", "status", "--porcelain=v1", "--untracked-files=all", "--", "contracts"),
        cwd=ROOT,
        text=True,
    )
    if contract_changes:
        print("Cutover approval requires contract inputs identical to the approved HEAD")
        return result.returncode
    old_service = "contracts/proto/scyg/blog/v1/blog_tool_service.proto"
    new_service = "contracts/proto/scyg/blog/v1/blog_content_service.proto"
    subprocess.run(("git", "cat-file", "-e", f"{accepted}:{old_service}"), cwd=ROOT, check=True)
    prior = subprocess.run(
        ("git", "cat-file", "-e", f"{accepted}:{new_service}"),
        cwd=ROOT,
        check=False,
    )
    if prior.returncode == 0:
        message = "Cutover approval cannot exempt an already-cut-over baseline"
        raise SystemExit(message)
    try:
        errors = [json.loads(line) for line in diagnostics.splitlines() if line.strip()]
    except json.JSONDecodeError:
        return result.returncode
    if not errors or any(
        not isinstance(error, dict) or not error.get("type") or not error.get("message")
        for error in errors
    ):
        return result.returncode
    print(
        f"Explicit one-time BlogToolService -> BlogContentService cutover approved: "
        f"base={accepted} head={current}; Buf lint and breaking both executed; "
        f"{len(errors)} incompatibilities retained above"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
