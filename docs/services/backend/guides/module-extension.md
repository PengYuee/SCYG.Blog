# 业务模块扩展指南

新增模块只应发生在真实业务需求出现后，不提前创建空目录。交付门禁按变更风险选择，组织形式、命名和文件拆分用于 review 沟通，不是把未来能力预先挡在目录名或依赖名之外的扫描器合同。

## 三条交付路径

### 1. 现有模块中的简单功能

没有改变公开契约、数据结构、启动接线或跨边界依赖时，沿用最近的现有 feature/application 层和该层的聚焦测试，并运行 `qa:feature`。不要求迁移、OpenAPI、bootstrap、架构夹具、真实 integration 或 E2E；只有相应边界实际发生变化时才增加这些门禁。

### 2. 契约或数据功能

改变 REST、消息或其他外部契约，或改变领域持久化数据时，运行 `qa:contract`。涉及 schema 的变更还必须通过 migration roundtrip，并用真实数据库 integration 或 E2E 验证迁移后的可观察行为。OpenAPI 变更必须以源契约为准，完成生成和文档副本同步。

### 3. 生命周期、新模块或边界功能

新增业务模块、改变模块间边界、bootstrap、readiness、资源生命周期、数据库连接、容器交付或新的协议边界时，保留架构 import checks，并执行相关的 bootstrap、数据库、container 和 E2E 门禁。只运行实际受影响的门禁，但不得绕过已改变边界的验证。

## 不因组织形式预先阻塞

命名、文件大小、常见模块层、generic token 用词和未来技术名称属于 review guidance：它们帮助读者定位责任、发现模糊抽象和控制变更规模，review 可以要求更清晰的命名或拆分，但不应被描述为所有未来实现都必须满足的扫描器规则。文件拆分采用“业务边界优先、技术职责其次”：先按业务主体、生命周期、不变量、事务边界或独立变更原因确定归属，再在该业务边界内区分 Service、Repository、持久化 Record、Projection 和 Mapper。文件名优先使用 `<business>_<role>.go`；`api.go`、`security.go`、`clock.go` 作为导航 anchor。数据库行结构优先使用 `<business>_record.go` 或 `<business>_gorm.go`，不使用含义不清的通用 `model.go`。

新的未来能力在真正实现时 review。不会仅因为目录名、文件名或依赖名称包含新的协议、集成或技术词汇，就提前禁止该能力。评审关注实际的依赖方向、契约、资源生命周期和测试证据。

## 领域边界与应用协作

目录按业务领域划分。独立的业务概念、数据所有权、不变量、生命周期或变更原因构成领域边界；Service、Repository、Record、Mapper 等技术职责在该边界内组织。

应用用例负责协调一个或多个领域能力，并由用例所有者定义事务边界。用例名称、单个接口动作或一次请求不单独构成领域模块。

协议、框架、存储和传输机制属于适配层，不构成业务领域边界。只有在具备独立状态、持久化、生命周期和业务规则时，相关能力才提升为独立领域。

领域之间通过公共 API 或消费方定义的窄接口协作；应用层不得代替领域持有数据或业务不变量。未来能力在实际实现时创建，不预先创建占位目录或抽象层。

## 保留的硬规则

以下边界是实现约束：

