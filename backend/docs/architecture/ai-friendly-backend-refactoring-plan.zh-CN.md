# SCYG.Blog 后端 AI 友好架构改造方案

> **历史方案（已被替代）。** 本文保留原始改造论证，不再作为当前目录、依赖或实施状态的权威来源。当前运行事实见 [`current-state-architecture.zh-CN.md`](current-state-architecture.zh-CN.md)，完成记录见 [`feature-first-backend-refactoring-implementation-plan.zh-CN.md`](feature-first-backend-refactoring-implementation-plan.zh-CN.md)。

| 项目 | 内容 |
| --- | --- |
| 状态 | **条件性采用，待阶段 0～2 设计验收后实施** |
| 版本 | 2.1 |
| 适用范围 | `backend/` Go 模块 |
| 改造性质 | 模块内部结构与依赖边界重构，不改变部署形态、数据库 schema 或业务语义 |

## 1. 执行摘要

当前后端已经是一个具备 `cmd`、`bootstrap`、`platform`、`transport`、`modules` 分层的 Go 模块化单体。现有技术分层基本合理，但 `content` 根 package 仍由 `content.Module` 统一承载文章、分类、标签、图片、清理、授权、读模型和事务协作者。

本方案的结论是：**采用能力边界重构，但不按目录图机械搬迁，也不一次性引入所有目标层级。** 首版只解决四个问题：

1. `article`、`taxonomy`、`media` 成为真实的 Go package 边界；
2. `content.Module` 收敛为组合入口，不再承载业务方法；
3. 单能力流程和跨能力 workflow 使用可证明的单一事务入口；
4. transport、bootstrap、测试和架构扫描器都遵守同一套依赖规则。

保留以下范围约束：不重写系统、不拆微服务、不引入 CQRS、Broker、Outbox、自动注册容器或通用 Repository，不修改数据库 schema，不改变 REST、权限、错误、图片 Blob 补偿和清理语义。

目标是“简单且可定位”，不是“目录最多”：

> 业务能力表达边界，package 表达真实依赖，workflow 表达跨能力事务，文件只按实际职责和规模拆分。

每个阶段要求可编译、可验证、可回退；不要求中间阶段具备长期兼容能力，也不允许新旧业务实现长期并存。

## 2. 决策范围

### 2.1 本次需要决定的事项

本方案决定以下内容：

- `content` 模块内部的能力划分。
- 能力公开 API 与内部实现的目录位置。
- 模块内部的 domain/application/adapter 依赖方向。
- content 内跨能力流程的归属。
- 现有 REST、数据库和图片能力的迁移顺序。
- 每个迁移阶段的交付物、验证方式、停止边界和回滚方式。

### 2.2 不在本次范围内的事项

本方案不决定以下内容：

- 微服务拆分。
- PostgreSQL 拆库。
- 独立读写数据库。
- Event Sourcing。
- Kafka、RabbitMQ 或其他 Broker。
- 通用 CommandBus、QueryBus 或自动注册容器。
- 统一通用 Repository。
- 未来 comment、user、notification 的具体业务设计。
- 没有真实消费者的 gRPC 或 WebSocket 契约。
- 现有数据库表的业务重设计。

如果未来需要上述能力，必须通过独立 ADR 说明触发条件、边界、迁移和运维成本。

## 3. 当前状态与证据

以下是当前代码确认到的事实，不是目标设计假设。

| 当前事实 | 依据 | 对改造的影响 |
| --- | --- | --- |
| 后端是唯一 Go module | `backend/go.mod` | 保留单 module，不创建第二个 `go.mod` 或 `go.work` |
| 入口由 `cmd/` 承担 | `backend/cmd/`、`backend/AGENTS.md` | 保留薄入口 |
| 依赖由 `internal/bootstrap/` 手工组装 | `backend/internal/bootstrap/dependencies.go` | 保留手工构造和显式依赖注入 |
| 基础设施位于 `internal/platform/` | `backend/internal/platform/` | domain/application 不得反向依赖平台实现 |
| REST 由 `internal/transport/rest/` 负责 | `backend/internal/transport/rest/router.go` | 保留 `rest` 命名，不改为泛化的 `httphandler` |
| `content` 根目录已有大量文章、分类、标签和图片文件 | `backend/internal/modules/content/` | 需要按能力收拢 |
| 领域层已经位于 `content/internal/domain` | `backend/internal/modules/content/internal/domain/` | 迁移时按已有职责搬迁，不机械重写 |
| 应用端口已经存在 | `content/internal/application/` | 复用现有 UnitOfWork、Repository、ReadModel 设计 |
| PostgreSQL 适配器已经存在 | `content/internal/postgres/`、`content/postgres/` | 保留内部适配器与模块组合入口的两级结构 |
| `content.Module` 持有多类文章、分类、图片协作者 | `backend/internal/modules/content/module.go` | 缩小为能力组合入口，不再继续增加业务方法 |
| REST Handler 已持有窄的读写服务接口 | `backend/internal/transport/rest/content/handler.go` | 保留窄接口模式，按能力继续拆分 |
| 文章创建会在同一事务中保存文章并绑定图片 | `article_command_usecase.go` | 迁移不能破坏跨表原子性 |
| 图片上传包含暂存、元数据保存、文件提交和补偿 | `article_image_usecase.go` | 不能把 Blob Storage 当成普通数据库 Repository |
| 图片有 pending、committed、orphaned 生命周期 | `article_image_value.go`、`article_image_lifecycle.go` | 媒体迁移必须保留状态和失败语义 |
| 架构扫描器当前识别 `internal/modules/<module>/internal/<layer>` | `backend/internal/architecture/imports.go` | 阶段一必须先扩展扫描规则，避免新目录绕过检查 |
| 当前架构测试通过 | `go test ./internal/architecture` | 作为迁移前基线证据 |

### 3.1 当前领域文件的实际职责

当前领域目录不是简单的“一类型一文件”。已有拆分大致如下：

```text
internal/modules/content/internal/domain/
├── article.go                    # Article 聚合与生命周期
├── article_reconstitute.go       # Article 持久化状态重建
├── article_validation.go         # 文章领域输入校验
├── article_type.go               # ArticleType 行为
├── article_type_reconstitute.go  # ArticleType 重建
├── tag.go                        # Tag 行为
├── tag_reconstitute.go           # Tag 重建
├── tag_article.go                # 文章与标签关联值
├── article_image_value.go        # 图片 ID、存储键、状态、媒体类型
├── article_image_lifecycle.go    # 图片创建、提交、孤儿化、取消
├── article_image_reconstitute.go # 图片重建
├── article_image_validator.go    # 图片内容安全校验
├── article_image_markdown.go     # 正文图片引用解析
├── article_image_jpeg.go         # JPEG 相关解析
├── identity_value.go             # ArticleID、ArticleTypeID、TagID
├── text_value.go                 # 标题、Slug、Name 等文本值
├── version_value.go              # Version 与版本递增
├── status.go                     # 文章状态
├── taxonomy_rule.go              # 分类和标签共享规则
├── clock.go                      # 领域时钟端口
└── errors.go                     # 领域错误
```

迁移时应保留这种“按职责拆分”的优点。`identity_value.go` 已经将同类 ID 分组，不应机械拆为 `article_id.go`、`tag_id.go`、`article_type_id.go`。只有当某个值对象具有独立且较大的行为时，才单独成文件。

### 3.2 必须纠正的当前事实

以下事实直接影响目标设计，不能按目标目录反推当前行为：

