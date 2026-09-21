# Backend Agent 规则

Go 1.26.0 模块化单体基础工程。本目录是仓库中唯一的 Go module。

## 文档

- [`docs/guides/backend-development.md`](docs/guides/backend-development.md)
- [`docs/guides/module-extension.md`](docs/guides/module-extension.md)
- [`docs/architecture/`](docs/architecture/)

命令以 [`Taskfile.yml`](Taskfile.yml) 为准，API 以 [`api/openapi.yaml`](api/openapi.yaml) 为准。

## 常用命令

- `task format`：使用固定版本的 `go run`、gofumpt 和 goimports 格式化 Go 源码；
- `task fmt:check`：不修改文件，检查格式漂移；
- `task lint`：运行固定版本的 golangci-lint v2 和 nilaway；
- `task unit`：运行禁用缓存、启用竞态检测的单元测试；
- `task ci`：运行当前可构建的 Gate B 检查。

## 固定环境

- Go：`1.26.0`；
- Go 构建镜像：`golang:1.26.0-bookworm@sha256:2a0ba12e116687098780d3ce700f9ce3cb340783779646aafbabed748fa6677c`；
- PostgreSQL：`postgres:17.5@sha256:aadf2c0696f5ef357aa7a68da995137f0cf17bad0bf6e1f17de06ae5c769b302`；
- Task：`v3.49.1`；可以使用 `go run github.com/go-task/task/v3/cmd/task@v3.49.1 <task>`，或安装该精确版本。

## 工具隔离

- Taskfile 中的工具命令使用 `go run package@version`；不得将这些工具加入 `go.mod`，也不得通过 `tools.go` 导入；
- `task ci` 会拒绝版本不是 `v3.49.1` 的 Task 运行器。

## 工程边界

- `cmd/api/main.go` 的 50 行纯代码是 review 触发线和目标，不是必须遵守的硬上限；
- 不得创建根目录 `go.mod`、`go.work`、第二个 module 或面向未来的空模块占位目录；
- 使用手动构造函数注入；禁止 Wire、Fx、Dig、service locator 和可变全局变量；
- 手写 Go 文件的 250 行纯代码是 review 触发线和目标，不是必须遵守的硬上限；
- 必须为导出标识符、签名、字段和非显然逻辑添加注释。

## 模块组织与交付规则

- 文件组织以业务为先：先识别业务主体、生命周期、不变量、事务边界或独立变更原因，再在该业务边界内拆分技术职责。不得创建跨业务的通用 `query.go` 或 `write.go` 桶文件；
- 当一个 package 包含多个业务主体时，使用 `<business>_<role>.go`。每个文件必须有一个主要职责；Service、Repository、持久化 Record 和 Mapper 不得混写。辅助函数可以留在其所属职责文件中；
- 使用明确的职责后缀，例如 `_service.go`、`_query_service.go`、`_repository.go`、`_query_repository.go`、`_record.go`、`_projection.go`、`_mapper.go`、`_validation.go`、`_errors.go`、`_transaction.go`、`_blob.go` 和 `_cleanup.go`。`api.go`、`security.go` 和 `clock.go` 可以作为 package 导航锚点；
- 带 GORM tag 的数据库行结构是持久化 Record，不是领域实体。优先使用 `<business>_record.go` 或 `<business>_gorm.go`；如果通用的 `model.go` 会掩盖类型到底是领域对象、API 结果、Projection 还是数据库行，则不得使用它；
- 大文件应先按业务边界拆分，再按读写机制拆分。当代码包含不同不变量、授权规则、事务所有权、数据表或变更原因时进行拆分；不得仅为了一个函数一个文件而拆分。文件大小只是 review 触发条件，不是硬阈值；
- `internal/modules/content/article/`、`taxonomy/` 和 `image/` 分别拥有自身 feature 的规则、查询、Repository、持久化 Record、校验和稳定错误。feature 不得导入兄弟 feature 或 `content/application`；
- `internal/modules/content/application/` 负责具名的跨 feature 工作流和事务边界。可以向 feature 的 `InTx` 方法传递同一个事务句柄，但不得直接写 SQL、访问持久化 Record 或 Repository；Blob I/O 不得跨越数据库事务；
- `content/security.go` 和 `content/clock.go` 只包含共享授权、当前作者和时间协作者。根 content package 不得导入子 feature、application、GORM、transport 或 platform/database；
- Feature Service、校验和 application 代码不得导入 Gin、HTTP DTO、生成的 OpenAPI 或 platform/database；feature Repository 可以使用 GORM 和 platform/database 访问自身表。REST 必须声明由消费方拥有的窄接口，不得接收 `*gorm.DB`、Repository、Blob filesystem 或聚合模块 facade；
- 持久化使用带版本的 SQL migration，永远不得使用 `AutoMigrate`；每个 feature Repository 只拥有自己的表。OpenAPI 仍然是 REST 权威，生成的 bindings 必须保持同步；
- Bootstrap 负责构造、migration 检查、readiness、有界启动失败和逆序清理。跨顶层模块调用使用公共 API 或消费方定义的窄接口；模块不得导入另一个模块的 `internal/**`。