1. `internal/modules/content/article/`、`taxonomy/`、`image/` 分别拥有本 feature 的业务实现、查询、Repository、持久化 Record、校验和稳定错误；先按业务主体或生命周期拆分，再按技术职责拆分。不得创建跨业务的通用 `query.go`、`write.go` 或大而全的业务文件；Service、Repository、Record、Mapper 不得混在同一文件中。
2. `internal/modules/content/application/` 只放具名跨 feature 协作。它可以持有 bootstrap 注入的事务句柄并调用 feature 的显式 `InTx` 方法，但不得写 SQL、访问持久化 Record/Repository 或承载 HTTP DTO。
3. `content/security.go` 与 `content/clock.go` 只提供共享安全、作者身份和时间协作者，不承载业务方法；根 package 不导入 feature、application、GORM、transport 或 platform/database。
4. feature 的 Service、校验与 application 不得出现 Gin、HTTP、OpenAPI generated 或 platform/database 类型；feature Repository 可以使用 GORM 和 platform/database 的错误适配，但只能访问本 feature 所有的表。
5. 每个 REST transport 在自身目录声明最小消费接口，将 DTO、状态、Header、multipart 和错误映射到 feature/application 类型。REST 不接收 `*gorm.DB`、Repository、Blob filesystem 或完整业务聚合对象。
6. 跨顶层业务模块调用只经过对方公共 API 或消费方定义的窄接口，禁止导入对方 `internal/**`。
7. OpenAPI 是 REST 源契约的唯一权威。公开行为变更必须同步生成 bindings 和文档副本，不手工编辑生成代码。
8. bootstrap 独占运行时构造，启动必须检查 migration 和 readiness 条件；构造失败与关闭必须按逆序、有界地清理已创建资源。
9. 跨 feature 复用的数据库字段约定放在 `internal/platform/persistence`；采用审计/软删除契约的 Record 在 `*_record.go` 中匿名嵌入 `persistence.AuditFields`。该类型只包含 `created_at`、`updated_at`、`deleted_at`、`is_deleted`，不承载 ID、版本、用户身份或业务状态转换。
10. `AuditFields` 的写入统一通过 `persistence.NewAuditFields`，读取通过 `Validate` 检查 `deleted_at` 与 `is_deleted`，Mapper 对 `updated_at` 为 NULL 的遗留行使用 `EffectiveUpdatedAt` 回退到 `created_at`。显式关闭 GORM 自动时间回调，不使用 `gorm.Model` 或 `gorm.DeletedAt`。
11. 图片状态表、关联表和 claim 表不因存在时间字段而套用 `AuditFields`；它们按自身生命周期保存 Record。软删除仍由 feature Repository 显式写入状态、版本和条件，并检查 `RowsAffected`。
```text
internal/modules/content/
├── security.go                    # Action、Resource、Authorizer、CurrentAuthor
├── clock.go                       # 共享 Clock
├── article/                       # Article feature；按文章业务边界与职责命名文件
├── taxonomy/                      # ArticleType、Tag；按业务主体与职责命名文件
├── image/                         # 图片、Blob、引用与清理；按生命周期与职责命名文件
└── application/
    └── article_images.go          # 文章正文图片跨 feature 协作

internal/platform/persistence/
└── audit_fields.go                # 跨 feature 的审计字段持久化约定
```

各 feature 的具体文件清单随实现演进维护，不能把不带业务主体的技术泛桶当作默认模板。文件应使用 `<business>_<role>.go` 导航；没有独立职责时，不为满足树形示例创建空文件。

文件名是导航规则，不是逐文件扫描合同。文件大小只是 review 触发条件；当多个业务主体、状态不变量、授权规则、事务边界、数据表或独立变更原因混在一个文件中时，应优先按业务边界拆分，再在每个业务边界内保留清晰的技术职责文件。没有独立职责时，不为满足树形示例创建空文件。

## 扩展现有 content

先判断行为归属：文章规则进入 `article`，分类与标签进入 `taxonomy`，图片与 Blob 生命周期进入 `image`；只有必须在一个数据库事务中同时修改多个 feature 时，才在 `application/` 新增具名动作。feature 的 Service 不直接调用 sibling feature。

实现顺序通常是：先用 Given/When/Then 锁定规则，再实现 feature Service 与 Repository，随后补充 REST、真实数据库和生命周期验证。跨 feature 动作必须明确事务 owner，并把同一个 transaction handle 传给各 feature 的 `InTx` 方法；Blob I/O 不得跨越数据库事务。

简单功能使用聚焦测试和 `qa:feature`。改变 REST 或数据契约时使用 `qa:contract`；schema 变更还必须通过 migration roundtrip，并用真实 PostgreSQL integration 或 E2E 验证。改变边界、bootstrap、资源生命周期、容器交付或新增顶层模块时，保留架构 import checks 以及受影响的 bootstrap、database、container 和 E2E 门禁。

## 新增顶层模块

只有出现真实且独立的业务领域时才创建 `internal/modules/<module>/`。先定义该模块的公共 API 和数据所有权，再由 bootstrap 构造具体实现；其他模块只能通过公共 API 或窄接口协作，不得共享内部 Repository、Persistence Record 或表。新增模块不复制一套空的 domain/ports/workflow/contract/generic repository 层。

新增能力时同步更新受影响的架构文档、source of truth 和验证命令；不要把未来协议、集成或技术名称仅因目录名预先加入禁止规则。