| 当前事实 | 依据 | 迁移要求 |
| --- | --- | --- |
| 目标 `article`、`taxonomy`、`media` package 当前不存在 | `backend/internal/modules/content/` 目录 | 这是 package 重构，不是 rename-only 文件搬迁 |
| `DeleteArticle` 只执行文章软删除，不处理图片 orphan | `article_command_usecase.go:95-105,119-135` | 不得把删除后 orphan 写成现有 workflow；若新增，另立业务变更 |
| 图片引用 orphan 发生在移除最后引用或取消 pending 图片 | `article_image_reference.go:87-117`、`article_image_usecase.go:143-168` | 保留现有状态转换和并发锁语义 |
| 图片上传和文章图片引用是两条不同流程 | `article_image_usecase.go:82-140`、`article_image_reference.go:53-122` | 不把 Blob commit/compensation 塞入引用绑定 workflow |
| `ArticleImageStorage` 端口当前没有实际实现或调用 | `internal/application/article_image_port.go` 及全仓调用关系 | 不把未使用抽象当作迁移基础 |
| `ImageFilesystem` 可为空，图片依赖降级为 unavailable 实现 | `content/postgres/postgres.go:45-49`、`module.go:77-92` | 保留部分可用构造和稳定失败语义 |
| Article、ArticleType、Tag 读模型当前共享一个 `ReadModel` | `internal/postgres/*_read_model.go`、`content/postgres/postgres.go:41-49` | 拆 read adapter 但共享数据库句柄和查询行为 |
| `ActionReadArticleImage` 已声明但当前读取用例未调用授权器 | `article_image.go:8-14`、`article_image_usecase.go:172-193` | 不在结构重构中顺手补充读取授权 |
| bootstrap 负责图片存储、worker、数据库和 REST 的生命周期 | `bootstrap/construct.go:149-192` | 保留资源所有权、关闭顺序和 worker 停止顺序 |

## 4. 目标架构模型

### 4.1 模块与能力的定义

本方案采用两级业务边界：

```text
content                         顶层业务模块
├── article                      文章能力
├── taxonomy                     分类和标签能力
└── media                        文章图片能力
```

当前 `article`、`taxonomy`、`media` 是 `content` 内的能力，不是三个独立顶层模块。它们仍共享内容领域、部署进程和 PostgreSQL 事务边界。

未来只有在满足以下条件时，才考虑提升为独立顶层模块：

- 有独立的业务生命周期；
- 有独立的数据库所有权；
- 有稳定的公开 API；
- 不再依赖 content 内部实现；
- 独立组织上下文确实能减少耦合。

### 4.2 总体依赖图

```text
cmd
└── bootstrap
    ├── platform
    ├── content/postgres              # bootstrap 可调用的组合入口
    └── transport/rest

transport/rest
    └── content/<capability>          # 只依赖公开 API；不依赖 internal/postgres

content/<capability>
    └── content/contract               # 仅限稳定错误、授权和外部引用契约

content/postgres
    ├── article/postgres factory       # 返回 article application ports
    ├── taxonomy/postgres factory      # 返回 taxonomy application ports
    ├── media/postgres factory         # 返回 media application ports
    ├── workflow coordinator           # 由同一 transaction-bound handle 组装
    └── content.Module                 # 仅供 bootstrap 读取能力入口、清理入口和策略

content/internal/<capability>/application
    ├── 自身 domain
    └── 自身 application ports

content/internal/<capability>/postgres
    ├── 自身 application ports
    ├── 自身 domain mapping
    └── platform/database 适配

content/internal/workflow
    ├── workflow-owned transaction coordinator
    ├── article application ports
    ├── media application ports
    └── article-media association ports
```

`content/postgres` 可以被 bootstrap 导入，但 transport 导入它必须由架构扫描器禁止；这是静态架构规则，不是 Go `internal` 的编译器隔离。各 PostgreSQL package 只导出构造函数和 capability application port，不导出具体 Repository 类型。

### 4.3 新人和 AI 只需要识别四条数据流

单能力写入：

```text
REST -> capability API -> application -> domain -> repository port
     -> postgres adapter -> 一个事务 -> PostgreSQL
```

跨能力写入：

```text
REST -> article API -> article application
     -> 已注入的 ArticleMediaCoordinator
     -> 一个提交 coordinator.Within
     -> article ports + media ports + association port
     -> 同一个 SQL 事务 -> PostgreSQL
```

图片 Blob 流程：

```text
验证/重编码 -> 暂存 Blob -> 保存 pending 元数据
            -> 事务外提交最终 Blob -> 按结果补偿
```

只读查询：

```text
REST -> capability API -> read port -> capability read adapter
     -> 共享 Database 只读句柄 -> View/Result -> REST DTO
```

只读投影的边界是 Query/View 的所有者，而不是被读取表的写入所有者。在同一个顶层 `content` 模块内，`article` 的 PostgreSQL read adapter 可以使用共享 `Database` 句柄连表读取 Article、ArticleType、Tag 等表，组装为 `article.API` 定义的只读 View；这不授予 article 修改 taxonomy 生命周期的权限，也不允许它导入 taxonomy 的 domain、application 或 postgres 实现。

`article.API` 是创建和修改文章的唯一对外入口。它先完成协议无关的解析、授权和执行计划生成，再调用注入的 `ArticleMediaCoordinator`；transport 不直接持有 workflow 或 `content.Module`。`ArticleMediaCoordinator` 是由 article application 声明的内部窄接口，具体实现位于 `content/internal/workflow` 并由 `content/postgres` 组装，因此 article application 不反向导入 workflow，避免循环依赖。

workflow 不得调用会自行开启事务的 Article 或 Media public command；Blob commit/compensation 不属于文章引用绑定事务。跨能力命令的提交阶段只能开启一次 coordinator `Within`。`PatchArticle` 可以在提交前执行无写入、无跨阶段锁持有的读取/预检；提交阶段必须重新读取并重放必要校验，以保持现有版本冲突、锁时机和错误优先级。

依赖必须单向：

```text
domain -> 标准库和本能力纯领域代码
application -> 本能力 domain、自己的 ports、content/contract
postgres adapter -> 本能力 application ports、domain mapping、platform/database
workflow -> 本能力 application ports、workflow association ports
transport -> capability public API、协议生成代码和 transport contract
bootstrap -> platform、content/postgres、transport 组合实现
```

## 5. 目标目录结构

### 5.1 后端总体目录

```text
backend/
├── AGENTS.md
├── README.md
├── Taskfile.yml
├── go.mod
│
├── api/
│   └── openapi.yaml
│
├── cmd/
│   ├── api/
│   │   └── main.go
│   └── migrate/
│       └── main.go
│
├── internal/
│   ├── bootstrap/
│   │   ├── app.go
│   │   ├── construct.go
│   │   ├── dependencies.go
│   │   └── lifecycle.go
│   │
│   ├── platform/
│   │   ├── config/
│   │   ├── database/
│   │   ├── observability/
│   │   ├── blobstorage/
│   │   └── httpserver/
│   │
│   ├── generated/
│   │   └── openapi/
│   │
│   ├── transport/
│   │   └── rest/
│   │       ├── router.go
│   │       ├── contract/
│   │       └── content/
│   │           ├── article_handler.go
│   │           ├── article_mapper.go
│   │           ├── taxonomy_handler.go
│   │           ├── taxonomy_mapper.go
│   │           ├── media_handler.go
│   │           ├── media_mapper.go
│   │           └── error_mapper.go
│   │
│   └── modules/
│       └── content/
│           ├── AGENTS.md
│           ├── module.go
│           ├── article/
│           ├── taxonomy/
│           ├── media/
│           ├── internal/
│           └── postgres/
│
├── migrations/
└── docs/
    ├── architecture/
    ├── modules/
    └── adr/
```

