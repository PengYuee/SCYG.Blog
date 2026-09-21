# SCYG.Blog 后端简化重构方案：Feature-first 模块化单体

| 项目 | 内容 |
| --- | --- |
| 状态 | 已实施；当前运行事实以 [`current-state-architecture.zh-CN.md`](current-state-architecture.zh-CN.md) 为准 |
| 版本 | 1.0 |
| 适用范围 | `backend/` Go module |
| 改造性质 | 内部代码与目录完全重构；默认保持 REST/OpenAPI、数据库 schema、既有数据和业务行为兼容 |

## 1. 决策

后端采用 **Feature-first 模块化单体**：业务模块按 feature 组织。feature 内部的文件拆分采用“业务边界优先、技术职责其次”：先按业务主体、生命周期、不变量、事务边界或独立变更原因确定文件归属，再在该边界内区分 Service、Repository、持久化 Record、Projection、Mapper、校验和错误。复杂的多 feature 写入由所属顶层模块的 `application/` 中具名业务动作协调。

文件名使用 `<business>_<role>.go` 表达导航意图，例如 `article_type_service.go`、`article_type_repository.go`、`article_type_record.go`、`article_type_query_repository.go`。不得创建跨业务的通用 `query.go`、`write.go` 或把 Service、Repository、Record、Mapper 混在同一文件。文件大小只是拆分触发信号，不是硬阈值；不按函数数量机械拆文件。

不采用 DDD、Clean Architecture 或 Hexagonal Architecture 的目录和抽象模板。业务模块内不创建 `domain`、`ports`、`workflow`、`contract`、通用 Repository、Repository interface、Service Locator、`common`、`shared` 或 `utils`；现有 REST 协议校验目录 `transport/rest/contract/` 保持不变。

目标是缩短定位路径，而不是增加层数：

```text
修改文章状态规则       -> content/article/*_service.go
修改文章列表读取       -> content/article/*_query_service.go 或 *_query_repository.go
修改文章持久化记录     -> content/article/*_record.go
修改标签与分类         -> content/taxonomy/<subject>_<role>.go
修改图片上传/引用/清理  -> content/image/<lifecycle>_<role>.go
修改文章正文图片协作   -> content/application/article_images.go
```

## 2. 目标与边界

### 2.1 目标

1. 让一个业务 feature 的实现、数据访问和测试处于相邻目录。
2. 让 Service、Repository 和 `application/` 的职责可以从文件名直接判断。
3. 让简单跨模块调用保持显式、单向、窄接口化。
4. 让需要同一 SQL 事务的多 feature 写入集中到一个具名 `application` 动作。
5. 消除当前 `content.Module` 作为业务门面的增长路径。
6. 保持一个 Go module、一个进程、一个主关系数据库和手工 bootstrap 组合。

### 2.2 非目标

本方案不引入：

```text
微服务
拆库或分布式事务
CQRS / CommandBus / QueryBus
事件总线、Outbox、Broker
gRPC、WebSocket
通用 ORM 抽象
双数据库运行或双写
为未来 MySQL 预建接口、目录、migration 或 CI
```

MySQL 不是当前交付目标。若未来成为明确需求，按第 10 节演进；不提前支付双方言维护成本。

### 2.3 默认兼容边界

除非另有独立业务变更，本次重构保持：

```text
OpenAPI 路径、HTTP 方法、请求字段、响应字段和错误结构
PATCH 的省略 / null / 值三态语义
文章 Draft -> Published -> Archived 和软删除语义
乐观锁版本冲突语义
图片 pending -> committed -> orphaned 状态语义
图片暂存、最终 Blob 提交、补偿和清理语义
数据库 schema、迁移版本与既有数据
bootstrap 的数据库、Blob、worker 生命周期与关闭顺序
Create/Patch 的验证、授权、错误优先级与现有 Clock 调用边界
```

### 2.4 备选方案与取舍

