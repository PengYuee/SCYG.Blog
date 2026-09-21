# syntax=docker/dockerfile:1.7@sha256:a57df69d0ea827fb7266491f2813635de6f17269be881f696fbfdf2d83dda33e
# Dockerfile frontend 1.7.0; digest pins the parser implementation.
FROM ghcr.io/astral-sh/uv:0.8.13@sha256:4de5495181a281bc744845b9579acf7b221d6791f99bcc211b9ec13f417c2853 AS uv

# Python 3.12.13 slim Bookworm.
FROM python:3.12.13-slim-bookworm@sha256:76d4b7b6305788c6b4c6a19d6a22a3921bf802e9af4d5e1e5bd771208dba74bf AS builder
ENV UV_COMPILE_BYTECODE=1
ENV UV_LINK_MODE=copy
ENV UV_PYTHON_DOWNLOADS=never
WORKDIR /app
COPY --from=uv /uv /usr/local/bin/uv
COPY pyproject.toml uv.lock README.md ./
RUN uv sync --frozen --no-dev --no-editable --no-install-project
COPY src ./src
RUN uv sync --frozen --no-dev --no-editable

# Python 3.12.13 slim Bookworm.
FROM python:3.12.13-slim-bookworm@sha256:76d4b7b6305788c6b4c6a19d6a22a3921bf802e9af4d5e1e5bd771208dba74bf AS runtime
ENV PATH=/opt/venv/bin:$PATH
ENV PYTHONPATH=/app/src
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV TMPDIR=/var/tmp/scyg-agent
RUN groupadd --system --gid 10001 scyg-agent && useradd --system --uid 10001 --gid 10001 --home-dir /nonexistent --shell /usr/sbin/nologin scyg-agent && mkdir -p /app/src /var/tmp/scyg-agent && chown 10001:10001 /var/tmp/scyg-agent && chmod 0750 /var/tmp/scyg-agent
COPY --from=builder /app/.venv /opt/venv
COPY --chown=root:root src /app/src
COPY --chown=root:root migrations /app/migrations
COPY --chown=root:root alembic.ini /app/alembic.ini
RUN chmod -R a-w /app /opt/venv && chmod -R a+rX /app /opt/venv
WORKDIR /app
EXPOSE 8080 9090
USER 10001:10001
HEALTHCHECK --interval=10s --timeout=3s --start-period=30s --retries=3 CMD ["/opt/venv/bin/python", "-c", "import json,sys,urllib.request; response=urllib.request.urlopen('http://127.0.0.1:8080/health/ready',timeout=2); payload=json.load(response); response.close(); sys.exit(0 if payload.get('ready') is True else 1)"]
ENTRYPOINT ["/opt/venv/bin/python", "-m", "scyg_agent", "run"]
