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

## Module file organization
- Business module production stems use an 实体前缀 and 固定职责后缀 and must fully match `<subject>_<role>` or a documented `<subject>_<role>_<subrole>` final suffix; arbitrary token matches, generic subjects, trailing garbage and unknown subroles are forbidden. Semantic exceptions are layer-specific; `api.go` and `module.go` remain required root anchors.
- Required roles include `command`, `query`, `result`, `usecase`, `port`, `view`, `model`, `repository`, `read_model`, `mapper`, `validation`, and `error`; use accurate semantic shared names instead of generic buckets.
- PostgreSQL rows are 数据库数据模型 and use `*_model.go`; `*_record.go`, `models.go`, `usecases.go`, `results.go`, `helpers.go`, `utils.go`, and `common.go` are forbidden.
- Module Go packages are limited to root, `internal/domain`, `internal/application`, `internal/postgres`, and `postgres`; these locations 禁止任何 Go 子 package，因此也禁止实体 Go 子包, unless a separate architecture decision changes the contract.
- Tests follow behavior and responsibility and need not map one-to-one to production files. These naming rules do not relax the existing 250 pure LOC limit.