未启用前不创建：

```text
api/proto/
internal/generated/proto/
internal/transport/grpc/
internal/transport/websocket/
```

### 5.2 Content 模块目录

```text
internal/modules/content/
├── AGENTS.md
├── module.go                         # 组合对象，不承载业务用例
├── contract/                         # 少量跨能力稳定契约
│   ├── application_error.go
│   ├── authorization.go
│   └── identity.go
├── article/                          # 文章公开 Command/Query/Result/API
├── taxonomy/                         # 分类和标签公开 Command/Query/Result/API
├── media/                            # 图片公开 API、策略和结果
├── internal/
│   ├── article/
│   │   ├── domain/
│   │   ├── application/
│   │   └── postgres/
│   ├── taxonomy/
│   │   ├── domain/
│   │   ├── application/
│   │   └── postgres/
│   ├── media/
│   │   ├── domain/
│   │   ├── application/
│   │   └── postgres/
│   └── workflow/
│       ├── article_media.go
│       ├── transaction.go
│       └── association_ports.go
└── postgres/                          # bootstrap 可调用的模块组合入口
```

以上是边界示意，不是文件名清单。能力内只有在代码规模和职责确实需要时，才拆分 `commands.go`、`queries.go`、`views.go` 或独立 adapter 文件。不得为了满足目录图创建空 package、空接口或无消费者的未来结构。

### 5.3 Article 能力

Article domain 负责文章生命周期、文章自身字段和文章对 taxonomy 的本地引用值。Article 不导入 taxonomy domain，也不通过 taxonomy usecase 查询 ArticleType 或 Tag 的存在性；现有数据库外键、错误转换和事务时序保持不变。

Article application 负责：

```text
文章 Command/Query 的解析与授权顺序
Article domain 的调用
Article repository/read-model ports
单能力文章读写流程
```

正文图片引用属于文章输入语义。Goldmark 只放在 article adapter；adapter 输出 article-owned 的纯引用值，不直接返回 media domain 的 `StorageKey`。引用值的格式校验、受控 URL 规则和去重语义必须保持现有测试断言。

### 5.4 Taxonomy 能力

Taxonomy 包含 ArticleType 和 Tag 两个独立生命周期。即使二者位于同一 capability，也不强制合并 Handler、domain 文件或 Repository。

Taxonomy application 负责：

```text
ArticleType/Tag Command 和 Query
授权与版本校验顺序
各自 domain 和 application ports
```

Article 对 TagID、ArticleTypeID 只持有本地引用值。`TagArticle` 关联和 Article/Tag 当前删除语义属于明确的数据所有权矩阵，不得通过共享 taxonomy domain 解决。

### 5.4.1 关联数据所有权

下表是目标设计的唯一所有权记录；阶段 0 只允许核对当前实现与补充具体锁顺序，不得重新把这些关系放入 `common`、`shared` 或宽 Repository。

| 数据或行为 | 写入/生命周期所有者 | 跨能力访问方式 | 必须保持的不变量 |
| --- | --- | --- | --- |
| `ArticleType`、`Tag` 实体 | taxonomy | article 只持有自己的标识值，不持有 taxonomy 领域实体 | 保持当前各自的创建、修改、软删除和版本语义 |
| `TagArticle` 关联 | article | taxonomy 不访问 article Repository 或关联表 | 文章保存时的标签去重、软删除过滤和当前删除语义不变；本次不新增活动引用检查 |
| `article_image_references` 引用语义 | ArticleMediaCoordinator | article 传入已解析的引用值；media 只暴露图片状态转换所需 port | 锁定、替换、引用计数和最后引用 orphan 判定保持原语义 |
| 图片元数据、Blob 与 `pending/committed/orphaned` | media | coordinator 通过 media application port 请求状态转换 | media 不直接持有 Article 聚合或管理 `ArticleID` 关联 |

`ArticleTypeID`、`TagID` 在 article domain 中是本地引用值，不与 taxonomy domain 共享同一领域实体类型。关联表的物理表位置不改变其业务所有权。若未来需要禁止删除仍被活动文章引用的 Tag 或 ArticleType，必须作为独立业务变更，不得混入本次重构。

### 5.5 Media 能力

Media domain 负责图片元数据、`pending/committed/orphaned` 生命周期、内容验证和状态转换。Media application 分开维护以下流程：

```text
上传：验证/重编码 -> 暂存 Blob -> 保存 pending 元数据
    -> 事务外提交最终 Blob -> 按结果补偿

取消/读取：保持当前 owner、状态、unavailable 和 HTTP 结果语义

清理：候选事务 -> 逐项加锁复核 -> 删除最终文件 -> 删除元数据
```

`ImageFilesystem` 可以为空。构造器必须保留当前 unavailable adapter 行为，不得因为拆分 media 而改变构造失败时机。图片策略由 bootstrap/content 组合入口注入，REST 和 media application 使用同一不可变策略。

`ActionReadArticleImage` 当前仅声明未使用；本次重构不得顺手新增读取授权。

### 5.6 跨能力 Workflow

```text
content/internal/workflow/
├── article_media.go
├── transaction.go
└── association_ports.go
```

Workflow 只承载当前真实存在的文章—图片跨能力流程：

```text
创建文章并绑定正文图片
修改文章正文并更新图片引用
移除最后引用后的 orphan 处理
```

当前 `DeleteArticle` 只执行文章软删除，不包含图片 orphan 处理；本方案不把它列为现有 workflow。若未来需要该行为，必须另立业务变更方案。

workflow 不是第四个公开业务 API。创建和修改命令始终先进入 `article.API`，由 article application 解析、鉴权并生成 `CreatePlan` 或 `PatchPlan`，再通过其声明的 `ArticleMediaCoordinator` 进入 workflow。workflow 只接收协议无关的已解析计划，不接收 HTTP DTO、Gin Context 或 OpenAPI 类型。

Workflow 由自己拥有跨能力协调契约：

```go
type UnitOfWork interface {
    Within(context.Context, func(context.Context, Scope) error) error
}

type Scope interface {
    Articles() articleapp.Repository
    Images() mediaapp.Repository
    Associations() AssociationRepository
}
```

接口名称可按实际代码调整，但必须满足：一次提交 `Within` 同时提供不同能力的 ports，所有实现使用同一个 transaction-bound handle。Workflow 不得调用已经自行开启事务的 Article/Media public command，不得重新引入宽的万能 `content/application.Transaction`。

`article_image_references` 的锁定、替换、引用计数和 orphan 判定由 `AssociationRepository` 表达；Media repository 只负责图片聚合本身。`TagArticle`、`article_image_references` 的业务所有权以 5.4.1 为准。

#### PatchArticle 的固定时序

`PatchArticle` 不能为了目录重构而被压扁成单一读取或单一校验步骤。其目标时序为：

```text
解析 Patch、执行文章基础授权
    -> 读取当前文章并校验版本
    -> 应用 Patch、解析正文图片引用
    -> 按当前顺序执行图片提交授权、当前作者和文件可用性预检
    -> 一次提交 coordinator.Within：重新读取、重放 Patch、保存文章、替换引用、状态转换
```

预检阶段不得写入文章、关联或图片状态，也不得把事务锁跨越到提交阶段。提交阶段的任一失败必须回滚文章、关联和图片状态；版本冲突、授权、缺失图片和关联锁竞争的错误优先级以当前回归测试为准并在阶段 0 记录。

