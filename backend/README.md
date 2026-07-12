# SCYG.Blog Go 后端

本目录是仓库唯一的 Go 模块，当前只运行 REST/HTTP。所有命令均从 `backend/` 执行。

## 前置条件

- Go `1.26.0`、Task `v3.49.1`。
- 静态门禁不要求数据库或 Docker，但首次运行固定版本工具时可能下载工具。
- 标注“PostgreSQL”的本地命令统一读取 `config.local.yaml`。
- 标注“Docker”的命令需要 Docker Engine 与 Compose；会创建并清理容器资源。

## 本地 YAML 配置

1. 仓库提供可提交的 `config.example.yaml`，其中包含 `app`、`http`、`database`、`docs`、`telemetry` 和独立的 `qa` 全部配置键与中文说明。
2. 当前开发机使用已被 Git 精确忽略的 `config.local.yaml`。把 `database.dsn` 与 `qa.postgres_admin_dsn` 中的 `请填写密码` 替换为真实密码，不要提交该文件。
3. 占位密码会在连接数据库前返回明确中文配置错误，避免误连。普通 API 运行配置不会持有或输出 QA 管理 DSN。
4. 本地 API、迁移、integration、e2e 与 F3 不依赖环境变量。API 和迁移均可用 `-config <路径>` 显式选择另一份 YAML。

配置优先级为“内置默认值 < YAML 文件 < 环境覆盖”。环境覆盖仅保留给无本地文件的 Compose/CI：容器入口显式传 `-config=`，再由 `SCYG_` 注入运行配置。本地 Task 目标始终显式传 `-config config.local.yaml`，迁移命令还会禁用环境覆盖。

## 开发与生成

| 命令 | 依赖 | 用途 |
| --- | --- | --- |
| `task format` | 可能下载固定工具 | 格式化 Go 源码 |
| `task generate` | 可能下载固定工具 | 重新生成 OpenAPI 绑定 |
| `task api:docs:sync` | 无外部服务 | 同步内嵌 OpenAPI 文档副本 |
| `task build` | 无外部服务 | 构建 `bin/api` |
| `task ci` | 可能下载固定工具 | 本地静态、单元、构建与漏洞门禁 |

## 数据库迁移

填写 `config.local.yaml` 后运行 `task migrate:up`、`task migrate:down` 或 `task migrate:roundtrip`。这些目标显式读取同一文件的 `database.dsn`，不会接受 `--dsn` 或读取 `SCYG_DATABASE_DSN`。命令直接修改目标 PostgreSQL，只能对开发/测试数据库执行；运行时不会调用 `AutoMigrate`。

## 测试

- `task unit`：无 PostgreSQL/Docker，竞态、随机顺序、禁用缓存。
- `task integration`：读取 `qa.postgres_admin_dsn`，创建带 `qa.database_prefix` 的隔离数据库。
- `task e2e`：读取相同 QA 配置，运行 `e2e` tag 的完整叙事。
- `task qa:plan`、`task qa:quality`、`task qa:scope`：最终静态审查入口。
- `task qa:foundation`：使用 `qa.command_timeout` 约束真实 PostgreSQL、API、Compose、故障与清理叙事；日志和 evidence 不输出管理 DSN。

## 本地运行

先填写 `config.local.yaml`，执行 `task migrate:up`，再执行 `go run ./cmd/api -config config.local.yaml`。API 的 `-config` 缺省值就是 `config.local.yaml`。该命令会长期监听 HTTP，应在交互式终端运行并以 `Ctrl+C` 触发有界优雅关闭。生产组合允许公开读取，但在身份模块落地前所有写入返回 403。

Docker 开发路径为 `task compose:smoke`，结束后必须执行 `task compose:down`。`task qa:container` 自带 finally/defer 清理。

## 架构交接

- [绑定架构](docs/architecture/go-backend-architecture.md)
- [ADR-010：Scalar 自托管资产版本](docs/architecture/adr-010-scalar-asset-pin.md)
- [新增业务模块](docs/guides/module-extension.md)
- [未来协议与外部集成](docs/guides/protocol-integration-extension.md)

本轮仅验证了 README 与静态门禁中明确记录的命令；需要 PostgreSQL、Docker、浏览器或长期监听器的命令没有被宣称为已通过。
