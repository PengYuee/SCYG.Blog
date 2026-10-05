# SCYG.Blog 后端项目交接文档

## 1. 文档状态

- 状态：当前后端项目交接说明。
- 适用范围：仓库 `backend/` Go 模块及其正式文档。
- 当前事实来源：源码、`api/openapi.yaml`、配置示例、`Taskfile.yml`、迁移文件和现行架构文档。
- Agent API 需求：[SCYG Agent 后端 API 需求文档](../../../plan/SCYG%20Agent后端API需求文档.md) 已冻结设计但尚未在当前 `backend/` 实现；不能把该文档中的目标能力视为当前运行能力。

## 2. 当前项目边界

`backend/` 是仓库唯一的 Go module，当前运行单体 REST/HTTP API。当前已实现的业务模块为：

- 身份认证与用户登录；
- 文章查询、创建、局部更新、发布、归档和删除；
- 文章类型与标签查询、维护；
- 文章正文图片上传、引用、媒体读取和清理；
- PostgreSQL 持久化；
- 本地文件系统图片 Blob 存储；
- `/live`、`/ready` 和自托管 API 文档。

当前 Blog 后端尚未实现 Agent HTTP/SSE API、AgentControlService、BlogContentService、Agent Run 表、Agent 事件或 Agent 结果存储。当前进程也没有 Agent gRPC listener。

## 3. 重要入口

| 内容 | 位置 |
| --- | --- |
| HTTP API 源契约 | [`backend/api/openapi.yaml`](../../../backend/api/openapi.yaml) |
| 生成的 OpenAPI bindings | [`backend/internal/generated/openapi/`](../../../backend/internal/generated/openapi/) |
| REST 路由装配 | [`backend/internal/transport/rest/router.go`](../../../backend/internal/transport/rest/router.go) |
| 手工依赖装配 | [`backend/internal/bootstrap/construct.go`](../../../backend/internal/bootstrap/construct.go) |
| 应用生命周期 | [`backend/internal/bootstrap/app.go`](../../../backend/internal/bootstrap/app.go) |
| 身份认证 | [`backend/internal/modules/identity/auth/`](../../../backend/internal/modules/identity/auth/) |
| 文章业务 | [`backend/internal/modules/content/article/`](../../../backend/internal/modules/content/article/) |
| 分类与标签 | [`backend/internal/modules/content/taxonomy/`](../../../backend/internal/modules/content/taxonomy/) |
| 图片与 Blob | [`backend/internal/modules/content/image/`](../../../backend/internal/modules/content/image/) |
| 跨 feature 工作流 | [`backend/internal/modules/content/application/`](../../../backend/internal/modules/content/application/) |
| 数据库迁移 | [`backend/migrations/`](../../../backend/migrations/) |
| 固定版本命令 | [`backend/Taskfile.yml`](../../../backend/Taskfile.yml) |
| 当前架构 | [`docs/services/backend/architecture/current-state-architecture.zh-CN.md`](architecture/current-state-architecture.zh-CN.md) |
| 开发指南 | [`docs/services/backend/guides/backend-development.md`](guides/backend-development.md) |
| 模块扩展规则 | [`docs/services/backend/guides/module-extension.md`](guides/module-extension.md) |
| 协议扩展规则 | [`docs/services/backend/guides/protocol-integration-extension.md`](guides/protocol-integration-extension.md) |

服务目录的 `backend/README.md` 只保留导航；正式后端文档维护在根 `docs/services/backend/`。

## 4. 本地环境和运行

### 4.1 工具版本

- Go `1.26.0`；
- Task `v3.49.1`；
- PostgreSQL `17.5`；
- 固定工具版本见 `backend/Taskfile.yml`。

### 4.2 配置

1. 复制 `backend/config.example.yaml` 为本地未跟踪的 `backend/config.local.yaml`。
2. 填写数据库 DSN 和本地图片存储目录。
3. QA 使用独立的 `backend/config.qa.example.yaml`，不要把 QA DSN 写入运行时配置。
4. 不提交包含真实密码的本地配置。

### 4.3 启动

从 `backend/` 执行：

```text
CONFIG=config.local.yaml task migrate:up
go run ./cmd/api -config config.local.yaml
```

API 默认配置文件为 `config.local.yaml`。应用启动时检查数据库连接、迁移版本和依赖装配；运行时不会自动执行 migration，也不使用 `AutoMigrate`。

健康端点：

```text
GET /live
GET /ready
```

### 4.4 Docker Compose

```text
task compose:smoke
task compose:down
```

`compose:smoke` 完成后必须清理 Compose 资源；完整 QA 容器流程使用 `task qa:container`。

## 5. 日常开发和验证

常用命令均从 `backend/` 执行：

| 命令 | 用途 |
| --- | --- |
| `task format` | 使用固定版本工具格式化 Go 源码 |
| `task fmt:check` | 检查格式漂移 |
| `task generate` | 生成 OpenAPI bindings |
| `task api:docs:sync` | 同步内嵌 OpenAPI 文档副本 |
| `task api:docs:check` | 检查 API 文档同步 |
| `task api:generate:check` | 检查生成物漂移 |
| `task unit` | 竞态检测、随机顺序、禁用缓存的单元测试 |
| `task lint` | golangci-lint 和 nilaway |
| `task ci` | 当前后端质量门禁 |
| `QA_CONFIG=path task integration` | PostgreSQL integration 测试 |
| `QA_CONFIG=path task e2e` | E2E 测试 |
| `QA_CONFIG=path task qa:foundation` | 数据库、API、Compose、故障和清理叙事 |