## 6. 模块公开 API 设计

### 6.1 公开 API 的内容

`content/article`、`content/taxonomy`、`content/media` 是模块对其他 package 暴露的能力契约。它们只暴露协议无关的 Command、Query、Result/View、ID 或外部引用和窄接口。

跨能力稳定契约统一放在 `content/contract`，仅允许包含：

```text
稳定 ApplicationError / ErrorCode
Authorizer、CurrentAuthorProvider
真正跨能力需要的外部引用值
```

`contract` 不是通用业务层，不放 Repository、领域实体、数据库连接、工具函数或万能 Service。`Clock`、`ArticleImagePolicy` 的归属按实际消费者确定；不得为了避免判断而创建 `common` 或 `shared`。

公开 API 不得包含：

```text
领域实体指针
GORM Model
Repository 实例
数据库连接
具体 Blob Storage
HTTP DTO
OpenAPI 生成类型
```

### 6.2 公开 API 和协议 DTO 的关系

```text
REST Request DTO
    -> capability Command/Query
    -> capability API
    -> capability Result/View
    -> REST Response DTO
```

HTTP、gRPC、WebSocket 不共享协议 DTO。当前没有真实 gRPC/WebSocket 消费者，不创建对应目录或契约。

### 6.3 API、Module 和组合入口

`content.Module` 只保存已经构造好的能力入口、清理入口和图片策略；它不是新增业务方法的 Facade，也不向 transport 暴露 workflow。bootstrap 通过 `content/postgres` 组合入口创建 Module，取得 `article.API`、`taxonomy.API`、`media.API`、`CleanupRunner` 和 media policy 后分别传入 REST/worker 构造器。

目标 REST 构造接缝等价于：

```text
NewREST(article.API, taxonomy.API, media.API, media.Policy, health, docs)
NewCleanupWorker(media.CleanupRunner, interval, logger)
```

具体函数名可以调整，但 transport 不得导入根 `content` package、`content/postgres` 或 `content/internal/**`。根 Module 的能力访问器只服务 bootstrap 组合；它们不得演变成带业务命令的转发方法。

### 7. 模块内部协作规则

#### 7.1 单一能力的写入流程

```text
Command
    -> capability application
    -> parse/validate input
    -> authorize
    -> capability UnitOfWork.Within
    -> load/create domain entity
    -> domain behavior
    -> capability repository port
    -> adapter
    -> commit
    -> application result
```

单能力 UnitOfWork 只能返回本能力的 ports，不返回其他能力的 domain 或 Repository。

#### 7.2 单一能力的读取流程

```text
Query
    -> capability application
    -> read-model port
    -> capability PostgreSQL read adapter
    -> application View/Result
    -> protocol DTO
```

当前 article、ArticleType、Tag read model 共享同一个 Database 根句柄；拆分后可以分 package，但不得新建数据库连接或改变 TagArticle 批量补充、排序、分页、软删除过滤和结果字段语义。

#### 7.2.1 只读投影与连表查询

同一顶层 `content` 模块内，能力自己的 PostgreSQL read adapter 可以为该能力公开的 Query 执行只读 JOIN、子查询或批量补齐。典型路径是：

```text
article Query -> article ReadModel -> SELECT ... JOIN ArticleType/Tag ... -> article View
```

该例中的表写入所有权不改变：ArticleType 和 Tag 仍由 taxonomy 管理；article 只拥有这个 Query 的 View 和查询语义。首版只采用以下边界，不引入通用报表服务、第二套读库或 CQRS：

| 规则 | 允许 | 禁止 |
| --- | --- | --- |
| Query 归属 | 由一个 capability API 声明 Query 和 View，所属 capability 的 read adapter 实现 SQL 投影 | 用另一个 capability 的 domain/usecase 组装 View，或让多个能力共同拥有一个普通业务 Query |
| 表访问 | 在本 `content` 模块内读取实现该 View 所需的表、视图或关联；沿用共享 `Database` 句柄 | 跨顶层模块直接 JOIN 数据表，或为了读取新增数据库连接、事务协调器 |
| 副作用 | `SELECT`、映射、批量补齐和分页/排序/软删除过滤 | 写入、状态转换、授权决策、领域校验或补偿逻辑 |
| 无明确归属的跨能力视图 | 仅在存在真实消费者时，建立名称与消费者明确的专用只读 projection，并在阶段 0 登记所有者、表范围和测试 | 提前创建 `common` 查询包、共享 Repository 或万能报表接口 |

read adapter 对其他能力表的了解仅限于稳定的存储投影字段；一旦查询需要执行业务规则、决定状态转换或写入关联，必须回到相应 capability API 或 `ArticleMediaCoordinator` 的命令流程。

#### 7.3 Content 内跨能力协作

正常情况下：

```text
article internal -X-> taxonomy internal
article internal -X-> media internal
taxonomy internal -X-> article internal
media internal -X-> article internal
```

跨能力流程只能由 `content/internal/workflow` 编排。Workflow 自己拥有最小协调契约：

```go
type UnitOfWork interface {
    Within(context.Context, func(context.Context, Scope) error) error
}

type Scope interface {
    Articles() articleapp.Repository
    Images() mediaapp.Repository
    Associations() AssociationRepository
}
```

`Scope` 的三个 port 使用同一个 transaction-bound handle。Workflow 不调用会自行打开事务的能力 public command，不通过多个 Module 模拟跨能力原子性，也不恢复一个包含所有 capability Repository 的宽 `Transaction`。只有 workflow 可以导入跨能力 application ports；单能力 application 不得导入其他能力的 domain/postgres。

#### 7.4 模块之间协作

未来增加 `comment` 或 `notification` 时：

```text
comment/application
    -> content/article public API
```

禁止：

```text
comment/application
    -X-> content/internal/article/domain
    -X-> content/internal/article/postgres
    -X-> content database
```

跨模块只能共享 ID、引用值、查询模型、稳定结果或真实存在的事件契约，不共享领域实体。

#### 7.5 跨模块事件

只有真实存在可靠通知需求时才引入事件。当前不创建 Outbox、Broker、Worker 或空事件目录。
## 8. 必须保持的业务不变量

这次是结构重构，不是业务重设计。以下行为必须保持。

### 8.1 Article 生命周期

当前 `Article` 具备：

```text
Draft -> Published -> Archived
Draft/Published -> Deleted
Archived 不允许再次修改或删除
版本每次实际状态变更只递增一次
过期版本必须拒绝修改
```

这些规则由当前 `domain/article.go` 和对应测试保护。迁移后测试应随领域代码移动，并保持断言不变。

### 8.2 Article 引用

当前文章保存：

```text
ArticleTypeID
TagIDs
```

文章聚合负责标签 ID 去重和文章自身引用规则。迁移前必须明确 `ArticleTypeID`、`TagID` 的归属，不能让 article 直接导入 taxonomy 的内部领域 package。

推荐做法：

- 对外 Command 使用协议无关的标识值；
- article domain 使用自己的引用值对象；
- taxonomy domain 使用自己的实体 ID 类型；
- 不跨能力共享完整领域实体。

### 8.3 ArticleImage 生命周期

当前图片状态为：

```text
pending
committed
orphaned
```

上传流程必须保留：

```text
验证图片内容
    -> 暂存 Blob
    -> 保存图片元数据
    -> 提交最终文件
```

失败时必须保留当前补偿策略：

