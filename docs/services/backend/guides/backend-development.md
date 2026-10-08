# Backend 开发指南

本文规定 `backend/` 的代码边界、接口变更流程、数据库约定和验证门禁。

## 1. 先确认权威来源

同一事实只在一个位置维护：

| 内容 | 权威来源 |
| --- | --- |
| REST 路径、请求响应、Schema、错误响应 | [`api/openapi.yaml`](../../../../backend/api/openapi.yaml) |
| 生成的 Go bindings | `internal/generated/openapi/`，由 OpenAPI 生成，不手工修改 |
| 数据库结构与迁移 | `migrations/` |
| Task 命令 | [`Taskfile.yml`](../../../../backend/Taskfile.yml) |
| 业务模块边界 | [`module-extension.md`](module-extension.md) |
| 当前整体架构 | [`current-state-architecture.zh-CN.md`](../architecture/current-state-architecture.zh-CN.md) |
| 强制工程规则 | [`AGENTS.md`](../../../../backend/AGENTS.md) |

## 2. 当前代码边界

`backend/` 是独立 Go module。当前运行路径是 REST、业务模块、GORM/PostgreSQL 和显式 SQL migration。

业务代码按业务主体组织：

```text
internal/modules/content/
├── article/       # 文章生命周期、校验、查询和持久化
├── taxonomy/      # 文章类型和标签
├── image/         # 图片、Blob、引用和清理生命周期
└── application/   # 必须跨 feature 协作的具名事务用例
```

边界规则：

