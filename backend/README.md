# SCYG.Blog Go 后端

本目录是仓库唯一的 Go 模块，当前只运行 REST/HTTP。所有命令均从 `backend/` 执行。

## 前置条件

- Go `1.25.0`、Task `v3.49.1`。
- 静态门禁不要求数据库或 Docker，但首次运行固定版本工具时可能下载工具。
- 标注“PostgreSQL”的命令需要 `SCYG_DATABASE_DSN` 指向可丢弃的真实 PostgreSQL。
- 标注“Docker”的命令需要 Docker Engine 与 Compose；会创建并清理容器资源。

## 开发与生成

| 命令 | 依赖 | 用途 |
| --- | --- | --- |
| `task format` | 可能下载固定工具 | 格式化 Go 源码 |
| `task generate` | 可能下载固定工具 | 重新生成 OpenAPI 绑定 |
| `task api:docs:sync` | 无外部服务 | 同步内嵌 OpenAPI 文档副本 |
| `task build` | 无外部服务 | 构建 `bin/api` |
| `task ci` | 可能下载固定工具 | 本地静态、单元、构建与漏洞门禁 |

## 数据库迁移

设置 `SCYG_DATABASE_DSN` 后运行 `task migrate:up`、`task migrate:down` 或 `task migrate:roundtrip`。这些命令直接修改目标 PostgreSQL，只能对开发/测试数据库执行；运行时不会调用 `AutoMigrate`。

## 测试

- `task unit`：无 PostgreSQL/Docker，竞态、随机顺序、禁用缓存。
- `task integration`：需要真实 PostgreSQL；不以 SQL mock 替代。
- `task e2e`：需要真实 PostgreSQL/API 测试环境，运行 `e2e` tag 的完整叙事。
- `task qa:plan`、`task qa:quality`、`task qa:scope`：最终静态审查入口。
- `task qa:foundation`：需要 Docker 与真实系统，执行迁移、Compose、HTTP、故障与清理叙事。

## 本地运行

设置全部必需的 `SCYG_` 配置和 `SCYG_DATABASE_DSN`，先执行 `task migrate:up`，再执行 `go run ./cmd/api`。该命令启动长期 HTTP 服务，应在交互式终端运行并以 `Ctrl+C` 触发有界优雅关闭。生产组合允许公开读取，但在身份模块落地前所有写入返回 403。

Docker 开发路径为 `task compose:smoke`，结束后必须执行 `task compose:down`。`task qa:container` 自带 finally/defer 清理。

## 架构交接

- [绑定架构](docs/architecture/go-backend-architecture.md)
- [新增业务模块](docs/guides/module-extension.md)
- [未来协议与外部集成](docs/guides/protocol-integration-extension.md)

本轮仅验证了 README 与静态门禁中明确记录的命令；需要 PostgreSQL、Docker、浏览器或长期监听器的命令没有被宣称为已通过。