```text
数据库失败 -> 丢弃临时文件
最终文件提交失败 -> 元数据补偿或交由清理流程收敛
```

这不是普通的“文件上传接口”，不能在迁移时简化为单一 Repository 调用。

### 8.4 REST 行为

以下内容首轮保持不变：

```text
OpenAPI 路径
HTTP 方法
请求字段
响应字段
错误状态码
错误响应结构
图片 URL
PATCH 字段的省略/null/值三态语义
```

## 9. 架构约束

### 9.1 Go package 约束

```text
domain
    只能依赖标准库和本能力内部纯领域代码

application
    只能依赖本能力 domain、ports 和协议无关的纯类型

adapter/postgres 或 adapter/markdown
    可以依赖对应技术库并实现 application ports
    不得被 domain 反向依赖

transport
    只能依赖能力公开 API和协议生成代码

bootstrap
    可以依赖具体组合实现
```

### 9.2 跨顶层模块约束

本节的“模块”指 `content`、identity 等顶层业务模块，而不是 `content` 内部的 article、taxonomy、media capability。前述 capability 可在其本地 PostgreSQL read adapter 中，为自己公开的 Query 只读访问同一 `content` 模块的表；该例外不允许跨顶层模块、写入、导入对方 domain/application/postgres，或绕过公开命令 API。

```text
模块 A 不得导入模块 B 的 internal/**
模块 A 不得导入模块 B 的 postgres/**
模块 A 不得访问模块 B 的数据库表
模块 A 不得持有模块 B 的领域实体
模块间不能形成循环 import
```

### 9.3 架构扫描器约束

阶段一必须扩展 `internal/architecture`，但只把依赖语义作为硬规则；文件名、文件数量、是否存在可选目录继续遵守 `backend/AGENTS.md` 的 review guidance。

扫描器先按下表分类，再判定 import；不得仅凭路径层数猜测 layer：

| 路径类别 | 允许的 content 内依赖 |
| --- | --- |
| `transport/rest/**` | `content/article`、`content/taxonomy`、`content/media`、必要的 `content/contract` |
| `bootstrap/**` | `content/postgres` 和公开能力类型；不得导入 `content/internal/**` |
| `content/<capability>` | 自身公开类型、`content/contract` 与自身实现所需的内部依赖 |
| `content/internal/<capability>/domain` | 自身纯领域代码 |
| `content/internal/<capability>/application` | 自身 domain、ports、`content/contract` |
| `content/internal/<capability>/postgres` | 本能力 application ports、platform Database 和本地映射；ReadModel 可只读 JOIN 同一 `content` 模块内实现本能力 View 所需的表 |
| `content/internal/workflow` | article/media application ports、association ports 和自身纯类型 |
| `content/postgres` | capability adapter factories、workflow factory、platform 具体实现与 Module 组合 |

`content/contract` 只允许被公开 API、application，以及为已声明错误类型进行映射的 transport error mapper 导入；domain、postgres adapter 和任意 transport helper 不得把它当工具包。新增契约必须有真实跨 capability 消费者，并在阶段 0 的契约清单中登记。

scanner 必须：

1. 正确解析 `internal/modules/<module>/internal/<capability>/<layer>`，同时得到 module、capability 和 layer；
2. 识别 `internal/modules/<module>/<capability>` 为公开能力 API；
3. 禁止 transport 导入根 `content`、`content/internal/**` 或 `content/postgres`；后两者中的 `content/postgres` 是 scanner 规则，不是 Go `internal` 的编译器隔离；
4. 禁止公开 capability API 反向依赖根 `content` 的业务类型；
5. 禁止 capability application 导入其他 capability 的 domain/postgres；
6. 只允许 workflow 导入跨能力 application ports 和 association ports；
7. 禁止 application/domain 导入 framework、transport、OpenAPI generated 或具体 postgres；
8. 保留当前对 Gin、GORM、生成协议代码和初始化副作用的检查。
9. 为现有跨表 Query 登记所属 capability、返回 View、读取表范围及分页/排序/软删除行为；确认 ReadModel 只读 JOIN 不引入 domain 依赖或写入路径。

正向 fixture 必须覆盖 bootstrap -> `content/postgres` 的合法组合；负向 fixture 必须覆盖 transport -> 根 `content`、`content/postgres`、`content/internal/**`，以及上述每条非法依赖。fixture 用来证明依赖边界，不用来强制某个文件名或空目录模板。新规则通过后，才开始业务 package 迁移。

## 10. 方案取舍

| 方案 | 优点 | 代价或风险 | 结论 |
| --- | --- | --- | --- |
| 保持当前 content 平铺 | 改动最小 | 业务上下文继续膨胀，AI 定位范围大 | 不采用 |
| 公开能力目录 + `content/internal` 实现 | 利用 Go `internal`，能力边界清晰，transport 无法直接访问实现 | 目录层级增加，需更新架构扫描器 | **采用** |
| 直接把 domain/application/postgres 放在 `content/article` 下 | 目录直观，迁移短期简单 | Go 编译器无法充分保护 transport，依赖规则易被绕过 | 不采用 |
| 每个能力提升为独立顶层模块 | 边界最强 | 当前业务共享事务和内容生命周期，拆分过早 | 不采用 |
| 引入完整 CQRS/事件总线 | 读写模型可扩展 | 增加一致性、运维和部署复杂度 | 不采用 |

## 11. 详细实施计划

迁移采用垂直切片，而不是先创建完整目录再横向搬迁所有文件。每个阶段结束后必须可编译、可验证；代码只能按逆依赖顺序回退最新连续阶段到上一个已验证版本。临时兼容只允许服务当前阶段，不能形成第二套长期业务实现。

### 阶段 0：基线与所有权决策

#### 目标

在任何 package 搬迁前，冻结现有行为并解决会改变接口或依赖方向的归属问题。

#### 必须完成的决策

1. ArticleTypeID、TagID、Version、Clock 的归属；Article 只持有本地引用值。
2. `article_image_references`、`TagArticle` 的数据所有权和跨表约束。
3. `content/contract` 中稳定错误、授权、当前作者和外部引用契约。
4. workflow coordinator、Scope、association ports 的 package 位置和 callback 签名。
5. 每个 PostgreSQL adapter 的导出 factory 和 application port 返回类型；不得导出具体 Repository。
6. `ImageFilesystem == nil` 时的 unavailable 构造语义、图片策略注入和 bootstrap 资源关闭顺序。
7. Markdown parser 的 article-owned 引用值、Goldmark adapter 和现有异常输入语义。
8. 当前权限实际调用矩阵；不得在重构中新增 `ActionReadArticleImage` 调用。
9. `article.API` 到 `ArticleMediaCoordinator` 的唯一调用路径、接口所有者和 `content/postgres` 组装方式；transport 不得接收根 Module 或 workflow。
10. `PatchArticle` 的预检/提交两阶段时序、每一步的授权位置、锁范围、重放规则和错误优先级。
11. 运行期状态恢复的只读对账项、允许的修复边界和证据保留位置。
12. 所有跨表 Query 的唯一所有者、公开 View、读取表范围及分页、排序、软删除过滤和批量补齐语义；无所有者的查询不得迁移。

#### 交付物

```text
能力与调用清单
数据/关联所有权矩阵
公开 API 与稳定错误清单
workflow transaction/Scope 接口草案
article API -> workflow 路由与 PatchArticle 时序图
adapter factory 清单
当前行为与测试映射表
跨表 Query/View 所有权清单
运行期状态对账与恢复清单
```

#### 验收

