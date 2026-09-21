"""Fixed S3 driver: duplicate cancellation returns the durable replay result."""

from __future__ import annotations

import sys

from scripts.t34_http_driver import cancel, evidence_root, fixture, write_receipt


def main(arguments: list[str]) -> int:
    """Send the same cancellation request twice and require the replay marker."""
    root = evidence_root(arguments)
    value = fixture("S3")
    if root is None or value is None:
        return 1
    markers = cancel(value)
    if markers != ("false", "true"):
        print("T34 S3 driver failed: cancellation did not return the durable replay result")
        return 1
    write_receipt(root, "S3", "cancel replay=false,true")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