| 方案 | 收益 | 代价与结论 |
| --- | --- | --- |
| 保留当前 DDD-lite 分层，只拆小文件 | 迁移量最小 | `content.Module`、ports、内部层级和跨目录定位成本继续存在，不能达成目标，拒绝 |
| 每个 feature 采用完整 DDD/Clean/Hexagonal 模板 | 编译期端口隔离更强 | 当前规模会增加 Repository interface、DTO/Persistence Record 转换和目录跳转，没有第二实现支撑这些抽象，拒绝 |
| Feature-first + 具体 Repository + 具名 application 动作 | 定位路径最短，跨 feature 事务所有者明确 | feature package 内的符号所有权不能全由 import scanner 证明，需依赖 Go 可见性、构造测试和 code review；接受该取舍 |

选择 Feature-first 不是取消边界，而是把硬边界集中在 package import、REST 窄接口、唯一 composition root、事务所有权和真实行为测试上；不为尚不存在的替换实现预建接口。

## 3. 当前事实与设计依据

以下是当前代码事实；目标设计不以旧目录作为必须保留的实现约束。

| 当前事实 | 依据 | 设计影响 |
| --- | --- | --- |
| 后端是一个 Go module，入口由 `cmd/` 承担 | `backend/go.mod`、`backend/cmd/` | 保持模块化单体，不拆多个 Go module |
| 依赖在 `internal/bootstrap/` 手工组合 | `internal/bootstrap/construct.go` | bootstrap 是唯一 composition root |
| REST 是独立 transport | `internal/transport/rest/router.go` | Handler 不写业务或 SQL |
| `content.Module` 同时持有文章、taxonomy、图片、读模型、UoW 和 Blob 协作者 | `internal/modules/content/module.go` | 删除其业务门面职责 |
| 文章创建与正文图片绑定需同一事务 | `article_command_usecase.go` | 需要一个明确的跨 feature transaction owner |
| Patch 需要预检后重新读取并提交，避免 Blob I/O 长时间持锁 | `article_command_usecase_patch.go` | `application/article_images.go` 固化这一时序 |
| 图片上传含 Blob 暂存、元数据写入、最终提交和补偿 | `article_image_usecase.go` | 图片上传仍是 image feature 的独立流程 |
| 图片引用替换涉及锁、引用计数和 orphan 状态 | `article_image_reference.go` | 文章图片协作不能退化为普通 CRUD |
| PostgreSQL 适配含 sequence 与错误翻译等方言细节 | `internal/modules/content/internal/postgres/` | SQL 方言只留在 feature Repository 和 migration |

## 4. 最终目录结构