```text
go test -count=1 ./internal/architecture
task qa:contract
task unit
```

上述交付物必须各有唯一权威位置，且所有权矩阵、路由图、Patch 时序图与扫描器分类表互不矛盾。集成测试和 E2E 作为独立证据执行，不用普通 `go test ./...` 冒充 integration 验收。

#### 停止边界

任一 ID、关系表、跨表 Query/View、事务协调、API 到 workflow 路由、Patch 时序、错误契约、图片状态、运行期恢复、权限或资源生命周期未确定，都不得进入 package 搬迁。

### 阶段 1：架构扫描器与 fixture

#### 目标

让新边界由编译器和 scanner 共同保护，同时不把目录模板升级成僵化 whitelist。

#### 工作内容

1. 先实现路径分类表，再修正 `moduleLayer`，正确解析 module、公开 capability、组合 `postgres`、workflow 和 layer。
2. 增加合法的 bootstrap -> `content/postgres` fixture。
3. 增加 transport -> 根 `content`、`content/postgres`、`content/internal` 负向 fixture。
4. 增加 domain/application/framework、application/postgres、跨 capability domain/postgres 负向 fixture。
5. 增加只有 workflow 可以跨能力的正向/负向 fixture。
6. 保留当前 Gin、GORM、generated 和 `init` 副作用检查。
7. 增加 `content/contract` 方向与消费者门槛的正/负 fixture。
8. 明确文件名、文件大小、五层目录形状仍是 review guidance，不是硬 whitelist。
9. 为跨表 Query 增加正向 fixture：本能力 PostgreSQL read adapter 可只读访问同一 `content` 模块内所需表；同时保留 application 跨 capability domain/postgres 的负向 fixture。

#### 验收

```text
go test -count=1 ./internal/architecture
task ci
```

旧代码和新 fixture 必须同时通过；任何规则只能依赖 import 语义和可见性，不能依赖空目录是否存在。

### 阶段 2：公开契约与事务协调契约

#### 目标

先建立真实可用的能力入口和唯一跨能力事务入口，再迁移业务实现。

#### 工作内容

1. 创建 `content/article`、`content/taxonomy`、`content/media` 的实际 API 类型和窄接口。
2. 创建 `content/contract` 的稳定错误和授权契约，不复制三套 ApplicationError。
3. 让新 public package 成为 Command/Query/Result 的类型所有者；旧根 package 只能临时 alias 新类型，依赖方向不得反过来。
4. 定义 workflow-owned `UnitOfWork`、`Scope` 和 `AssociationRepository`。
5. 定义单能力 UnitOfWork 与 workflow UnitOfWork 的区别；禁止 public command 互相嵌套开事务。
6. 为同一 transaction-bound handle、回滚和 `ErrNestedTransaction` 建立契约测试。
7. 由 article application 声明 `ArticleMediaCoordinator` 窄接口；其实现由 `content/postgres` 注入，确保 article application 不导入 workflow。
8. 将 REST/worker 的构造接缝改为能力级窄接口，禁止传递根 `content.Module`。

#### 验收

```text
go test ./...
go test -count=1 ./internal/architecture
task unit
```

API 不依赖 `content/internal/**`、GORM、OpenAPI DTO 或 HTTP 类型。必须以编译和契约测试证明：transport 只持有 capability API，article application 只依赖 coordinator 接口而不依赖其具体实现，且 workflow 使用同一 transaction-bound handle。旧入口若仍存在，必须标记删除阶段和唯一迁移调用方。

### 阶段 3：Taxonomy 垂直切片

#### 目标

用 ArticleType/Tag 作为低风险样板，完成一次从 API 到数据库和 REST 的完整能力迁移。

#### 工作内容

1. 迁移 ArticleType、Tag domain 和各自 application usecase。
2. 保持授权顺序、版本冲突、软删除和当前删除语义；不得为 Tag 或 ArticleType 凭空新增活动 Article 引用检查。
3. 迁移 taxonomy repository、read model 和 mapper；不改变同一 Database 根句柄、分页、排序和软删除过滤。
4. 按现有职责保留 `article_type_handler.go`、`tag_handler.go`，不强制合并成单文件。
5. 将 taxonomy 测试移动到能力附近，拆除依赖完整 `content.Module` 的宽 fixture。

#### 验收

```text
go test ./internal/modules/content/...
go test -count=1 ./internal/architecture
task qa:contract
```

### 阶段 4：Article 与 Article-Media Workflow 垂直切片

#### 目标

先处理真实的跨能力事务，再迁移 Article 创建和正文修改用例，避免阶段之间出现编译断层。

#### 工作内容

1. 迁移 Article domain、reconstitution、validation 和本地引用值。
2. 将 Goldmark 语法解析与 article-owned 引用策略分开，保持去重、受控 URL 和异常输入语义。
3. 实现由 article application 声明、`content/postgres` 注入的 workflow coordinator；提交阶段的单次 `Within` 同时提供 Article、Image 和 association ports。
4. 将创建文章并绑定图片、正文引用更新、移除最后引用后的 orphan 迁移到 coordinator；文章公开命令仍只由 `article.API` 分派。
5. 按 5.6 的固定时序实现 PatchArticle：预检不写入且不跨 Blob I/O 持锁，提交事务重读并重放 Patch，不为了“简化”而合并或拆散现有故障边界。
6. `DeleteArticle` 继续只做软删除；不新增删除后图片 orphan。

#### 验收

```text
go test ./internal/modules/content/...
go test -count=1 ./internal/architecture
go test ./internal/transport/rest/content -count=1
```

必须额外证明：Article 与图片引用使用同一事务句柄，任一保存失败都回滚；预检后并发修改仍在提交阶段报版本冲突；版本冲突优先于缺失图片或图片提交授权；旧/新图片按稳定 ID 顺序锁定、替换引用后再计数并 orphan；最后引用 orphan 和恢复行为不变。

### 阶段 5：Media 垂直切片与 PostgreSQL factories

#### 目标

迁移图片能力，同时保留 Blob、数据库、清理和资源生命周期语义。

#### 工作内容

1. 迁移图片 domain、验证、生命周期、reconstitution 和 media application。
2. 分别保留上传、取消、读取、清理流程，不把它们合并为普通 Repository 调用。
3. 保持 temp -> metadata -> final Blob commit -> compensation 顺序。
4. 保持 pending/committed/orphaned、owner、ETag、Cache-Control、响应头和固定图片 URL 行为。
5. 当 `ImageFilesystem` 为空时继续构造 unavailable adapters；不改变错误发生时机。
6. 各 adapter package 导出 factory，返回 application ports；具体 Repository 类型继续未导出。
7. `content/postgres` 用同一个 database transaction handle 组合 article、taxonomy、media 和 workflow；不能直接构造其他 package 的未导出类型。
8. 拆分 read adapter，但共享 Database 根句柄和现有 Article TagIDs 批量补充行为。

#### 验收

```text
go test ./internal/modules/content/...
task integration
go test ./internal/transport/rest/content -count=1
```

integration 验收必须明确 PostgreSQL、文件系统和配置前置，覆盖 Repository CRUD、乐观锁、TagArticle、图片元数据、同一事务回滚、ReadModel 查询、锁竞争和 Blob 补偿。

### 阶段 6：Module、bootstrap、worker 和 REST aggregate adapter

#### 目标

完成组合入口和协议适配迁移，不改变外部 REST 或 bootstrap 生命周期。

#### 工作内容

