# Backend Agent Guide

Go 1.26.0 modular-monolith foundation. This directory is the repository's only Go module.

## Commands
- `task format` — format Go sources with exact-version `go run` gofumpt and goimports.
- `task fmt:check` — reject formatting drift without changing files.
- `task lint` — run pinned golangci-lint v2 and nilaway.
- `task unit` — run uncached race-enabled unit tests.
- `task ci` — run the current buildable Gate B checks.

## Pinned environment
- Go: `1.26.0`.
- Go build image: `golang:1.26.0-bookworm@sha256:2a0ba12e116687098780d3ce700f9ce3cb340783779646aafbabed748fa6677c`.
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

## Module file organization
- Mandatory naming floor: production stems use 小写 snake_case and are non-empty, with no leading, trailing, or consecutive underscores. Generic token set: `common` / `shared` / `utils` / `utility` / `helpers` / `models` / `usecases` / `results`. Every token in this set is forbidden anywhere in a stem.
- `api.go` and `module.go` are module-root-only anchors. PostgreSQL rows are 数据库数据模型 and must use `<subject>_model.go`; `*_record.go` is forbidden.
- Module Go packages are limited to root, `internal/domain`, `internal/application`, `internal/postgres`, and `postgres`; these locations 禁止任何 Go 子 package，因此也禁止实体 Go 子包, unless a separate architecture decision changes the contract.
- Recommended naming follows "layer by directory, subject by prefix, responsibility by name". Prefer `<subject>_<role>.go` or `<subject>_<role>_<subrole>.go` when useful, but these are examples rather than a role whitelist. Scanner does not enumerate responsibility suffixes, so a clear new responsibility needs no Scanner change.
- Tests use a business subject and observable behavior in their names. Tests use the same generic token set defined above, and need not map one-to-one to production files. These naming rules do not relax the existing 250 pure LOC limit.