- REST Handler 负责 HTTP/OpenAPI DTO、状态码、Header、multipart 和错误转换；不直接访问 GORM、Repository 或 Blob filesystem。
- feature Service 和 application 不依赖 Gin、HTTP 或 generated OpenAPI 类型。
- feature Repository 只访问所属 feature 的表。
- 跨 feature 的同事务操作进入 `application/`，由 application 持有事务边界，并调用各 feature 的 `InTx` 方法。
- 文章 REST 响应由 `application.ArticleResponses` 协调 article/taxonomy；读写快照、摘要装配及失败语义见[当前架构](../architecture/current-state-architecture.zh-CN.md#http-与-openapi)，不在 REST 或 feature Repository 中另建跨表装配。
- 数据库结构由 migration 管理；不使用 `AutoMigrate`、`gorm.Model` 或 `gorm.DeletedAt`。
- 运行时对象由 bootstrap 构造；不要在 Handler 或业务模块中自行创建全局数据库连接。

## 3. 路由归属

公开路由只承担公开读取；需要授权的内容写操作进入管理端路径。

| 用途 | 路径 |
| --- | --- |
| 公开文章列表和详情 | `GET /api/v1/articles`、`GET /api/v1/articles/{articleId}` |
| 管理端文章创建、列表、详情、修改、删除 | `/api/v1/manage/articles`、`/api/v1/manage/articles/{articleId}` |
| 管理端发布和归档 | `POST /api/v1/manage/articles/{articleId}/publish`、`/archive` |
| 管理端正文图片上传 | `POST /api/v1/manage/article-images` |
| 正文图片取消 | `DELETE /api/v1/article-images/{imageId}` |
| 正文图片媒体读取 | `GET /media/article-images/{storageKey}` |

文章创建使用管理端路径，成功响应的 `Location` 指向管理端文章资源。

修改、删除、发布、归档使用强 `If-Match`：

- 缺失 `If-Match`：`428 Precondition Required`；
- 版本不匹配：`412 Precondition Failed`；
- 成功写入：返回新的强 `ETag`；
- 创建成功：返回 `201`、`Location` 和初始强 `ETag`。

文章 Patch 只修改内容字段，不承担状态迁移。发布和归档使用独立动作接口。

## 4. 修改 REST/OpenAPI 的标准流程

### 4.1 先改源契约

先修改 `api/openapi.yaml`，再生成代码：

```powershell
cd E:\gitproject\SCYG.Blog\backend
task api:generate
task api:docs:sync
```

`api/openapi.yaml` 是 REST 契约源文件。`internal/generated/openapi/` 和 `internal/transport/rest/apidocs/assets/openapi.yaml` 由生成命令维护。
修改路径或 `operationId` 时，同时检查：

- `internal/contracttest/policy_test.go`：路径集合和授权策略；
- `internal/contracttest/schema_test.go`：请求/响应 Schema 和 Header；
- 相关的 operation 专用契约测试；
- REST Handler 方法和生成的 request/response 类型；
- E2E 或 integration 中的实际 URL。

### 4.2 更新窄接口和 Handler

REST adapter 只声明它实际消费的最小业务接口。Handler 的职责是：

```text
请求 DTO
  -> 解析和边界校验
  -> feature/application command
  -> 业务结果
  -> response DTO、ETag、Location 或 RFC 9457 problem
```

Handler 负责 DTO 转换、边界校验、业务调用和响应映射；SQL 查询、事务编排和状态迁移属于业务层。

同一 feature 的 REST Handler 可以复用模块内部逻辑，但复用边界按业务语义划分：

- Handler 按接口职责调用明确的用例或 Service 方法；不要让公共、管理或其他权限边界共用依赖 `bool`、模式字符串或隐式上下文的“万能”方法。
- 公共读取和管理读取即使访问同一张表，只要授权、可见性、筛选、排序、统计口径或响应投影不同，就应保留语义明确的入口，例如 `ListPublic...` 与 `ListManage...`。
- 相同的分页校验、名称校验、参数绑定、错误转换、查询片段、Projection Row 映射等，应在 Service/Repository 内部复用，避免每个 Handler 复制实现。
- 公共资源、管理资源和摘要投影按响应契约分别建模；不能复用管理 DTO 后再依赖调用方忽略敏感字段。
- Service 方法数量不是复用质量指标。优先保证授权、不变量和返回模型可从方法签名判断，再在更底层复用真正相同的实现。

推荐结构：

```text
Public Handler  ->  Public use case
Manage Handler  ->  Manage use case

Public/Manage use cases
  ->  shared feature validation/query/mapping helpers
  ->  Repository/Projection
```

只有当两个入口的授权、数据可见性、排序、分页、投影和错误语义确实相同时，才直接共用同一个对外 Service 方法。

multipart 上传必须明确：

- 允许的 part 名称；
- 文件数量；
- 请求体上限；
- 单文件上限；
- 媒体类型和内容校验；
- 临时文件清理路径。

### 4.3 更新测试

至少同步三类测试：

1. REST 单元测试：验证 DTO 映射、状态码和 Header；
2. contract 测试：验证 OpenAPI 路径、Schema、operationId 和错误响应；
3. 真实 integration/E2E：验证数据库事务或真实 HTTP 行为。


## 5. GORM 和数据库约定

查询和写入都绑定请求上下文：

```go
result := repo.db.WithContext(ctx).
    Where("id = ? AND is_deleted = false", id).
    First(&row)
```

持久化实现遵循以下规则：

- 领域实体、数据库 Record、Projection Row 和 DTO 分离；
- Projection 只选择需要的列，使用专用 row struct；
- 参数使用 GORM 参数绑定，不拼接用户输入；
- 版本更新使用条件 `UPDATE`，同时检查 `RowsAffected`；
- 乐观并发冲突转换为稳定的业务错误；
- 事务边界由用例负责，Repository 不私自提交调用方事务；
- 时间、软删除和版本字段按现有 migration/Record 约定映射；
- 数据库错误进入统一错误转换，不把 DSN、密码或 SQL 敏感值写入普通日志。

新增表或修改字段时：

1. 添加 migration；
2. 更新 Record、Mapper、Repository 或 Projection；
3. 验证 migration `up -> down -> up`；
4. 使用真实 PostgreSQL 验证约束、事务和查询行为。

## 6. 配置和 QA 数据库

普通运行时配置和 QA 配置必须分离：

```text
CONFIG      普通运行时 YAML
QA_CONFIG   严格 QA-only YAML
```

本地迁移默认使用 `go run ./cmd/migrate -config <YAML> up`，YAML 模式不接受环境变量覆盖数据库目标。容器初始化复用 `migrate -config= up`：必须显式提供 `SCYG_DATABASE_DSN`，并满足普通运行时配置校验（production 包括非默认 JWT secret）；数据库须已存在。该环境模式不会自动借用管理员身份建库，缺库时改用包含 `qa.postgres_admin_dsn` 的显式 YAML。根集成 Compose 已通过一次性 `blog-setup` 执行该模式，常驻 API 只检查迁移版本。

QA 配置示例：

```yaml
qa:
  postgres_admin_dsn: postgres://postgres:<password>@127.0.0.1:5432/postgres?sslmode=disable
  database_prefix: scyg_qa_
  command_timeout: 2m
```

`postgres_admin_dsn` 必须连接 `postgres` 管理库。QA 管理账号需要同时具备 `CREATEDB` 和 `CREATEROLE`。QA 编排器创建并清理本次运行的临时数据库。

Windows 下推荐使用正斜杠或绝对路径：

```powershell
task integration QA_CONFIG=./config.qa.yaml PACKAGES=./internal/modules/content/article
task e2e QA_CONFIG=./config.qa.yaml PACKAGES=./internal/e2e
task qa:database QA_CONFIG=./config.qa.yaml
```

`task qa:database` 负责 migration roundtrip、integration、E2E 和临时数据库清理。

## 7. 测试门禁选择

| 变更 | 最低验证 |
| --- | --- |
| 纯业务规则或局部实现 | 受影响 package 的 unit test、`qa:feature` |
| REST/OpenAPI 路径、字段、错误或 Header | `qa:contract`、相关 REST 测试 |
| 数据库表、字段、migration 或 GORM 持久化行为 | migration roundtrip、真实 PostgreSQL integration |
| 跨 feature 事务、Blob 生命周期或图片引用 | application integration，必要时 REST E2E |
| bootstrap、readiness、运行生命周期或完整 HTTP 流程 | 受影响的 bootstrap、database 和 E2E |
| 容器、部署或镜像交付 | `qa:container` |

常用局部命令：

```powershell
# 普通单元测试
go test ./...

# 格式和生成物
task fmt:check
task api:generate:check
task api:docs:check

# API 契约
task qa:contract

# 全量静态和单元门禁
task qa:static
```

带 `integration` 或 `e2e` build tag 的测试必须显式提供 `QA_CONFIG`。

## 8. 交付前检查

```text
[ ] OpenAPI 源契约已更新
[ ] bindings 已重新生成
[ ] 内嵌 OpenAPI 文档已同步
[ ] 路径、operationId、Schema 和响应 Header 测试已更新
[ ] 管理写操作使用管理端路径
[ ] 适用的 If-Match/ETag 行为已覆盖
[ ] 生产写入使用默认授权策略
[ ] migration、Record、Mapper、Repository 已同步
[ ] 适用的真实 PostgreSQL 行为已验证
[ ] 适用的真实 HTTP E2E 已验证
[ ] `task fmt:check`
[ ] `task api:generate:check`
[ ] `task api:docs:check`
[ ] `go test ./...`
[ ] 命令、路径和配置与当前实现一致
```

## 9. 文档

本文随 REST 路由、OpenAPI 生成流程、QA 入口和测试门禁更新。业务规则维护在 feature 和测试中，架构决策维护在 `docs/services/backend/architecture/`，模块扩展规则维护在 [`module-extension.md`](module-extension.md)。本文中的命令与代码路径均以 `backend/` 为工作目录。