```text
backend/
├── cmd/
│   ├── api/
│   │   └── main.go
│   ├── migrate/
│   │   └── main.go
│   ├── qa-database/
│   │   └── main.go
│   └── content-reconcile/
│       ├── main.go
│       ├── reconcile.go
│       └── report.go
├── api/
│   └── openapi.yaml
├── migrations/
├── internal/
│   ├── bootstrap/
│   │   ├── app.go
│   │   ├── construct.go
│   │   ├── dependencies.go
│   │   └── lifecycle.go
│   ├── generated/
│   │   └── openapi/
│   ├── platform/
│   │   ├── blobstorage/
│   │   ├── config/
│   │   ├── database/
│   │   ├── httpserver/
│   │   └── observability/
│   ├── transport/
│   │   └── rest/
│   │       ├── router.go
│   │       ├── error_mapper.go
│   │       ├── contract/
│   │       └── content/
│   │           ├── handler.go
│   │           ├── article_handler.go
│   │           ├── taxonomy_handler.go
│   │           ├── image_handler.go
│   │           └── mapper.go
│   └── modules/
│       ├── content/
│       │   ├── security.go             # 共享安全协作者；不含业务方法
│       │   ├── clock.go                # 共享 Clock
│       │   ├── article/
│       │   │   ├── api.go
│       │   │   ├── article_service.go 或按业务边界拆出的 *_service.go
│       │   │   ├── article_query_service.go / article_query_repository.go（仅在读取复杂时创建）
│       │   │   ├── article_repository.go
│       │   │   ├── article_record.go / article_gorm.go
│       │   │   ├── article_mapper.go
│       │   │   ├── article_validation.go
│       │   │   ├── errors.go
│       │   │   └── *_test.go
│       │   ├── taxonomy/
│       │   │   ├── api.go
│       │   │   ├── article_type_tag_service.go
│       │   │   ├── article_type_service.go
│       │   │   ├── tag_service.go
│       │   │   ├── <business>_query_service.go / <business>_query_repository.go（仅在读取复杂时创建）
│       │   │   ├── article_type_repository.go / tag_repository.go
│       │   │   ├── article_type_record.go / tag_record.go
│       │   │   ├── article_type_mapper.go / tag_mapper.go
│       │   │   ├── article_type_tag_validation.go / article_type_tag_pagination.go
│       │   │   ├── article_type_tag_repository.go
│       │   │   ├── article_type_tag_errors.go
│       │   │   └── *_test.go
│       │   ├── image/
│       │   │   ├── api.go
│       │   │   ├── <lifecycle>_service.go
│       │   │   ├── <business>_query_service.go / <business>_query_repository.go（仅在读取复杂时创建）
│       │   │   ├── <business>_repository.go
│       │   │   ├── <business>_record.go / <business>_gorm.go
│       │   │   ├── <business>_mapper.go
│       │   │   ├── <lifecycle>_blob.go / <lifecycle>_cleanup.go
│       │   │   ├── <business>_validation.go
│       │   │   ├── errors.go
│       │   │   └── *_test.go
│       │   └── application/
│       │       ├── article_images.go
│       │       └── article_images_test.go
├── tests/
│   ├── integration/
│   └── e2e/
└── docs/
```
目录只是导航，不要求 feature 对称。只有职责真实存在时才创建对应文件；简单 feature 也应使用最小的、带业务主体的职责文件名，不创建无业务主体的技术泛桶。

## 5. Feature 内部职责

### 5.1 `api.go`

`api.go` 公开协议无关的 Command、Query 和 Result。它不包含 HTTP DTO、GORM persistence Record、数据库连接、Blob 实现，也不定义为某个 transport 或其他调用方量身定制的服务接口。
窄接口由消费方定义，例如：

```go
// transport/rest/content/handler.go
type articleCommands interface {
    Delete(context.Context, article.Delete) error
}
```

feature 只实现调用方实际需要的能力；不因测试或形式要求预建 producer-owned 大接口。

### 5.2 Service 文件

Service 负责一个业务边界自己的动作：输入校验、授权、状态判断、调用 Repository、业务错误映射和单 feature 事务。文件先按业务主体或生命周期划分，再以 `_service.go` 区分职责：

```text
article/article_service.go
    文章生命周期动作

taxonomy/article_type_service.go、taxonomy/tag_service.go
    ArticleType、Tag 的创建、修改、删除

image/<lifecycle>_service.go
    上传、取消、读取或清理等独立图片生命周期动作
```

Service 不写 HTTP 响应，不处理 OpenAPI DTO，不持有其他顶层模块的完整 Service，也不包含 PostgreSQL 专有 SQL。

### 5.3 Query Service 与 Query Repository

复杂读取按业务主体拆为 `<business>_query_service.go` 与 `<business>_query_repository.go`；简单读取可合入该业务主体的 Service 或 Repository。`query` 是职责后缀，不是跨业务的顶层容器，也不是 CQRS 层。

### 5.4 Repository 文件

Repository 是业务边界的具体数据访问代码，不是 DDD Repository：

```text
GORM 查询、更新、行锁、分页、关联表操作
数据库错误映射
乐观锁条件更新
少量 PostgreSQL 专有 SQL
```

每个 Repository 是具体类型；不定义泛型 Repository，也不为未来 MySQL 预定义 Repository interface。文件名使用 `<business>_repository.go`，查询复杂时使用 `<business>_query_repository.go`。

