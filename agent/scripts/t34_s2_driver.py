"""Fixed S2 driver: persisted-cursor SSE reconnect without duplicates."""

from __future__ import annotations

import os
import sys

from scripts.t34_http_driver import (
    evidence_root,
    fixture,
    initial_sse,
    reconnect_observation,
    sse,
    write_receipt,
)


def main(arguments: list[str]) -> int:
    """Reconnect from the configured persisted cursor and validate replay ordering."""
    root = evidence_root(arguments)
    value = fixture("S2")
    raw_cursor = os.environ.get("SCYG_T34_S2_CURSOR", "")
    try:
        cursor = int(raw_cursor)
    except ValueError:
        print("T34 S2 driver prerequisites: SCYG_T34_S2_CURSOR must be a non-negative integer")
        return 1
    if root is None or value is None or cursor < 0:
        return 1
    initial = initial_sse(value, cursor)
    replay = sse(value, initial.cursor) if initial is not None and initial.cursor > cursor else None
    observation = reconnect_observation(cursor, initial, replay)
    if observation is None:
        print("T34 S2 driver failed: reconnect replay was not strictly ordered and unique")
        return 1
    write_receipt(root, "S2", observation)
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
