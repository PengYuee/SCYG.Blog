# SCYG.Blog Go 后端

`backend/` 是仓库唯一的 Go 模块，提供 REST/HTTP；启用 Agent 集成时，同一进程还提供内部 BlogContent gRPC。除根级集成 Compose 外，本页命令均从 `backend/` 执行。

## 前置条件

- Go `1.26.8`、Task `v3.49.1`。
- 静态门禁不要求数据库或 Docker，但首次运行固定版本工具时可能下载工具。
- 标注“PostgreSQL”的本地命令必须通过 `CONFIG=<运行时 YAML>` 或 `QA_CONFIG=<QA YAML>` 显式提供配置；不会从未指定的本地文件推断配置。
- 标注“Docker”的命令需要 Docker Engine 与 Compose；会创建并清理容器资源。

## 本地 YAML 配置

1. 仓库提供可提交的 `config.example.yaml` 作为运行时配置示例，包含 `docs` 文档开关及默认关闭的 `agent` 集成；QA 门禁使用独立的 `config.qa.example.yaml`。
2. 当前开发机使用被 Git 忽略的 `config.local.yaml`。运行时配置中的 `database.dsn` 使用真实密码，不要提交该文件；QA 管理 DSN 只存在于临时 QA 配置。
3. 本地 API、迁移、integration、e2e 与 F3 不依赖环境变量。API 可用 `-config <路径>` 显式选择运行时配置；迁移命令必须显式传入运行时 `CONFIG`，integration/e2e 必须显式传入 QA-only `QA_CONFIG`。

运行时配置与 QA 配置分离：复制 `config.example.yaml` 为 `config.local.yaml` 供 API 使用；复制 `config.qa.example.yaml` 为被 Git 忽略的临时 `config.qa.yaml`。`CONFIG` 仅指运行时 YAML，`QA_CONFIG` 仅指 QA YAML；QA 配置不接受环境覆盖，也不会回退到 `config.local.yaml`。

## 开发与生成

| 命令 | 依赖 | 用途 |
| --- | --- | --- |
| `task format` | 可能下载固定工具 | 格式化 Go 源码 |
| `task generate` | 可能下载固定工具 | 重新生成 OpenAPI 绑定与共享 Proto 的 Go 绑定 |
| `task api:docs:sync` | 无外部服务 | 同步内嵌 OpenAPI 文档副本 |
| `task proto:generate` | 固定版本 Buf/npm 工具链 | 从根 `contracts/` 生成到 `internal/generated/proto/` |
| `task build` | 无外部服务 | 构建 `bin/api` |
| `task ci` | 可能下载固定工具 | 本地静态、单元、构建与漏洞门禁 |

## 数据库迁移

迁移目标必须显式提供运行时配置：`CONFIG=path task migrate:up` 或 `CONFIG=path task migrate:down`。数据库门禁使用 `QA_CONFIG=path task qa:database`，由唯一编排器执行 migration、integration 与 E2E；不会回退到 `config.local.yaml` 或读取环境 DSN。

## 测试

- `task unit`：无 PostgreSQL/Docker，竞态、随机顺序、禁用缓存。
- `QA_CONFIG=path task integration`：使用显式 QA 配置和本次 run capability 创建隔离数据库。
- `QA_CONFIG=path task e2e`：使用相同显式 QA 配置运行 `e2e` tag 的完整叙事。
- `task qa:plan`、`task qa:quality`、`task qa:scope`：最终静态审查入口。
- `QA_CONFIG=path task qa:foundation`：执行真实 PostgreSQL、API、Compose、故障与清理叙事；日志和 evidence 不输出管理 DSN。

## 本地运行

先填写 `config.local.yaml`，执行 `CONFIG=config.local.yaml task migrate:up`，再执行 `go run ./cmd/api -config config.local.yaml`。API 的 `-config` 缺省值就是 `config.local.yaml`。该命令会长期监听 HTTP，应在交互式终端运行并以 `Ctrl+C` 触发有界优雅关闭。生产组合允许公开读取；身份模块已接入用户登录与 JWT 认证，受保护的写入通过认证上下文进行授权，不再采用身份模块未实现时的统一拒绝写入策略。

`agent.enabled` 默认 `false`，此时八个 Agent HTTP/SSE 路由整组返回 404，且不创建 Agent client 或 BlogContent listener。启用时默认连接 `127.0.0.1:9090`，BlogContent 默认监听 `127.0.0.1:50051`；内部 gRPC 不使用 service JWT，不应公开端口。Agent 离线不阻止 Blog 启动或 `/ready` 就绪；本地 BlogContent listener 绑定失败则启动失败。关闭时先排空 HTTP/SSE，再以独立的 `agent.grpc_shutdown_timeout` 排空 gRPC。接口及业务语义见[协议与外部集成](guides/protocol-integration-extension.md)。

Docker 开发路径为 `task compose:smoke`，结束后必须执行 `task compose:down`。`task qa:container` 自带 finally/defer 清理。

Blog↔Agent 联合部署定义位于仓库根 `compose.yaml`，从仓库根选择该文件；它使用根目录构建上下文及共享 `contracts/`，内部 gRPC 端口未映射到宿主机。

## 开发指南

- [Backend 开发指南](guides/backend-development.md)
- [新增业务模块](guides/module-extension.md)
- [历史架构决策](architecture/go-backend-architecture.md)
- [当前架构](architecture/current-state-architecture.zh-CN.md)
- [ADR-010：Scalar 自托管资产版本](architecture/adr-010-scalar-asset-pin.md)
- [协议与外部集成](guides/protocol-integration-extension.md)