Repository 不暴露给 REST，不跨顶层模块注入，也不承担授权或协议映射。

### 5.5 Persistence Record 文件

`<business>_record.go` 或 `<business>_gorm.go` 只放持久化行结构和与数据库行紧密绑定的关联记录：

```text
article/article_record.go
    ArticleRecord、TagArticleRecord

taxonomy/article_type_record.go、taxonomy/tag_record.go
    ArticleTypeRecord、TagRecord 及本业务边界的只读关联记录

image/<business>_record.go
    图片元数据与引用关系记录
```

持久化 Record 不等于领域实体、API Result 或 Projection；Mapper 负责边界转换。Record 与 Repository 同属 feature package，不创建独立 `storage` 子 package。

### 5.6 其他职责文件

`<business>_mapper.go` 只负责 Record、领域值和 API Result 之间的转换；`<business>_validation.go` 负责该业务边界的输入/内容校验；`errors.go` 负责 feature 稳定业务错误。图片 Blob I/O 使用具名 `<lifecycle>_blob.go`，清理使用 `<lifecycle>_cleanup.go`，不访问文章表、不决定文章引用。


### 5.8 共享协作者与动作所有权

content 根只保留 `security.go` 与 `clock.go` 中不承载业务方法的 `Action`、`Resource`、`Authorizer`、CurrentAuthor 与 Clock 等跨 content feature 共同使用的安全和时间协作者。根 package 不导入任何 child feature/application、GORM、transport、generated 或 platform/database；feature/application 可以单向导入本模块根。具体动作常量跟随行为所有者：文章动作归 article，taxonomy 动作归 taxonomy，图片上传与取消动作归 image，正文图片确认动作归 `application.ArticleImages`。迁移保持所有实际传给 Authorizer 的字符串值不变；当前未实际调用 Authorizer 的 `ActionReadArticleImage` 不构成运行时兼容契约，最终清理时删除。

## 6. `application/`：复杂多 Feature 协作

`content/application/` 不是 DDD Application Layer，也不是通用 workflow 目录。它只保存需要协调两个及以上 feature 写入的、具名的业务动作。

当前唯一必须存在的文件是：

```text
content/application/article_images.go
```

它处理：

```text
创建文章并绑定正文图片
修改文章正文并替换图片引用
移除最后引用后的图片 orphan
```

它拥有：

```text
操作顺序
唯一的 transaction owner
Create 的一次写事务
Patch 的一次短读取/预检事务和一次最终写事务
各事务 handle 向下传递
跨 feature 错误归并
最终写事务的整体 rollback 边界
```

它不处理：

```text
图片上传、读取、清理
普通文章 CRUD
普通 Tag / ArticleType CRUD
HTTP DTO
通用工具函数
数据库 Record 定义
```

### 6.1 文章图片协作时序

```text
REST
  -> application.ArticleImages.Create / Patch
      -> Create：解析文章与图片 key，执行文章授权
      -> Patch：文章授权
          -> 第一次短 transaction
              -> 读取文章、校验版本、重放 Patch、提取原始 key
              -> 回调返回前调用 image.ParseReferenceKeys
          -> commit
      -> 如有受控图片，application 执行图片确认授权、取得 CurrentAuthor，image 执行无写入的 Blob 可用性预检
      -> Create 的写 transaction / Patch 的第二次写 transaction
          -> 重新读取文章并重放 Patch（Patch 时）
          -> article.Service 的事务内写入
          -> image.Service 按稳定锁顺序重新验证数据库状态
          -> 替换引用并更新图片状态
          -> commit
```

图片上传不在该事务中：

```text
验证/重编码 -> 暂存 Blob -> 写 pending 元数据
            -> 事务外提交最终 Blob -> 成功或补偿
```

`application/article_images.go` 只协调文章正文对已上传图片的引用；不把 Blob 提交和补偿塞入文章事务。