改动 OpenAPI 路径、operationId 或 schema 时，必须同步检查生成代码、内嵌文档和 contract tests。改动 migration 时，必须同时覆盖 up/down、migration roundtrip 和真实数据库验证。

## 6. 架构约束

- 使用手动构造函数注入；不使用 Wire、Fx、Dig、service locator 或可变全局依赖容器。
- 业务 feature 不导入 Gin、生成的 OpenAPI 或 platform/database；Repository 才可使用 GORM 和数据库适配器。
- `article`、`taxonomy`、`image` 各自拥有业务规则、Repository、Record、Mapper 和稳定错误；feature 不导入兄弟 feature。
- `content/application` 只负责具名跨 feature 工作流和事务边界，不直接写 SQL、访问 Record 或 Repository。
- 持久化使用版本化 SQL migration，不使用 `AutoMigrate`。
- 生命周期、乐观并发控制和 claim 写入必须使用显式列集合、条件 `WHERE` 和 `RowsAffected` 检查。
- 列表分页由数据库执行；排序只能使用白名单并包含稳定二级排序键。
- 外部 Blob 清理使用短事务 claim，事务外执行文件操作，再用精确 token 完成或释放。
- 新的公开 REST 接口必须先更新 `backend/api/openapi.yaml`，再重新生成 bindings 和内嵌文档。

详细约束以 [`backend/AGENTS.md`](../../../backend/AGENTS.md) 和现行架构文档为准。

## 7. 当前认证和权限

HTTP 登录入口为 `/api/v1/auth/login`，成功后获得 Bearer JWT。REST middleware 将已验证 principal 写入 request context；受保护路由要求有效 principal，公开文章、分类、标签和媒体读取路径按 OpenAPI 策略开放。

当前 content 授权以 Blog 现有认证和资源规则为准。不要在新 feature 中自行复制 JWT 解析、用户上下文或授权判定；应通过现有认证边界和消费方窄接口接入。

## 8. Agent API 交接重点

Agent API 需求已确认，但尚未实施。后续实现必须以需求文档为准，先更新合同再改业务代码：

1. 复用 `contracts/proto/scyg/agent/v1/` 和 `contracts/proto/scyg/blog/v1/`，不要创建第二套 Proto 目录。
2. AgentControlService clean cutover 为 5 个 RPC：CreateRun、GetRun、StreamRunEvents、ResumeRun、CancelRun。
3. BlogContentService clean cutover 为 8 个通用业务 RPC：SearchArticles、GetArticle、ListTags、ListArticleTypes、CreateArticle、UpdateArticle、PublishArticle、ArchiveArticle。
4. Blog 首版不引入 Redis；Agent 是 Run owner、状态、事件、结果和最终幂等的权威。
5. Blog 只验证用户 JWT，并将 `user_id` 传给 Agent；Agent 执行 Run owner 校验。
6. SSE 是 Blog 的薄代理：使用 Bearer JWT，转发不透明 `Last-Event-ID`，原样转发 Agent 已编码 SSE frame。
7. Agent→Blog 写入使用 `operation_id` 并在 Blog PostgreSQL 中提供事务幂等；不能用缓存替代最终幂等。
8. 不保留旧 Runtime、旧 BlogToolService、旧 CreateAgentRun 或旧兼容别名。

当前这些内容是待实施设计，不代表已有运行代码。实现时需要同步 migration、bootstrap、OpenAPI、Proto 生成、Compose、测试和正式文档。

## 9. 交接后的推荐实施顺序

1. 冻结并审阅共享 Proto 变更。
2. 生成 Go/Python Proto 代码并补齐干净构建流程。
3. 实现 BlogContentService 和 operation_id migration。
4. 实现 AgentControlService client/server 适配。
5. 实现 Blog HTTP Agent routes 和 SSE 薄代理。
6. 接入 bootstrap、配置、listener、readiness 和 graceful shutdown。
7. 更新 OpenAPI、生成物、contract tests、integration tests 和 E2E。
8. 执行 `task ci`，再执行涉及 PostgreSQL、Compose 和跨服务调用的验收场景。

## 10. 已知风险和检查入口

- 当前后端文档已迁移到根 `docs/services/backend/`；修改旧 `backend/docs/` 路径不会更新正式文档。
- Agent 需求文档中的目标协议尚未与当前 Agent 旧 Proto 实现完成 clean cutover，不能只新增接口而保留旧双路径。
- 当前 Blog 与 Agent 的 Compose 网络尚未形成统一集成部署；跨服务实现前需要新增根级集成 Compose。
- 生成物不应手工编辑；修改 Proto 或 OpenAPI 后应从源合同重新生成。
- 当前仓库可能同时存在 frontend、agent 和计划目录的其他工作区变更；交接时只提交已明确授权的文件，避免使用全仓库 `git add .` 混入无关改动。

交接完成的判断标准：新接手者可以从本文定位入口、复制配置、启动当前 Blog、运行质量门禁，并明确区分当前已实现能力与待实施 Agent 设计。
