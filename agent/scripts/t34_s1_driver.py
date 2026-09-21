"""Fixed S1 driver: normal SIMPLE Run to a terminal SSE event."""

from __future__ import annotations

import sys

from scripts.t34_http_driver import evidence_root, fixture, simple_snapshot, sse, write_receipt


def main(arguments: list[str]) -> int:
    """Observe one authenticated SIMPLE fixture Run until terminal SSE."""
    root = evidence_root(arguments)
    value = fixture("S1")
    if root is None or value is None:
        return 1
    if not simple_snapshot(value):
        return 1
    frames = sse(value, 0)
    if frames is None or not frames:
        print("T34 S1 driver failed: terminal SSE result was not observed")
        return 1
    terminal = frames[-1]
    if terminal.name not in {"run_succeeded", "run_failed", "run_cancelled"}:
        print("T34 S1 driver failed: terminal SSE result was not observed")
        return 1
    write_receipt(root, "S1", f"terminal={terminal.name} events={len(frames)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