## 持久化与外部副作用不变量

- 生命周期、乐观并发控制和领取租约（claim）的写入必须使用显式列集合。持久化聚合不得使用 GORM 的 `Save` API 或无约束的全字段 `Updates`；必须将 ID、版本、状态和 claim 条件放入 `WHERE`，并在条件写入时检查 `RowsAffected`；
- 列表分页必须由数据库执行 `Count` 加 `Limit`/`Offset`，或使用 keyset pagination。排序表达式必须来自白名单，并包含稳定的二级排序键；禁止先加载完整结果集，再在内存中分页；
- 执行外部文件、Blob 或网络副作用的清理流程，必须采用短数据库领取租约事务：事务内领取记录，事务外执行副作用，之后使用精确的 claim token 完成或释放记录。领取租约必须有过期时间，外部操作必须幂等；
- 每个新 migration 必须同时提供 up/down 文件，更新 `CurrentVersion`，并覆盖 schema contract 和 migration recovery。Migration 必须可重试，且不得主动删除业务数据；
- 涉及 PostgreSQL 专用行为或遗留带引号标识符时，可以在所属 Repository 内使用参数化 Raw SQL；不应一概禁止 `Raw` 或 `Exec`；
- 数据库标识符命名必须与 migration 保持一致：新建表和字段统一使用未加引号的 `snake_case` 小写命名，GORM 的 `TableName` 和 `column` tag 必须逐项匹配 schema。已有带引号的 PascalCase 表和字段属于遗留兼容边界；新 feature 不得继续混用，也不得只修改 GORM tag 而不更新 schema。对于已经共享或部署过的 schema，统一命名必须新增可重试的版本化 migration，并同步更新 Repository、SQL、测试、fixture 和文档；对于当前尚未交付的首次实现，可以直接修改初始 migration，不需要为了兼容而增加迁移层；
- 共享审计字段只能放在 `internal/platform/persistence`，由 `AuditFields` 统一表达 `created_at`、`updated_at`、`deleted_at` 和 `is_deleted`；采用该类型的 Record 必须满足 `created_at NOT NULL` 以及其余字段与软删除一致性约束，并在 `*_record.go` 中匿名嵌入。写入必须使用 `persistence.NewAuditFields`，读取必须校验 `AuditFields.Validate`；`updated_at IS NULL` 的遗留行只能在 Mapper 中通过 `EffectiveUpdatedAt` 回退到 `created_at`；
- `AuditFields` 是跨 feature 的持久化约定，不是领域实体或通用 `gorm.Model`；不得放入 ID、Version、用户身份或业务状态转换，也不得使用 `gorm.DeletedAt`。必须关闭 GORM 的 `CreatedAt`/`UpdatedAt` 自动时间回调，由注入的业务 Clock 显式提供时间；
- 只有采用完整审计/软删除契约的表才能复用 `AuditFields`。图片 `pending`/`committed`/`orphaned` 生命周期表、关联表和 claim 表继续使用各自的 Record 字段；删除仍由所属 Repository 显式写入 `deleted_at`、`is_deleted`、`version` 及条件 `WHERE`，并检查 `RowsAffected`。