1. `content.Module` 只保存 article、taxonomy、media API、清理入口和 media policy；不保存可被 transport 调用的 workflow 或业务转发方法。
2. 更新 bootstrap `NewContent`、`NewREST`、cleanup worker 和所有测试 fixture 的类型接缝：REST 分别接收 capability API 和 policy，worker 只接收 media cleanup 窄接口。
3. 保持 ImageFilesystem、数据库、cleanup worker 的所有权和逆序关闭顺序。
4. REST 保留一个薄的 generated aggregate Handler，实现完整 `StrictServerInterface` 并委托到能力 Handler。
5. 按现有职责组织 `article_handler.go`、`article_type_handler.go`、`tag_handler.go`、`article_image_handler.go`，不把文件名当作硬约束。
6. 保持 `captureArticleTypeImagePatch` 在 OpenAPI contract middleware 之前执行。
7. media Handler 继续负责 multipart spool、ETag、缓存、内容类型、下载头和有界读取。

#### 验收

```text
task qa:contract
task ci
```

并完成 REST 合同测试：路径、字段、错误映射、PATCH 三态、图片上传/取消/读取、ETag、304、缓存和响应头均不变；以构造测试和架构 fixture 证明 REST 未接收根 Module、workflow 或 PostgreSQL 实现。

### 阶段 7：删除旧入口、固化文档和最终验证

#### 目标

完成 clean cutover，删除旧根业务入口，留下单一推荐导航路径。

#### 工作内容

1. 删除旧根 package 的业务方法、临时 alias 和转发接口；根 Module 仅保留 bootstrap 所需的能力/清理入口。
2. 删除依赖完整 `content.Module` 的 universal test fixture。
3. 更新 `backend/AGENTS.md` 与 `content/AGENTS.md`，只记录硬边界、推荐导航和真实验证命令。
4. 更新架构 fixture、模块 ADR 和能力测试索引。
5. 不创建未使用的 integration/e2e、gRPC、WebSocket 或事件目录。

#### 完成标准

```text
go test ./...
task qa:contract
task ci
task integration
task e2e
```

integration/e2e 需在各自真实环境前置满足时执行，并保留退出码和完整日志。

最终必须满足：

- Article、Taxonomy、Media 各有唯一公开 API 入口；
- workflow 是唯一跨能力事务协调入口；
- transport 无法导入根 `content`、`content/internal/**` 或 `content/postgres`；
- content/postgres 不暴露具体 Repository 类型；
- bootstrap 资源所有权和关闭顺序不变；
- 图片 nil storage、Blob 补偿、清理和权限语义不变；
- generated REST aggregate adapter 仍完整覆盖 OpenAPI；
- 旧 `content` 平面业务入口不再存在；
- 没有长期双实现、永久兼容 alias 或宽 universal facade。
## 12. 阶段停止与回滚规则

### 12.1 代码回滚

每个阶段必须在同一阶段内完成编译和验证。代码只能按逆依赖顺序回退最新连续阶段，直到上一个已验证版本；不得承诺任意中间阶段都可脱离后续依赖独立 revert。阶段之间不要求部署双轨运行，因为本方案不改变部署形态和数据库 schema。

如果出现以下任一情况，停止下一阶段：

- 架构检查不能区分合法和非法依赖；
- public API、REST 合同或错误映射变化；
- bootstrap 资源所有权、worker 生命周期或关闭顺序变化；
- 领域测试、权限顺序或事务窗口变化；
- 图片 Blob 补偿、清理、nil storage 或状态语义变化；
- 需要长期保留新旧双实现才能继续；
- workflow 无法证明所有 ports 使用同一事务句柄。

### 12.2 运行期状态恢复

本方案不要求数据库 schema 变化；正常迁移不执行数据搬迁或双写。该前提不等于可以忽略运行期状态：新的 package 实现仍会写入文章、关联和图片元数据，并在事务外提交或补偿 Blob。

每次涉及 ArticleMediaCoordinator、上传、清理或图片适配器的发布后，都必须保留以下只读对账结果：

| 对账对象 | 检查 | 允许的恢复边界 |
| --- | --- | --- |
| 文章正文引用与 `article_image_references` | 引用解析结果、关联记录和引用计数一致 | 仅按现有引用替换/重试语义修复，不改文章业务字段 |
| 图片元数据与最终 Blob | `pending/committed/orphaned` 状态与 Blob 可读性一致 | 仅执行既有补偿、取消或清理收敛流程，不直接伪造 committed 状态 |
| 过期 pending/orphan 与临时文件 | 由现有清理候选、加锁复核和幂等删除流程处理 | 不绕过 worker 直接批量删除 |

对账结果、修复决定和命令真实退出码必须留存。若发现需要修改数据库表、字段、状态含义、关系约束或删除语义，必须停止并拆出独立数据库迁移或业务变更方案，不能混入纯 package 重构。

### 12.3 兼容路径

不保留永久兼容别名、旧 package 转发层或双写逻辑。临时 alias 只能由新 public package 作为类型所有者、旧根 package 单向指向新类型，并必须在阶段 7 删除。

### 12.4 验证分层

```text
结构/依赖：go test ./internal/architecture
REST/OpenAPI：task qa:contract
静态质量：task ci
真实 PostgreSQL/Blob：task integration
真实 HTTP/生命周期：task e2e
```

`task integration` 和 `task e2e` 需要各自的数据库、文件系统和配置前置；普通 `go test ./...` 不得作为 integration/e2e 覆盖声明。

## 13. 风险与缓解

| 风险 | 表现 | 缓解措施 |
| --- | --- | --- |
| 新路径绕过架构扫描 | `moduleLayer` 把 capability 当 layer；transport 可导入根 `content` 或 `content/postgres` | 阶段 1 先落地路径分类表，再增加逐类正/负 fixture |
| 类型与包循环 | 新 API 反向 alias 根 package，或 workflow/application 互相导入 | 新 public package 先成为类型所有者；阶段 2 先定 coordinator 方向 |
| 宽 Transaction 回潮 | capability ports 仍共享四类 Repository 和多个 domain | 单能力 UoW 与 workflow Scope 分离；同一 handle 契约测试 |
| adapter 可见性失败 | `content/postgres` 直接构造 sibling package 未导出 Repository | 各 adapter 导出 factory，返回 application ports，不导出具体类型 |
| 关联表归属错误 | media 直接承载 ArticleID、锁、计数和 orphan 规则，或 taxonomy 趁迁移新增引用删除限制 | 用所有权矩阵和 association port 固化现有写入者、删除语义、锁顺序和测试 |
| Module 重新膨胀 | 所有能力仍通过根 Module 方法暴露 | Module 只保存能力 API、清理入口、policy 和组合依赖；REST 不导入根 Module |
| Patch 时序被压扁 | 预检与提交合并导致错误优先级、Blob I/O 锁范围或并发语义改变 | 固定两阶段时序；提交阶段重读重放，并用版本冲突优先级和回滚测试锁定 |
| 图片状态/Blob 语义丢失 | pending、compensation、清理、nil storage 或 HTTP 结果变化 | 分开验证 upload/reference/cleanup/HTTP 四条流程 |
| 权限语义意外变化 | 重构中新增未使用的 `ActionReadArticleImage` 授权 | 阶段 0 建立实际调用矩阵，保持调用点和顺序 |
| REST 生成适配断裂 | 三个 Handler 无法独立实现完整 StrictServerInterface | 保留薄 aggregate generated adapter，保持 middleware 顺序 |
| ReadModel 行为变化 | 拆分后新建连接、丢失 TagIDs 补充或改变排序 | 共享 Database 根句柄，保留投影和分页测试 |
| 连表查询越界 | ReadModel 导入其他 capability 的领域逻辑，或借投影执行写入、状态决策 | Query/View 有唯一能力所有者；同一 content 模块内仅允许本地 read adapter 执行 `SELECT` 投影，跨顶层模块仍走公开 API 或另行设计 |
| bootstrap 生命周期变化 | 图片存储、worker、数据库关闭顺序改变 | 阶段 6 迁移构造接缝并运行 lifecycle 测试 |
| 文件拆分过细 | 大量几十行的 ID、状态和类型文件 | 文件名和数量只作 review guidance |
| 旧路径长期保留 | 新旧实现同时修改 | 临时 alias 单向且限阶段 2～6，阶段 7 删除 |
| 回退后状态不一致 | 代码已回退但引用、图片元数据或 Blob 已由新路径写入 | 限定连续阶段回退，发布后执行只读对账并按既有补偿/清理流程恢复 |
| 重构范围扩张 | 顺便引入 gRPC、事件总线、微服务或 schema 变化 | 没有真实触发条件不创建目录和依赖 |

