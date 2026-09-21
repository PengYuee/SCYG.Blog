"""Shared secret redaction for Agent QA and bounded E2E diagnostics."""

from __future__ import annotations

import re
from typing import Final
from urllib.parse import urlsplit, urlunsplit

SECRET_LINE: Final = re.compile(
    r"(?i)(password|token|secret|api[_-]?key|private[_-]?key|authorization|dsn|jwt)"
)
SENSITIVE_ASSIGNMENT: Final = re.compile(
    r"""(?ix)
    ^(?P<prefix>
        \s*(?:-\s*)?
        (?:
            [A-Z][A-Z0-9_]*(?:PASSWORD|TOKEN|SECRET|API_KEY|PRIVATE_KEY|AUTHORIZATION|DSN|DATABASE_URL)
            | PASSWORD
            | TOKEN
            | SECRET
            | JWT
            | API_KEY
            | PRIVATE_KEY
            | AUTHORIZATION
            | DSN
            | DATABASE_URL
        )
        \s*[:=]\s*
    ).*$"""
)
JWT: Final = re.compile(r"eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+")
DSN: Final = re.compile(r"(?i)\bpostgres(?:ql)?(?:\+[a-z0-9_-]+)?://[^\s]+")
URL_SECRET: Final = re.compile(r"(://)[^:/\s@]+:[^@/\s]+(@)")
URL_QUERY_SECRET: Final = re.compile(
    r"(?i)([?&](?:access_token|token|api[_-]?key|password|secret)=)[^&#\s]+"
)
INLINE_SECRET_ASSIGNMENT: Final = re.compile(
    r"(?ix)(?P<prefix>\b(?:PASSWORD|TOKEN|SECRET|JWT|API[_-]?KEY|PRIVATE[_-]?KEY|AUTHORIZATION|DSN|DATABASE_URL)\s*[:=]\s*)(?:Bearer\s+)?[^\s,;#]+"
)


def _redact_dsn(match: re.Match[str]) -> str:
    """Redact DSN credentials while retaining non-sensitive connection context."""
    try:
        parsed = urlsplit(match.group())
    except ValueError:
        return "postgresql://[REDACTED]"
    netloc = parsed.netloc
    if "@" in netloc:
        netloc = f"[REDACTED]@{netloc.rsplit('@', maxsplit=1)[1]}"
    query = URL_QUERY_SECRET.sub(r"\1[REDACTED]", f"?{parsed.query}").removeprefix("?")
    fragment = "[REDACTED]" if parsed.fragment else ""
    return urlunsplit((parsed.scheme, netloc, parsed.path, query, fragment))


def sanitize(value: str) -> str:
    """Redact secret-bearing diagnostics while retaining non-sensitive context."""
    lines: list[str] = []
    for raw_line in JWT.sub("[REDACTED]", value).splitlines():
        assignment = SENSITIVE_ASSIGNMENT.match(raw_line)
        if assignment is not None:
            lines.append(f"{assignment.group('prefix')}[REDACTED]")
            continue
        redacted = INLINE_SECRET_ASSIGNMENT.sub(r"\g<prefix>[REDACTED]", raw_line)
        redacted = DSN.sub(_redact_dsn, redacted)
        redacted = URL_SECRET.sub(r"\1[REDACTED]\2", redacted)
        redacted = URL_QUERY_SECRET.sub(r"\1[REDACTED]", redacted)
        scan = INLINE_SECRET_ASSIGNMENT.sub("", URL_QUERY_SECRET.sub("", redacted))
        lines.append("[REDACTED_FIELD]" if SECRET_LINE.search(scan) else redacted)
    return "\n".join(lines)
