# Backend Agent Guide

Go 1.25.0 modular-monolith foundation. This directory is the repository's only Go module.

## Commands
- `task format` — format Go sources with exact-version `go run` gofumpt and goimports.
- `task fmt:check` — reject formatting drift without changing files.
- `task lint` — run pinned golangci-lint v2 and nilaway.
- `task unit` — run uncached race-enabled unit tests.
- `task ci` — run the current buildable Gate B checks.

## Pinned environment
- Go: `1.25.0`.
- Go build image: `golang:1.25.0-bookworm@sha256:81dc45d05a7444ead8c92a389621fafabc8e40f8fd1a19d7e5df14e61e98bc1a`.
- PostgreSQL: `postgres:17.5@sha256:aadf2c0696f5ef357aa7a68da995137f0cf17bad0bf6e1f17de06ae5c769b302`.
- Task: `v3.49.1`; bootstrap with `go run github.com/go-task/task/v3/cmd/task@v3.49.1 <task>` or install that exact version.

## Tool isolation
- Taskfile commands use `go run package@version`; tools must not be added to `go.mod` or imported by `tools.go`.
- `task ci` rejects a Task runner version other than `v3.49.1`.

## Boundaries
- Keep `cmd/api/main.go` at or below 50 pure LOC.
- Do not create a root `go.mod`, `go.work`, a second module, or future-module placeholders.
- Use manual constructor injection; Wire, Fx, Dig, service locators, and mutable globals are forbidden.
- Handwritten Go files must remain at or below 250 pure LOC.
- Add comments for exported identifiers, signatures, fields, and non-obvious logic.
