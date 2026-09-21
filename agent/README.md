# SCYG Agent

Strict Python 3.12 service scaffold for the SCYG Agent process.

## Bootstrap check

Copy the example to the untracked `agent.toml`, fill the required values, and run a bounded,
non-listening check from the `agent/` directory:

```powershell
Copy-Item agent.toml.example agent.toml
uv run --locked python -m scyg_agent --check
```

`agent.toml` values override matching `SCYG_AGENT_*` environment variables. Fields omitted from the
file fall back to environment variables and then to typed defaults. Set `SCYG_AGENT_CONFIG_FILE` only
when the file is not at `./agent.toml`. The check parses the merged configuration and composes the
FastAPI application without opening a port, connecting to PostgreSQL, or starting gRPC/worker
services. Do not commit the local TOML file; it may contain database and Provider credentials.

## Local PostgreSQL setup

With PostgreSQL available at the endpoint in `agent.toml`, initialize the database, one application
role, Agent truth migrations, and LangGraph checkpoints with one command:

```powershell
uv run --locked scyg-agent setup
```

The command derives `postgresql://postgres@<host>:<port>/postgres` from `database_url` and securely
prompts for the administrator password. For a different administrator endpoint:

```powershell
uv run --locked scyg-agent setup --admin-url "postgresql://admin@localhost:5432/postgres"
```

The administrator password is not stored. If setup fails, the message includes the failing stage and,
when PostgreSQL provides it, a SQLSTATE without echoing credentials: `database` covers administrator
connection, role, database, and schema preparation; `migration` covers Agent truth migrations; and
`checkpoint` covers LangGraph checkpoint setup. For a non-default administrator user or endpoint, pass
`--admin-url`; the URL must omit the password because the command prompts for it. Setup is rerunnable:
it does not drop Agent tables, but it reapplies the application role password and schema privileges.

After setup succeeds, start the service with
`uv run --locked scyg-agent run`.

## Python contract bindings

Generate the package-owned protobuf and gRPC transport bindings with pinned Buf plugins:

```powershell
uv run scyg-agent-contracts generate
```

Verify committed bindings against a fresh temporary generation without consulting Git status:

```powershell
uv run scyg-agent-contracts check
```

## PostgreSQL acceptance tests

Endpoint-backed tests use one untracked configuration file instead of per-test environment
variables. Copy the example, set the normal and destructive-migration PostgreSQL DSNs, then run:

```powershell
Copy-Item tests/test-agent.toml.example tests/test-agent.toml
$env:SCYG_TEST_CONFIG_FILE = (Resolve-Path tests/test-agent.toml)
uv run pytest -q
```

`normal_database_url` is used by repository, event, command, worker, and checkpoint acceptance.
`migration_database_url` is isolated for tests that run `downgrade base` and `upgrade head`.
The local `tests/test-agent.toml` is ignored and must not be committed. Without it, PostgreSQL
acceptance tests skip while offline and unit tests continue to run.

## Local one-process Compose

Copy `.env.example` to an untracked `.env`, fill every required value, and start the local Agent
topology from this directory. Database DSNs must target `postgres:5432/scyg_agent`, use the single
`scyg_agent` role, and URL-encode the password while the paired password variable keeps the original
value. Use a distinct `SCYG_COMPOSE_PROJECT_NAME` for each parallel instance.

Compose environment values are visible to users who can inspect the Docker daemon. This is a local
development topology; the project does not require encrypted configuration for this setup.

```powershell
docker compose config
docker compose up --build --wait
```

`agent-setup` is a bounded one-shot task. It prepares the `scyg_agent` role, migrates Agent truth,
initializes LangGraph checkpoints with the same DSN, and then starts the single long-running `agent`
process. PostgreSQL, HTTP, and gRPC host ports bind only to `127.0.0.1`.

Stop the process gracefully and remove local state when it is no longer needed:

```powershell
docker compose down --volumes --remove-orphans
```