## 14. 统一验收标准

### 14.1 结构验收

```text
content/article、content/taxonomy、content/media 各有唯一公开 API 入口
content/contract 只包含有真实跨 capability 消费者的稳定契约
content/internal/<capability> 只包含对应能力实现
content/internal/workflow 是唯一跨能力事务协调实现
content/postgres 只负责组合和 adapter factory 调用
transport/rest 只负责 REST 协议适配
```

目录和文件名是导航建议，不是单独的通过条件。

### 14.2 依赖验收

```text
domain 不依赖 framework、application、postgres、transport
application 不依赖其他 capability 的 domain/postgres、framework、transport
workflow 只依赖 capability application ports 和 association ports
transport 不依赖根 content、content/internal 或 content/postgres
content/postgres 不暴露具体 Repository 类型
模块不导入其他模块的 internal 或 postgres
bootstrap 负责组合、资源所有权和逆序清理
```

能力 PostgreSQL read adapter 可以为自己的公开 Query 只读 JOIN 同一 `content` 模块的表，但不得导入其他 capability 的 domain/application/postgres，也不得包含写入、授权或领域状态决策。

### 14.3 行为验收

```text
文章创建、修改、发布、归档、软删除行为不变
标签和分类生命周期与当前删除语义不变；不新增活动引用检查
版本冲突、授权顺序和事务窗口不变
图片 pending/committed/orphaned 行为不变
图片上传失败补偿、清理重试和 nil storage 语义不变
图片 URL、ETag、304、缓存、响应头和读取权限行为不变
跨表 Query 的 View、分页、排序、软删除过滤和批量补齐行为不变；其 ReadModel 不产生写入副作用
REST/OpenAPI 合同行为不变
数据库迁移和既有数据行为不变
bootstrap worker、数据库、Blob 资源关闭顺序不变
```

### 14.4 AI 协作验收

每个能力必须有唯一推荐导航：

```text
backend/AGENTS.md
    -> content/AGENTS.md
    -> content/<capability>/api.go 或 capability 公开入口
    -> content/internal/<capability>/application
    -> content/internal/<capability>/domain
    -> content/internal/<capability>/postgres
    -> 对应 capability 测试
```

跨能力需求才进入：

```text
article API
    -> article application
    -> ArticleMediaCoordinator
    -> content/internal/workflow
    -> workflow transaction/association ports
    -> article/media application ports
    -> 对应集成测试
```

必须通过以下可观察检查：

1. 修改文章发布规则不需要读取 taxonomy/media postgres；
2. 修改图片验证限制能够定位到 media domain/application 和 media 测试；
3. 修改正文图片引用能够定位到 article parser、article application、workflow 和 reference tests；
4. 任一 capability application 的非法跨能力 import 被 scanner 拒绝；
5. REST Handler 无法编译依赖根 `content`、`content/internal/**`，且 scanner 拒绝 `content/postgres`；
6. 典型需求的实现入口、实现层和测试入口各只有一个推荐路径。
7. 修改文章详情或列表的只读投影时，可从 article Query 定位到 article ReadModel；不需要修改 taxonomy 写入用例，也不会获得写入 taxonomy 表的路径。

## 15. 阶段 0 必须解决的事项

以下事项不是普通未决问题，而是 package 搬迁前的阻塞项：

1. `ArticleTypeID`、`TagID`、`Version`、`Clock` 的类型归属；
2. `TagArticle`、`article_image_references` 的所有权及跨表约束；
3. Markdown parser 的 article-owned 引用值和 Goldmark adapter 边界；
4. workflow coordinator、Scope、association ports 的包位置和签名；
5. adapter factory 的导出构造器和未导出 Repository 的可见性方案；
6. `ApplicationError`、授权、当前作者、图片策略等模块级契约；
7. optional image dependencies、nil storage、worker 生命周期和关闭顺序；
8. `content.Module` 的能力访问器形式和 bootstrap/REST/worker 测试接缝。
9. `article.API` 到 `ArticleMediaCoordinator` 的单向路由、接口所有者和无循环 import 证明。
10. PatchArticle 的两阶段时序、错误优先级、锁范围与重读重放规则。
11. 上线后的引用、图片状态、Blob 对账、恢复边界和证据保留方式。
12. 跨表 Query 的所有者、公开 View、允许读取的同模块表及回归测试位置。

这些事项不能通过创建 `common`、`shared`、`utils` 或宽 `Transaction` 回避。未来 gRPC/WebSocket 版本固定方式不属于本次方案，直到出现真实消费者前不设计。

## 16. 结论

本方案采用以下最小目标：

```text
content
├── contract             # 稳定错误、授权和必要外部引用
├── article              # 文章公开 API
├── taxonomy             # 分类和标签公开 API
├── media                # 图片公开 API、策略和结果
├── internal/
│   ├── article          # 文章 domain/application/postgres
│   ├── taxonomy         # 分类、标签 domain/application/postgres
│   ├── media            # 图片 domain/application/postgres
│   └── workflow         # 唯一跨能力事务与关联编排
└── postgres             # bootstrap 可调用的模块级组合入口
```

这套结构的成功标准不是目录完整，而是：

- capability 有唯一公开入口；
- 单能力流程只访问本能力 application ports；
- 创建和修改文章只由 article API 分派，跨能力流程再通过其注入的 workflow coordinator；
- 所有跨能力写入使用同一个事务句柄；
- `content/postgres` 不泄露具体 Repository；
- transport 无法访问根 Module、内部实现或组合 PostgreSQL；
- Module 只组合，不继续增长业务方法；
- REST、数据库、权限、图片、Patch 时序和 bootstrap 生命周期行为不变；
- 新人和 AI 可以从能力入口直接定位实现与测试。

实施顺序必须是：

```text
基线与所有权决策
    -> scanner 与 fixture
    -> public contract 与 workflow coordinator
    -> taxonomy 垂直切片
    -> article + article-media workflow 垂直切片
    -> media 与 PostgreSQL factories
    -> Module/bootstrap/worker/REST aggregate adapter
    -> 删除旧入口、固化文档、完整验证
```

在阶段 0～2 没有通过所有权、事务 coordinator、适配器可见性、错误契约和 scanner 验收前，不进入大规模业务迁移。

因此，本方案是**条件性采用**：它解决的是当前 content 根 package 的边界和可维护性问题，但明确拒绝把目录数量当作架构质量，也拒绝在结构迁移中偷偷改变业务行为。