### 6.2 事务方法规则

复杂协作中，`application` 是唯一 transaction owner。`ArticleImages` 持有 bootstrap 注入的 `*gorm.DB`，只调用 `Transaction`，不写 SQL，也不访问 Repository、Persistence Record 或表。被调用的 feature Service 提供显式事务内方法，并且不再自行开启 transaction。图片引用分为无授权、无 I/O 的 key 解析，事务前授权与 Blob 预检，以及最终写事务内的重验证和写入：

```go
// article.Service
func (s *Service) CreateInTx(ctx context.Context, tx *gorm.DB, command Create) (Article, error)
func (s *Service) PreviewPatchInTx(ctx context.Context, tx *gorm.DB, command Patch) (PatchPreview, error)
func (s *Service) PatchInTx(ctx context.Context, tx *gorm.DB, command Patch) (Article, error)

// image.Service
func ParseReferenceKeys(raw []string) (ReferenceKeys, error)
func (s *Service) PrepareReferences(ctx context.Context, authorID content.AuthorID, keys ReferenceKeys) (PreparedReferences, error)
func (s *Service) ReplaceReferencesInTx(ctx context.Context, tx *gorm.DB, articleID int64, prepared PreparedReferences, now time.Time) error
```

`PatchPreview` 只携带第一次重放所得的文章结果和原始图片 key candidate，不携带可跨事务信任的数据库状态。application 必须在第一次 transaction 回调返回前将 candidate 交给 `image.ParseReferenceKeys`；第二次事务仍重新读取并完整重放 Patch。`PreparedReferences` 不是授权或数据库状态证明。`ReplaceReferencesInTx` 必须对新引用重新锁定并验证 key、owner、状态和过期时间，对移除引用重新统计引用数。无新引用时不执行图片确认授权或 CurrentAuthor，`PreparedReferences` 零值表示合法空集合并允许移除全部旧引用。`InTx` 不向 REST 暴露，也不跨顶层模块传播。本次重构不合并现有各业务步骤的 `Clock.Now()` 调用。

## 7. 跨模块协作规则

### 7.1 简单调用

简单、只读或无事务副作用的调用采用“调用方定义最小接口，bootstrap 注入实现”。

```go
// comment/article_lookup.go
package comment

type articleLookup interface {
    IsPublished(context.Context, int64) (bool, error)
}
```

调用方只知道所需能力，不持有对方完整 Service：

```text
comment -> articleLookup
content -> userActiveChecker（仅在实际需要时）
```

接口由消费者定义；不创建顶层 `contracts`、`ports` 或“所有模块通用服务接口”。

### 7.2 复杂调用

涉及两个以上 feature 写入、同一最终写 transaction、补偿、锁顺序或并发语义的动作进入该顶层模块的 `application/`：

```text
content/application/article_images.go
user/application/delete_user.go          # 仅当真实用户注销流程出现
```

业务动作属于谁，`application/` 就放在谁的顶层模块内。禁止创建全局：

```text
internal/application/
internal/workflow/
internal/orchestration/
```

### 7.3 依赖图必须无环

当前 content 的目标依赖：

```text
REST
 ├── article.Service
 ├── taxonomy.Service
 ├── image.Service
 └── application.ArticleImages
        ├── *gorm.DB（仅调用 Transaction）
        ├── article.Service
        └── image.Service

taxonomy.Service -> 无 feature 业务依赖
image.Service    -> 无 article 依赖
article.Service  -> 无 image 依赖；文章图片协作由 application 发起
```

`article` 不需要为 ArticleType/Tag 存在性而调用 `taxonomy.Service`；引用完整性由现有数据库约束和稳定错误映射保证。`taxonomy` 不得反向调用 `article`，除非产品新增明确的删除引用策略。

若出现真实双向协作需求，按以下顺序处理：

1. 判断是否只是数据库引用完整性，能否由 FK、唯一约束和查询解决；
2. 判断是否有唯一业务动作所有者；有则放入该模块的 `application/`；
3. 如果长期无法分配所有者，合并两个人为拆开的 feature，或创建一个具名业务模块；
4. 不用 interface 掩盖循环调用。

## 8. REST、bootstrap 与架构检查

### 8.1 REST

REST 只负责 OpenAPI DTO 映射、协议校验、HTTP 状态和响应头。保留生成代码要求的 aggregate `StrictServerInterface` Handler，但其每个方法只委托给窄的 feature 或 application API。

```text
文章创建 / Patch（含状态迁移） -> application.ArticleImages
文章删除 / 详情 / 列表 -> article.Service
分类、标签操作 -> taxonomy.Service
图片上传、取消、读取 -> image.Service
```

实施阶段 1 只把 aggregate Handler 拆成能力更窄的字段；这些过渡接口继续使用当前 content 根 Command/Query/Result、方法名和旧 `content.Module` 可实现的方法集。阶段 2～4 不把候选 feature Service 接入 REST。阶段 5 在唯一运行时切换提交中同时替换接口方法名、参数/结果类型、DTO mapper 与 bootstrap 注入，直接进入上面的目标依赖；不创建适配器或双路由。

REST 不接收：

```text
*gorm.DB
Repository
完整 content.Module
Blob 文件系统
跨 feature 内部对象图
```

### 8.2 bootstrap

`internal/bootstrap/` 是唯一构造具体对象的地方：

```text
打开 Database 和 Blob storage
    -> 创建 feature Repository
    -> 创建 Article / Taxonomy / Image Service
    -> 创建 ArticleImages application
    -> 创建 REST handler 与 cleanup worker
```

bootstrap 可以持有具体类型；其数据库生命周期接口显式提供 `GORM() *gorm.DB`，并且只有 bootstrap 将该 handle 传给 feature 构造器。业务 feature 不通过全局 Module 或 Service Locator 查找彼此。

### 8.3 Architecture scanner

架构扫描器只检查能够从 package import 确定的依赖边界：

```text
模块根共享 package 不导入 child feature/application、GORM、transport、generated 或 platform/database
feature 可以导入本模块根，但不导入 transport、OpenAPI generated、同模块 sibling feature 或本模块 application
application 可以导入本模块根、参与协作的 feature 与 GORM，但不导入 transport、HTTP DTO、database/sql 或 platform/database
transport 不导入 GORM 或 platform/database
一个顶层模块的业务 package 不导入另一个顶层模块的实现 package
bootstrap 可以组合全部具体实现
```

Repository、Persistence Record 和 Service 位于同一 feature package 时，scanner 无法只凭 import 判断调用方使用了哪个符号；同理，scanner 只能判断 application 是否导入 GORM，不能证明其只调用 `Transaction`。REST 是否持有 Repository 由 Handler 构造测试验证；application 的 GORM 与 feature 符号使用由最小导出面和 code review 保护，不新增扫描 SQL 字符串或方法名的脆弱源码测试。其他符号级所有权由 Go 可见性和 code review 保护。

## 9. 重构与验证策略

具体阶段、停止条件和命令以 [Feature-first 后端重构实施计划](feature-first-backend-refactoring-implementation-plan.zh-CN.md) 为唯一实施来源，本方案不重复维护易漂移的阶段清单。

实施遵守以下稳定规则：

1. 本方案正式批准前不迁移业务代码。
2. 阶段 0 先修复 `qa:database`：只接受显式 `QA_CONFIG`，由 `cmd/qa-database` 生成一次 QA run 的不可预测 ID 和专属子前缀，创建并记录 migration roundtrip 数据库，再让 integration/E2E 继续在该 run 前缀下创建各 fixture 独占的子数据库。编排器只对本次记录的 migration 数据库和 run 前缀下的子数据库执行 destructive migration/drop，并在结束时证明零残留；`task integration`/`task e2e` 必须把同一显式 `QA_CONFIG` 路径传给子进程，测试不得回退到 `config.local.yaml`；CI 只在 runner 临时目录物化基础配置，不依赖被忽略的本地文件。
3. 全部工作在独立重构分支完成；中间阶段是提交和验证节点，不是可部署版本。
4. 候选 feature 源码可在统一切换前与旧源码暂时共存，但任一时刻只能有一套实现进入 REST、worker 和 bootstrap 的运行时对象图；不双路由、不双写。
5. taxonomy、image、article/application 分别迁移其日常 unit 与真实 PostgreSQL 行为测试。保护 sequence、约束错误、事务回滚、正常引用替换、Blob 补偿和生命周期日常合同的测试必须保留；仅覆盖超大并发、锁顺序全排列、并发排列组合或多故障组合的测试按实施计划第 2.6 节删除。
6. 阶段 1 的 REST 窄字段继续使用旧根类型并由旧 `content.Module` 实现；阶段 5 才在唯一切换提交中同时迁移接口签名、mapper 与 bootstrap 注入。REST mapper 切换到 `stableFailure` 时，旧 `ApplicationError` 仅在过渡期实现同一能力；旧根类型和错误类型随旧架构一起删除。
7. 唯一生产切换点同时切换 REST、bootstrap 与 worker，并执行真实 integration、E2E、容器启动健康和 bootstrap 有界关闭门禁。
8. 最终删除旧 Module、UoW、ports、内部目录、临时兼容能力和不可达实现；不保留长期双实现。

最终门禁：

```text
task qa:static
task qa:database QA_CONFIG=<DISPOSABLE_QA_CONFIG>
task qa:container
```

`qa:database` 必须在一次 QA run 内通过 `cmd/qa-database` 实际执行 migration roundtrip、integration 与 E2E：migration roundtrip 使用编排器精确创建的数据库，integration/E2E 保留每个 fixture 独立建库并统一使用该 run 的专属前缀。编排器结束时必须证明 migration 数据库和所有子数据库均已删除。仅使用 `-run '^$'` 编译 tagged tests 不构成行为证据，隐式本地配置或无法证明所有权的数据库不得作为门禁目标。

### 9.1 发布只读对账边界

阶段 0 固定对账规格、静默窗口、状态判定和证据合同；article/image 解析能力迁移后，阶段 4 实现 `cmd/content-reconcile` 与 `task reconcile:content`。该工具复用 `article.ManagedImageReferences` 和 package-level `image.ParseReferenceKeys`，只使用带有不可变环境/实例标识的只读数据库身份和 Blob 读取能力，在停止写流量、应用进程与 cleanup worker 的静默窗口内，对活动及软删除文章的 Markdown 引用、关系表、图片状态、引用数和 Blob 大小/摘要执行一致性检查，并以有界临时文件样例和 `truncated` 标记表示目录扫描范围。判定必须先验证 Markdown/关系集合、状态和引用数不变量，再分类清理候选；只有引用数为零的过期 pending、过期且无引用的 orphaned 与过期临时文件可以作为既有清理候选，其他不一致阻断发布或回退并进入独立恢复方案。阶段 4 同时新增 `task reconcile:fixture`，由它创建随机数据库、临时 Blob 根和 SELECT-only 身份，调用实际命令并在结束时删除全部 fixture 资源；具体状态判定、证据路径和恢复矩阵以实施计划第 10 节为准。

## 10. 将来支持 MySQL 的演进

当前只实现 PostgreSQL。为未来降低迁移范围，遵守以下规则：

```text
Service 和 application 不写 PostgreSQL 专有 SQL 或 SQLSTATE 判断
feature/repository.go、migrations/、platform/database/ 承载方言差异
GORM 用于普通 CRUD、事务、分页和常规行锁表达
数据库错误先映射为 feature 稳定错误，再由 REST 映射 HTTP
```

真正支持 MySQL 时，按实际差异在原 feature 目录中增加文件：

```text
article/repository.go
article/repository_postgres.go
article/repository_mysql.go
```

全部保持 `package article`；不创建 `storage` 子 package，从而避免 Service、Persistence Record 和 Repository 的 import cycle。届时才增加：

```text
migrations/postgres/
migrations/mysql/
PostgreSQL 和 MySQL 两套真实集成测试
```

若需求是同一动作同时写 PostgreSQL 与 MySQL，则不属于本方案；那需要主数据源、异步复制、幂等、对账和最终一致性设计。

## 11. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 用 `application/` 重新造出万能层 | 仅允许具名、多 feature、复杂写入动作；普通 CRUD 留在 feature Service |
| feature 双向调用 | 保持无环依赖；使用数据库约束、明确流程所有者、合并强耦合 feature，而非 interface 掩盖循环 |
| Service 直接变成 SQL 杂物 | GORM 查询、锁和错误映射只在 feature Repository |
| Repository 演化为 DDD 抽象 | 只用具体类型；没有第二真实实现时不定义 interface |
| 重构顺手改变业务行为 | 将 API/schema/状态/事务/Blob 行为作为回归验收；新规则另立业务变更 |
| 删除现有 integration、并发或故障测试导致回归失去证据 | 迁移并实际执行保护事务、锁、补偿和生命周期合同的现有测试；只删除实现耦合或已有等价行为证据的测试 |
| 数据库门禁误伤共享数据 | `qa:database` 只接受显式 QA 配置；编排器生成不可预测 run 前缀，只删除本次记录的 migration 数据库及该 run 前缀下的测试子数据库，并验证零残留；无法证明所有权时在任何 `down`/drop 前失败 |
| 在线对账与并发写入交错产生误报 | 发布和回退均先建立静默窗口；数据库使用只读一致快照，Blob 仅在应用与 cleanup worker 停止后读取 |
| 把代码回退或 cleanup 当成通用数据恢复 | cleanup 仅处理引用数为零的过期 pending、过期且无引用的 orphaned 和过期临时文件；任何状态/引用数不变量错误、committed Blob 损坏或引用集合不一致均进入独立恢复方案 |
| 目录模板膨胀 | 只在职责存在时创建文件；`query.go` 和 `application/` 可不存在 |
| 未来 MySQL 预抽象 | 当前只保留 PostgreSQL；真实需求出现才拆 `repository_*` 与 migration |

## 12. 最终验收

结构：

```text
每个 feature 的 Service、Repository、Persistence Record 和测试相邻
content 根只包含 security.go、clock.go 与共享安全/时间协作者，不包含业务门面
复杂协作仅位于所属模块的 application/<business_action>.go
业务模块内不存在 domain、ports、workflow、contract、generic repository、Service Locator
不存在 content.Module 业务门面
```

依赖：

```text
REST -> feature/application 的窄接口
application -> `*gorm.DB`（仅建立 transaction）+ 参与协作的 feature Service
Service -> 自己的 Repository / Blob
Repository -> GORM / platform database
顶层模块间只通过消费者定义的窄接口；不共享对方 Repository、Persistence Record 或表写入权
```

行为：

```text
REST/OpenAPI 合同不变
文章、taxonomy、图片生命周期不变
乐观锁、授权、错误映射、Create/Patch 错误优先级与 Clock 调用边界不变
Blob 补偿、清理、nil storage 与 HTTP 缓存语义不变
Create 使用一次写事务；Patch 使用一次短读取/预检事务和一次最终写事务；文章与图片状态变更共享最终写事务并在其中重新验证图片状态
现有 PostgreSQL、并发锁、故障补偿和 E2E 行为测试实际通过；容器门禁证明镜像构建、启动与健康，bootstrap/E2E 证明有界优雅关闭
```

最终导航：

```text
文章       -> content/article/
分类与标签 -> content/taxonomy/
图片       -> content/image/
文章图片协作 -> content/application/article_images.go
REST 协议   -> transport/rest/content/
对象组合   -> bootstrap/
发布只读对账 -> cmd/content-reconcile
```
