# SCYG.Blog Feature-first 后端重构实施计划

| 项目 | 内容 |
| --- | --- |
| 状态 | 已完成；阶段 0～6 已实施，当前 Go 单元与架构验证已通过；需外部 PostgreSQL/容器的门禁仍以 CI 环境执行 |
| 版本 | 1.4 |
| 架构决策 | [Feature-first 模块化单体重构方案](feature-first-backend-refactoring-plan.zh-CN.md) |
| 实施范围 | `backend/` Go module 的 content Feature-first 重构；阶段 0 完成 QA 数据库隔离与 CI/Task 配置切换，阶段 4 实现发布只读对账命令及其受控 fixture，阶段 6 更新当前架构与扩展指南，并为历史方案补充替代标记 |

## 1. 目标与完成定义

本计划将当前 `content` 的根业务门面、`internal/domain`、`internal/application` 和 `internal/postgres` 重构为：

```text
content/
├── security.go              # 共享 Action、Resource、Authorizer 与 CurrentAuthor
├── clock.go                 # 共享 Clock
├── article/                 # 文章 feature
├── taxonomy/                # 分类与标签 feature
├── image/                   # 图片 feature
└── application/
    └── article_images.go    # 文章正文图片协作
```

每个 feature 的文件拆分采用“业务边界优先、技术职责其次”：

```text
api.go                         协议无关的 Command、Query、Result；调用方窄接口由消费方 package 定义
<business>_service.go          业务动作、授权、状态规则、事务编排
<business>_query_service.go    复杂读取用例；简单时合入同一业务 Service
<business>_repository.go       GORM、锁、关联表、数据库错误映射
<business>_record.go            当前业务边界的 GORM 持久化行结构
<business>_mapper.go            Record、领域值和 API Result 的边界转换
<business>_validation.go        当前业务边界的输入与内容校验
errors.go                       当前 feature 的稳定业务错误
```

先按业务主体、生命周期、不变量、事务边界或独立变更原因确定归属，再在该边界内区分职责；不得创建跨业务的 `query.go`、`write.go` 或大而全文件。

完成不等于新目录可以编译。必须同时满足：

1. REST、OpenAPI、数据库 schema、既有数据与业务语义保持兼容；
2. `content.Module` 不再作为业务 Facade 或 REST 依赖；
3. 文章、taxonomy、图片都只有一套运行时实现；
4. `content/application/article_images.go` 是文章与正文图片引用的唯一复杂协作入口；
5. 不保留 `domain`、`ports`、`workflow`、业务 `contract`、通用 Repository、Repository interface 或永久 compatibility alias；
6. 架构检查、REST 合同、常见业务路径和现有真实 PostgreSQL/HTTP 行为测试通过；本次不以增加测试数量为目标，但必须迁移并执行保护现有事务、锁、补偿和生命周期合同的测试。

## 2. 实施原则

### 2.1 分支与部署边界

在独立重构分支完成本计划。中间阶段可以作为代码提交和验证节点，但不作为生产部署版本。原因是根 `content.Module`、REST 构造接缝和事务协作会在重构期间变化，不能承诺任意中间节点均可独立部署。

每一阶段必须可编译并通过本阶段验证。开发分支只能回退到最近一个已验证提交；生产回退只能部署最近一个已发布、完整验证且与当前 schema 兼容的 release artifact，不能部署中间阶段提交。阶段 2～4 可以在重构分支内暂时保留旧源码，以维持尚未迁移的调用方编译；新旧实现不得同时被运行时入口调用，且所有旧源码必须在阶段 6 删除。由于本次默认不改 schema，不需要双写、数据回放或新旧服务并行运行。

### 2.2 不创建空结构

只在迁入第一个真实代码文件时创建目录：

```text
没有图片协作代码时，不创建 application/
查询简单时，不创建独立的 `<business>_query_service.go`
没有独立校验时，不创建 `<business>_validation.go`
```

目录树是结果，不是实施起点。

### 2.3 事务规则

- 单 feature 写入由 feature Service 自己开启一次 `db.Transaction`。
- `application.ArticleImages` 是文章正文图片协作唯一的 transaction owner：Create 开启一次写事务；Patch 先开启一次短读取/预检事务，在事务外完成授权与 Blob 可用性预检，再开启一次最终写事务。任何数据库事务都不得跨越 Blob I/O。
- `application` 持有 bootstrap 注入的 `*gorm.DB`，只调用其 `Transaction` 建立边界，不写 SQL，也不访问 Persistence Record、Repository 或表。
- 在 Create 写事务和 Patch 最终写事务中，`application` 将同一个 transaction handle 传入 `article` 与 `image` 的显式 `InTx` 方法；`InTx` 方法不得再开启 transaction，也不向 REST 暴露。
- `article` 的事务内创建、Patch 预检与最终重放负责文章规则、读取和保存；`image` 的无 I/O key 解析可由 application 在 Patch 预检事务回调返回前调用，事务内引用替换负责关系替换、图片状态变更与最后引用 orphan。Clock 使用同一个注入实例，但本次重构保留当前各业务步骤独立读取 `Clock.Now()` 的顺序，不合并调用或新增“整个动作共用一个时间值”的语义。
- Blob 上传的暂存、最终提交和补偿保持在 image feature；不塞进文章事务。

### 2.4 共享协作者与稳定错误

`content` 根 package 只保留不承载业务方法的共享安全与时间协作者：`Action`、`Resource`、`Authorizer`、`AuthorizerOrDeny`、`DenyAll`、`DevelopmentAuthorizer`、`ErrPermissionDenied`、`AuthorID`、`InvalidAuthorIDError`、`CurrentAuthorProvider`、`CurrentAuthorUnavailableError` 与 `Clock`。bootstrap 创建一个 UTC system clock，并将同一实例注入 article、taxonomy、image 与 `application.ArticleImages`；各业务步骤按阶段 0 固定的现有顺序读取时间。

共享 `Action` 类型与授权端口保留在 content 根；具体动作常量归拥有行为的 package：文章动作归 article，taxonomy 动作归 taxonomy，图片上传与取消动作归 image，`ActionCommitArticleImages` 归 `application.ArticleImages`。保持所有实际传给 Authorizer 的字符串值不变；当前从未传给 Authorizer 的 `ActionReadArticleImage` 不作为兼容契约，阶段 6 在确认无真实调用方后删除。

`ActionCommitArticleImages` 仅在正文含受控图片时由 `application.ArticleImages` 使用共享 `Authorizer` 执行；Create 为该动作继续使用 `Resource{Kind: "article", ID: 0}`，Patch 使用请求文章 ID，并保持动作字符串、资源 kind、资源 ID、无图片时的授权短路和调用顺序不变。图片读取不调用 `Authorizer`：committed 图片公开，pending 图片仅当前 owner 可读，orphaned 图片返回未找到。

稳定错误留在各 feature 的 `errors.go` 或具名 application 动作内部，不保留根 `ApplicationError` 或业务 `contract` package。`DenyAll` 返回共享 `ErrPermissionDenied`；各 feature 以及 `application.ArticleImages` 将 Authorizer 或 CurrentAuthor 失败归并为自己的 `permission_denied` 稳定错误。REST 在自身包内定义未导出的 `stableFailure`（`StableCode() string`）和固定 code 常量；`writeApplicationProblem` 通过 `errors.As` 识别该能力，`writeProblem` 只接收 `string`。feature/application 不导入 transport，REST 不依赖任一 feature 的具体错误类型，RFC 9457 code 与 HTTP 映射保持不变。

`image.Policy`、`PolicyOptions` 与默认值归 image feature 所有。bootstrap 从已验证配置构造一次不可变 policy，并将同一实例传给 image Service、cleanup runner 与 REST multipart 单文件暂存限制。`config.ArticleImages.UploadRequestBytes` 仍是 transport 的 HTTP 请求体上限：bootstrap 仅经 `httpserver.Options` 传入，不能并入 image.Policy；配置继续保证它大于 `MaxFileBytes`。

### 2.5 依赖规则

```text
REST -> article / taxonomy / image / content application 的消费者定义窄接口
application -> `*gorm.DB`（仅建立 transaction）+ article.Service + image.Service + 共享安全与时间协作者
feature -> 自己的 Repository + 必要的 Blob 协作者 + content 根共享安全与时间协作者
feature -/-> 同一顶层模块的其他 feature 或 `application/`；跨 feature 写入只由 application 协调
Repository -> GORM + platform/database
bootstrap -> 所有具体类型并负责注入
```

禁止：

```text
feature -> 同一顶层模块的其他 feature package 或 `application/`
REST -> *gorm.DB / Repository / Blob filesystem
一个顶层模块的业务 package -> 另一个顶层模块的实现 package 或数据表；跨模块能力只通过消费方定义的窄接口由 bootstrap 注入
feature -> transport / OpenAPI generated
application -> HTTP DTO / Gin Context
application -> SQL / Persistence Record / Repository / 数据表
```

### 2.6 测试范围

本次重构不以新增测试数量为目标，但现有测试只要保护可观察合同，就必须迁移到新公开构造并继续执行。门禁分为：

```text
结构与静态：架构 scanner、格式、lint、vet、build
协议与常见路径：REST/OpenAPI 合同、PATCH 三态、权限与稳定错误映射
真实 PostgreSQL：sequence、双引号标识符、唯一/外键错误、乐观锁、软删除与事务回滚
图片与生命周期：Blob 暂存/提交/补偿、正常引用替换、最后引用 orphan、清理复核、HTTP 缓存与启动关闭
```

阶段 1～4 还必须分别编译默认、integration tag 与 E2E tag 下的完整 package/test graph。`-run '^$'` 在这里仅是全模块编译证据，不能替代各阶段明确列出的真实行为测试。

现有测试中，凡是保护本次会移动的正常事务、持久化、权限、状态和生命周期合同的测试，必须迁移到新公开构造并继续执行；不因测试文件名包含 `concurrency`、`fault` 或 `integration` 就删除。日常场景至少保留：正常 Create/Patch/Delete/查询、版本冲突、权限拒绝、事务回滚、图片引用替换与清理复核。

以下测试可以按本计划的范围直接删除，不要求补做等价测试：仅覆盖超大并发规模、锁顺序全排列、故障组合全排列、极限输入矩阵、超大资源量、平台特有攻击变体或新的 E2E 叙事，且不承载上述日常可观察合同的测试。删除项只需在迁移清单记录“超出日常场景范围”的理由；如果测试同时保护日常合同，只删除极端分支并保留最小代表性场景。

本计划不要求为重构新增高并发规模、锁顺序全排列、故障组合全排列、极限输入矩阵或新的 E2E 叙事。安全边界只保留最小代表性门禁：缺少配置、错误维护库、未登记数据库、一个非法名称样例，以及同一只读身份执行一次代表性写入失败；不展开字符、角色属性、相邻前缀和故障组合的穷举测试。


## 3. 阶段 0：批准决策、建立基线与迁移清单

### 目标

先批准目标架构，再修复数据库验证入口的隔离契约，并固定外部行为、Clock 调用边界和现有 PostgreSQL 语义，防止重构过程把行为变化误判为目录变化。阶段 0 可以修改 QA/CI 基础设施并固定只读对账工具的规格、权限、输出和验收合同，但不得迁移业务运行时代码；依赖 article/image 新解析能力的对账工具在阶段 4 实现。

### 工作项

1. Feature-first 模块化单体重构方案已经批准并实施完成；阶段 6 已完成，目标结构和运行时接缝以当前架构文档为准。
2. 在执行任何 `qa:database` 前修复其隔离契约：删除对隐式 `config.local.yaml` 的依赖，要求调用方显式提供 `QA_CONFIG`。新增 `cmd/qa-database` 作为一次 QA run 的唯一编排器，并采用 capability-prefix 所有权模型：
   - `QA_CONFIG` 是独立、严格的 QA-only YAML，不复用 API 的 `config.local.yaml` 或完整运行时 YAML；提交 `config.qa.example.yaml` 作为唯一示例，`config.example.yaml` 只保留运行时配置。它只允许以下字段，`qaconfig.Load(path)` 必须拒绝未知字段、空值和环境变量覆盖，并删除当前未使用的 `database.dsn` / `DatabaseDSN` 合同：
     ```yaml
     qa:
       postgres_admin_dsn: postgres://<admin>:<password>@<host>:<port>/postgres?...
       database_prefix: scyg_qa_
       command_timeout: 120s
     ```
     `cmd/qa-database` 从此基础配置生成权限为 `0600` 的 run 配置；integration/E2E fixture 只读取该 run 配置以取得 admin DSN、run 前缀和超时。仅 `cmd/migrate` 及其双引号标识符 integration test 使用的临时 YAML 另由编排器生成，包含派生目标 `database.dsn` 与同一 admin DSN；前者由编排器以显式 `-config` 传给 `cmd/migrate`，后者只以 `MIGRATION_QUOTED_CONFIG` 的受限路径传给该 integration test。两者都不是 `QA_CONFIG`，也不得写入日志或 evidence。
   - 编排器读取基础 QA 配置，生成不可由调用方指定、至少含 128 bit 密码学随机量的 run ID，并物化本次运行的临时配置后实际执行 migration roundtrip、integration 与 E2E。run ID 必须零填充为 25 个 ASCII 小写 base36 字符；fixture 随机后缀必须为 13 个 ASCII 小写 base36 字符（至少 64 bit）。完整 run 前缀固定为 `<base-prefix><run-id>`，可按前缀枚举的子数据库固定为 `<base-prefix><run-id><marker><fixture-suffix>`；`base-prefix`、`marker` 和全部编码字段只允许 ASCII 小写字母、数字和下划线。连接后读取 PostgreSQL `max_identifier_length`，创建前必须验证 `len(base-prefix)+25+len(marker)+13` 不超过限制；完整保留 run ID 与 fixture 后缀，超长时失败而非截断。当前最长 marker 为 `content_postgres_`；使用示例配置的 8 字符前缀时，该布局恰为 63 字符，不得额外添加分隔符。
   - migration roundtrip 数据库由编排器创建，并以精确名称登记；integration/E2E 保留每个 fixture 独立建库，但只能从临时配置取得本次不可伪造的 run 前缀并派生子数据库名。子数据库的所有权证明是“本次 run capability + 服务端返回名称以完整 run 前缀开头”，不要求子进程把创建结果回写父进程 registry。`cmd/migrate` 的双引号标识符真实 PostgreSQL 场景保留，但不再由测试自行生成、创建、派生或删除未登记名称：编排器生成 `<run-id>migrate_quoted"`，验证其只含固定前缀、run ID 与固定 `_quoted"` 后缀、长度不超过 `max_identifier_length` 且不以完整 run 前缀开头，将其精确登记，并将名称与仅限该测试的 `MIGRATION_QUOTED_CONFIG` 路径分别通过 `MIGRATION_QUOTED_DATABASE` 和 `MIGRATION_QUOTED_CONFIG` 传给 `cmd/migrate` integration test。测试缺少任一变量立即失败；在执行 `up` 前，它必须验证临时配置中的 `database.dsn` 解码后的唯一数据库路径段与 `MIGRATION_QUOTED_DATABASE` 精确相等，随后只使用该配置执行 `up/down/version` 与状态断言。首次 `up` 仍由 `cmd/migrate` 验证“缺库时创建”，所有清理由编排器完成。该数据库不进入按 run 前缀扫描的子数据库集合，扫描完成后仍须验证精确登记对象已删除。
   - `internal/qa/config` 和 `internal/qa/database` 改为显式接收配置路径：`task integration` 与 `task e2e` 必须要求 `QA_CONFIG`，将该路径作为子进程环境传入；所有 QA、integration/E2E fixture 测试入口统一读取该路径并调用严格的 `qaconfig.Load(path)`，缺失时立即失败，任何此类入口都不得调用 `LoadLocal` 或回退到 `config.local.yaml`。双引号标识符的 `cmd/migrate` integration test 是唯一例外：它不得调用 `qaconfig.Load`，而是只从 `MIGRATION_QUOTED_CONFIG` 用 `cmd/migrate` 的运行时配置加载器读取 `database.dsn` 和 admin DSN，先核对其解码后数据库路径段与 `MIGRATION_QUOTED_DATABASE`，再执行测试；该专用配置 loader 必须拒绝环境覆盖。`internal/qa/database` 成为测试 fixture 创建、派生 DSN、终止连接和删除数据库的唯一实现位置；除该 helper 与 `cmd/qa-database` 外，测试包不得直接执行 `CREATE/DROP DATABASE`、枚举 `pg_database` 或自行从 admin DSN 派生目标 DSN。`task integration` 还接受显式 `PACKAGES` 以运行阶段级有界包集合。涉及 bootstrap 的测试在临时运行时 YAML 上必须禁用运行时环境覆盖：为 `bootstrap.Options` 增加默认 `false` 的 `DisableConfigEnvironment`，由 `bootstrap.New` 传给 `config.Options.DisableEnvironment`；生产 `cmd/api` 保持默认值，所有 QA/integration/E2E fixture 显式传 `true`，不得以并行测试中 `t.Setenv` 的方式清理环境。
   - `Taskfile.yml` 的 `integration` 与 `e2e` 必须在启动任何 `go test` 前拒绝空的 `QA_CONFIG`，将显式路径导出给子进程；仅阶段级直接调用可以以非空 `PACKAGES` 运行调用方给出的有界 package 集合，为空时才使用完整 `./...`。`cmd/qa-database` 的固定 full gate 必须显式调用 `PACKAGES=`，覆盖父环境中的同名变量，不能由调用方缩小 integration/E2E package graph。CI workflow 必须在 runner 临时目录物化 `config.qa.yaml`，把同一文件路径传给 `task qa:database QA_CONFIG=...`；不得依赖 `config.local.yaml`、`SCYG_*` QA 覆盖或仓库工作目录探测。
   - 删除公开的 `migrate:roundtrip` Task；阶段 0 完成后 `qa:database` 只能调用 `cmd/qa-database`，不得再调用任一 `migrate:*` Task。`migrate:up`/`migrate:down` 若继续保留，只能要求调用方显式提供配置路径，禁止默认读取 `config.local.yaml`，并且不得作为 QA 数据库的创建、回滚或清理入口；任何 QA roundtrip 只能由 `cmd/qa-database` 对其精确登记的 migration 数据库执行。
   - `cmd/qa-database -config <QA_CONFIG>` 是唯一固定的 QA 子进程编排入口，不接受调用方提供的任意命令或数据库名。它在已验证的 backend module root 中、按 `qa.command_timeout` 为每一步建立 context，依次执行精确登记 migration 数据库上的 `go run ./cmd/migrate -config <migration-config> up -> down -> up -> version`，再执行固定的 `task integration QA_CONFIG=<run-config> PACKAGES=` 与 `task e2e QA_CONFIG=<run-config> PACKAGES=`；仅 migration 子进程接收显式 `-config <migration-config>`，仅 integration/E2E 子进程接收 `QA_CONFIG=<run-config>` 与空 `PACKAGES`，双引号标识符 integration test 还接收已登记的 `MIGRATION_QUOTED_DATABASE` 与仅限该测试的 `MIGRATION_QUOTED_CONFIG`。每个子进程以编排器构造的环境启动：保留 Go/Task 与操作系统运行所需变量，但删除全部 `SCYG_*` 环境变量后再注入上述固定变量，不能信任或转发父进程的运行时配置覆盖。`Taskfile.yml` 继续是 direct integration/E2E 的唯一 flags 所有者，必须保留 `-race`、`-shuffle=on`、`-count=1` 与 `PACKAGES` 语义；编排器不自行复制或弱化这些 flags。每步记录命令名和真实退出码。子进程 stdout/stderr 必须先在内存中脱敏：移除任意 PostgreSQL 连接 URL，并替换本次基础、run 与 migration DSN 的精确值及其解析后密码的原始和 percent-encoded 形式，之后才可输出控制台或写入 evidence；原始流不得落盘、上传或作为 evidence。持久化的完整输出指保持顺序与全部非敏感内容的完整脱敏 transcript；测试以唯一哨兵凭据证明其不出现在控制台、日志或 evidence。任一子进程失败、超时、上下文取消或启动失败后，编排器仍进入同一 defer 清理路径；最终非零退出必须同时保留首个子进程失败和清理失败。
3. `cmd/qa-database` 在同一进程的 `defer` 中按精确登记名称删除 migration 数据库及双引号标识符测试数据库；对子数据库只使用参数化的 `starts_with(datname, $1)` 按完整 run 前缀枚举，逐个重新验证允许字符、长度、完整前缀和当前维护库后再终止连接并按安全引用的精确标识符删除，禁止使用 `LIKE`、调用方提供的通配模式或截断名称。已登记但因前置子进程失败而尚未物化的数据库，在完成同一维护库、服务端地址/端口与精确名称复核后以 `DROP DATABASE IF EXISTS` 视为已清理；不得因此覆盖首个子进程失败，也不得扩大为前缀外扫描。临时目标 DSN 只能由已验证的 admin DSN 派生：创建前只验证 admin 的 `current_database()` 为 `postgres`，并比较派生目标 DSN 与 admin DSN 的主机/端口；创建后连接目标数据库，验证其 `current_database()` 为精确登记名称且服务端地址/端口与 admin 一致；删除前再次验证 admin 当前库、服务端地址/端口和目标所有权。任一复核失败时不得执行破坏性动作。最后再次使用 `starts_with` 查询 `pg_database` 证明本次 run 零残留。缺少 `QA_CONFIG`、migration 精确登记、run capability、维护库复核或任一所有权条件时，必须在任何 `down`/`DROP DATABASE` 前失败。无副作用负向测试只需覆盖缺少配置、错误维护库、未登记 migration 数据库和一个非法名称样例；不要求对 `_`、`%`、引号、长度、相邻前缀或失败组合穷举。真实 PostgreSQL 正向测试证明正常 migration 数据库、双引号标识符路径和测试子数据库路径完成清理；所有权失败只证明不会执行删除，不展开多故障清理组合。`reconcile:fixture` 的 admin 身份必须同时具备 `CREATEDB` 与 `CREATEROLE`；编排器在创建任一数据库、角色或 Blob 根前查询 `current_user` 对应的 `rolcreatedb` 与 `rolcreaterole`，任一不满足立即失败。只读 fixture 只在自己创建的目标数据库内撤销 `PUBLIC` 的数据库、schema、现有表和 sequence 权限，再按精确表清单授予 `SELECT`；不修改共享实例其他数据库的 ACL。CI 在 runner 临时目录物化脱敏基础配置并显式传入；本地应用配置、共享开发库、预生产和生产库不得作为目标。同步更新 `Taskfile.yml`、对应 `internal/deliverytest` 的任务结构断言、数据库质量 workflow、`backend/README.md` 与配置示例中的真实命令和前置条件。
4. 记录当前 REST/OpenAPI 路径、请求/响应字段、错误状态码、PATCH 三态、图片上传的两层字节边界（HTTP `UploadRequestBytes` 与 multipart 单文件 `MaxFileBytes`）及文章路由委托：Create/Patch 进入文章图片协作，Delete/查询进入 article。
5. 固定 Create 的现有顺序：文章字段与创建状态解析 -> Markdown 受控图片位置和完整 storage key 校验 -> Create/可选 Publish 授权 -> 如有受控图片，再执行图片确认授权、CurrentAuthor 与 Blob 可用性预检 -> transaction。
6. 固定 Patch 的现有顺序：ID/版本/状态解析 -> 文章动作授权 -> 第一次 transaction 读取、版本校验、Patch 重放、图片引用提取与完整 key 校验 -> 如有受控图片，再执行图片确认授权、CurrentAuthor 与 Blob 预检 -> 第二次 transaction 重读、重放、保存与引用状态更新。版本冲突继续先于非法 key 和缺失图片文件返回；key 校验继续在第一次 transaction 回调成功返回前完成。
7. 固定 Clock 语义：Create 的文章创建与图片引用转换分别读取 Clock；Patch 的预检重放、最终重放与图片引用转换分别读取 Clock。本次重构不合并这些调用。若未来要共用一个 `now`，必须作为独立行为变更评审。
8. 将当前源码和测试按目标 feature 分类，形成迁移清单；每个测试标记为“迁移”“由等价行为测试替代”或“删除”。删除项必须记录理由；只有删除仍需保留的可观察合同时才需要记录替代证据，纯极端测试按 2.6 节记录“超出日常场景范围”即可。清单还必须分别列出阶段 1～5 可暂留的 legacy root 运行时源和为编译/验证这些源而暂留的 root `*_test.go`，逐项记录其允许的精确 import path；该逐文件 import 表是 scanner 临时白名单的唯一输入，不能由目录、package 名或 glob 推导。
9. 登记 PostgreSQL 专有点：序列取号、双引号标识符、错误翻译、唯一/外键约束、行锁顺序和实际使用的数据库函数。
10. 固定表写入权：article 写 `articles`、`article_tags`；taxonomy 写 `article_types`、`tags`；image 写 `article_images`、`article_image_references`。taxonomy 仅为自身软删除判定只读 `articles`、`article_tags`，不得导入 article package、Persistence Record 或写入这两张表。
11. 固定 taxonomy 删除语义：活动文章引用的 ArticleType 或 Tag 拒绝软删除；只有软删除文章保留的 `article_tags` 关联不阻止 Tag 删除；不新增其他删除规则。
12. 固定共享协作者、动作归属和错误映射：Action/Resource 值、授权顺序、CurrentAuthor、Clock 调用边界、REST 固定错误 code 与 HTTP 响应。
13. 明确当前删除文章仅软删除，**不新增删除后 orphan 图片行为**。

### 验收

先用无副作用测试证明 destructive migration 保护，再在与 CI 相同的 Go、Task、QA run、PostgreSQL 和容器前置下执行：

```text
go test ./internal/qa/... ./cmd/migrate/... ./cmd/qa-database/... -count=1
task qa:static
task qa:database QA_CONFIG=<DISPOSABLE_QA_CONFIG>
task qa:container
```

阶段 0 的命令验收还必须证明：空或非 QA-only 的 `QA_CONFIG` 在任何数据库连接前失败；缺少 `CREATEDB` 或 `CREATEROLE` 的 admin 身份在创建 fixture 资源前失败；目标数据库内 `PUBLIC` 撤权后，只读角色先显式执行 `SET default_transaction_read_only = off`，再以同一连接进行代表性 DML 与 DDL，二者均因缺少表或 schema 权限失败；阶段级非空 `PACKAGES` 有界运行不会偷偷退回 `./...`，而父进程预置非空 `PACKAGES` 时 `qa:database` 仍以 `PACKAGES=` 执行完整 package graph；带唯一哨兵凭据的失败子进程不会把凭据写入控制台、日志或 evidence；父进程注入的 `SCYG_DATABASE_DSN` 等运行时覆盖不进入 `cmd/qa-database` 的任一子进程，且 QA fixture 的临时运行时 YAML 不受该覆盖影响。上述证据与真实退出码一并保留。

保留真实退出码和完整脱敏输出。`qa:database` 必须证明 migration 数据库由本次编排器精确登记，integration/E2E 子数据库由本次 run capability 派生，并在结束时全部删除；无论正常退出还是子进程失败，均不得触及不属于完整 run 前缀的数据库。此基线实际运行现有 integration、E2E 和容器启动叙事，不得用 `go test -run '^$'` 的仅编译结果替代行为基线。

### 停止条件

以下任一项没有唯一结论时，不进入代码迁移：

```text
Feature-first 架构决策尚未批准
qa:database 仍依赖隐式本地配置、公开 roundtrip 仍可绕过编排器、不能证明 migration 精确登记及测试子数据库属于本次 QA run，或可能对该 run 之外的数据库执行 down/drop
ArticleImages 的两阶段 Patch 方法、transaction owner 和事务内重验证职责
Create/Patch 的验证、授权、Clock、预检/重读/重放顺序与错误优先级
图片锁顺序、引用计数和最后引用 orphan 规则
taxonomy 跨表只读删除判定
稳定错误到 REST RFC 9457 的映射、旧 ApplicationError 过渡方式，以及共享 security/Clock 切出后旧根调用的错误归并
最终 aggregate Handler 字段、数据库 handle 暴露方式与 bootstrap 构造接缝
数据库专有 SQL、约束行为和对应真实 PostgreSQL 测试
QA-only `QA_CONFIG` schema、run/migration 临时配置的职责边界，以及 `reconcile:fixture` 管理角色的 CREATEDB/CREATEROLE 前置与目标数据库有效只读权限
只读对账工具的 Blob 枚举、静默证据生产者与 schema、attempt 保留规则、状态判定、成功条件与阶段 4 落点
```

## 4. 阶段 1：更新架构扫描与测试构造接缝

### 目标

让 scanner 保护新架构的真实依赖关系，而不是继续强制旧的 DDD-lite 路径。

### 工作项

1. 先切出最终形态的根共享协作者，但不迁移根业务 façade：`content/security.go` 仅定义 `Action`、`Resource`、`Authorizer`、`AuthorizerOrDeny`、`DenyAll`、`DevelopmentAuthorizer`、`ErrPermissionDenied`、`AuthorID` 与 CurrentAuthor 协作者；`content/clock.go` 仅定义结构化 `Clock` 接口。它们不导入旧 `internal/**`、feature、application、GORM 或 transport。`DenyAll` 返回 `ErrPermissionDenied`；仍由旧 `content.Module` 使用的 `permission` 归并逻辑在过渡期将该 sentinel 映射回既有 `ApplicationError`，直到阶段 5 不再有旧业务调用方。候选 feature/application 各自归并为自己的稳定 `permission_denied` 错误。
2. 将 `content.Clock` 从 `internal/application.Clock` alias 改为 `clock.go` 的自有接口；旧 domain/application 接收的同形 Clock 可直接使用该接口。将 root `nilLike` 依赖迁入仍需它的共享构造代码或各自 feature，阶段 6 删除旧 `Module` 时不得遗留该隐式依赖。
3. 根 package 不再拥有最终业务动作常量：taxonomy、image、article 与 `ArticleImages` 分别在阶段 2、3、4 定义并使用相同字符串值的目标常量。旧根业务文件为维持唯一旧运行时实现而暂留的同名值只用于编译这些 legacy 文件；新 feature/application 不得引用它们，它们也不是 compatibility alias，阶段 6 连同旧根业务文件一起删除。
4. 更新 `internal/architecture` 的路径分类：

   ```text
   internal/modules/<module>/*.go
   internal/modules/<module>/<feature>/**
   internal/modules/<module>/application/**
   internal/transport/rest/**
   internal/bootstrap/**
   ```

5. 删除将 `domain`、`application`、`postgres` 作为硬编码 layer 的规则。
6. scanner 只验证 package import 边界：已切出的 `security.go`、`clock.go` 及阶段 1 后新增的根共享文件不导入任一 child feature/application、旧 `internal/**`、GORM、transport、generated 或 platform/database；feature 可以导入自身模块根共享 package，但不导入 transport、generated OpenAPI、同一顶层模块的 sibling feature 或本模块 `application/`；application 可以导入模块根、参与协作的 feature 与 GORM，但不导入 Gin、HTTP DTO、`database/sql` 或 platform/database；transport 不导入 GORM 或 platform/database；一个顶层模块的业务 package 不导入另一个顶层模块的实现 package；bootstrap 可以组合具体实现。
7. 阶段 1～5 的旧根 `content` 运行时源及其仍为旧实现编译/验证所必需的 root `*_test.go` 是唯一、显式的 scanner 临时例外：scanner 代码维护由阶段 0 迁移清单导出的“相对路径 -> 精确允许 import path 集合”表，逐项精确匹配、禁止 glob。白名单运行时文件只可保留 `content/internal/**` 依赖；白名单测试文件只可保留表中登记的 legacy root、legacy persistence 与测试构造所需 platform 依赖，且不得导入新 feature/application、transport 或 generated。未知根文件（包括测试文件）或白名单文件的未登记 import 必须失败。例外不适用于 `security.go`、`clock.go`、任何新根文件、feature 或 `content/application/**`。阶段 6 删除登记文件、白名单和该 scanner 例外后，根 package 的最终规则必须实际生效。
8. Repository、Persistence Record 与 Service 位于同一个 feature package 时，scanner 无法只凭 import 判断调用方使用了哪个符号；scanner 也不能证明 application 对 GORM 只调用 `Transaction`。REST 是否持有 Repository 由 Handler 构造测试验证；application 的 GORM 与 feature 符号使用由最小导出面和 code review 保护，不新增扫描 SQL 字符串或方法名的脆弱源码测试，也不将其伪装成 scanner 已保证的规则。
9. 将 generated aggregate Handler 改为文章查询、文章创建/Patch、文章删除、taxonomy、image 与 image policy 的独立字段，删除通用 `QueryService`/`CommandService` 及从其中断言 image Service 的方式。阶段 1 的消费方窄接口明确保留当前 content 根 Command/Query/Result 类型、当前方法名和旧 `content.Module` 可实现的方法集，只缩小每个字段的能力；这些签名是阶段 5 前的过渡构造接缝，不宣称已经依赖目标 feature package。
10. 将 REST 错误映射改为本地 `stableFailure`/固定 code：去除 `problem.go`、contract middleware 与 Handler 协议校验对根 `ApplicationError`、`ErrorCode` 的静态依赖，保持全部 RFC 9457 响应不变；未知 `StableCode()` 一律映射为内部错误，不直接形成公开 problem code。
11. 由于本阶段生产 `rest.New` 仍调用旧 `content.Module`，为旧 `ApplicationError` 临时增加 `StableCode() string`，使其满足新 REST mapper；该方法与旧错误类型在阶段 6 一起删除，不作为永久兼容层。
12. 生产 `rest.New` 在阶段 1～4 将同一个旧 `content.Module` 分别传入上述过渡窄接口，以维持唯一旧运行时实现；不得新增 `content.Components`、适配器层、双路由或第二套运行时实现。阶段 5 在同一个切换提交中将接口方法名、参数/结果类型和 mapper 一次性改为 article/taxonomy/image/application API，并由 bootstrap 直接注入新实现；阶段 6 删除根业务类型和过渡接口。
13. 固定 bootstrap 目标构造接缝：bootstrap 直接构造 taxonomy、image、article、`ArticleImages`、cleanup runner 与 REST；不以 `content.Module`、`content.Components` 或等价聚合结果转交给 REST 或 worker。阶段 5 才切换生产调用方。
14. 建立正向 fixture：REST 到消费者接口、`content/application` 到 GORM transaction 与 article/image、bootstrap 到各具体构造器；建立负向 fixture：根共享文件导入 child/legacy/GORM/transport/generated/platform database、REST 导入 GORM/platform database、feature 导入 sibling/application/transport、application 导入 Gin/HTTP DTO/`database/sql`/platform database、业务模块直接导入另一顶层模块实现 package。fixture 必须分别覆盖仅在阶段 1～5 合法的已登记 legacy 根运行时文件、已登记 legacy root `*_test.go` 的 persistence/platform 构造 import，以及白名单文件未登记 import 和未登记测试文件被拒绝的样例；阶段 6 删除这些样例和例外。
15. 将 fixture 从“目录是否存在”改为“import 是否违反边界”与“Handler 构造是否只接收对应能力”。

```text
go test -run '^$' ./...
go test -tags=integration -run '^$' ./...
go test -tags=e2e -run '^$' ./...
go test -count=1 ./internal/architecture
go test ./internal/transport/rest/content -count=1
task qa:contract
```

### 停止条件

scanner 无法在不依赖目录对称或空目录的前提下区分合法与非法 import 时，先修正 fixture 和规则；根共享文件、登记 legacy 根文件和新 feature 的分类必须有唯一结论。若规则需要识别同 package 内的 Repository/Persistence Record 符号，则改用构造测试、可见性或 code review，不扩张 import scanner 的承诺。

## 5. 阶段 2：迁移 Taxonomy 候选完整切片

### 目标

以低风险的 ArticleType 与 Tag 验证 Feature-first 目录、具体 Repository 和 REST 窄依赖是否足够简单。

### 目标目录

```text
internal/modules/content/taxonomy/
├── api.go
├── article_type_tag_service.go
├── article_type_service.go
├── tag_service.go
├── article_type_query_service.go
├── tag_query_service.go
├── article_type_query_repository.go
├── tag_query_repository.go
├── article_type_repository.go
├── tag_repository.go
├── article_type_record.go
├── tag_record.go
├── article_type_mapper.go
├── tag_mapper.go
├── article_type_tag_mapper.go
├── article_type_tag_validation.go
├── article_type_tag_pagination.go
├── article_type_tag_repository.go
├── article_type_tag_errors.go
└── *_test.go
```

### 工作项

1. 将 ArticleType、Tag 的 Command、Query、Result 移入 `taxonomy/api.go`。
2. 将 ArticleType 与 Tag 的动作分别迁入 `<business>_service.go`；两者共享的构造、持久化、映射、校验、分页和稳定错误辅助分别放入明确表示双主体的 `article_type_tag_service.go`、`article_type_tag_repository.go`、`article_type_tag_mapper.go`、`article_type_tag_validation.go`、`article_type_tag_pagination.go` 与 `article_type_tag_errors.go`；常量字符串值保持不变。
3. 将 ArticleType、Tag 的复杂读取分别迁入 `<business>_query_service.go` 与 `<business>_query_repository.go`；分页共用辅助放入 `article_type_tag_pagination.go`，不创建跨业务的无主体 `query.go`。
4. 将 ArticleType、Tag 的 GORM Record、业务 Repository、Mapper 与校验按业务主体拆分；软删除判定使用各自局部只读 `Article` / `TagArticle` Record，不使用 article package 的内部 Record。
5. 保持分类和标签独立的业务方法；不因共用目录把它们合并为一个大 Service 方法。
6. 保持当前删除语义：活动文章引用的分类或标签拒绝删除；已软删除文章的遗留标签关联不阻止标签删除；不新增其他引用删除规则。
7. 阶段 2 不把 taxonomy Service 接入 REST，也不把过渡 Handler 接口改成 taxonomy package 类型；阶段 1 建立的 taxonomy 窄字段继续使用旧根类型并由旧 `content.Module` 实现。新 taxonomy API 与 mapper 的签名切换统一在阶段 5 完成。
8. 迁移 taxonomy 的常见路径和真实 PostgreSQL 测试，包括 sequence、唯一/外键错误、乐观锁、活动文章引用阻止删除、软删除文章关联不阻止 Tag 删除。旧 taxonomy 根源码仅在重构分支内保留到阶段 6，以维持尚未切换的旧调用方编译；不得让 REST 同时调用新旧实现。

### 验收

```text
go test -run '^$' ./...
go test -tags=integration -run '^$' ./...
go test -tags=e2e -run '^$' ./...
go test ./internal/modules/content/taxonomy -count=1
task integration QA_CONFIG=<DISPOSABLE_QA_CONFIG> PACKAGES=./internal/modules/content/taxonomy
go test ./internal/transport/rest/content -count=1
go test -count=1 ./internal/architecture
task qa:contract
```

真实 PostgreSQL 用例必须执行；REST 在本阶段只证明构造接缝和合同未漂移，新 taxonomy 的生产路由统一在阶段 5 切换。

### 通过标准

```text
taxonomy 不依赖 article、image 或 content/application
taxonomy 候选实现的 API 足以在阶段 5 接入消费方窄接口；本阶段 REST 仍只使用旧根类型的过渡接缝
tag 与 ArticleType 的版本、软删除、分页、排序和错误语义不变
旧 taxonomy 根实现待阶段 6 删除，且未被新运行时入口调用
```

## 6. 阶段 3：迁移 Image 候选完整切片

### 目标

将图片的数据库元数据、Blob 生命周期、校验和清理集中到 `content/image/`，不改变其双失败边界。

```text
internal/modules/content/image/
├── api.go
├── <lifecycle>_service.go
├── <business>_query_service.go / <business>_query_repository.go（仅在读取复杂时创建）
├── <business>_repository.go
├── <business>_record.go / <business>_gorm.go
├── <business>_mapper.go
├── <lifecycle>_blob.go
├── <lifecycle>_cleanup.go
├── <business>_validation.go
├── errors.go
└── *_test.go
```

### 工作项

1. 将上传、取消、读取 Command/Query/Result 迁入 `image/api.go`。
2. 将上传、取消、读取业务动作、对应稳定错误、上传/取消动作常量及授权顺序迁入按生命周期命名的 `<lifecycle>_service.go` 与 `image/errors.go`；实际传给 Authorizer 的常量字符串保持不变。`ActionReadArticleImage` 当前没有真实授权调用，不迁入 image 或新增读取授权，阶段 6 随旧根动作常量删除。
3. 将图片格式、尺寸、像素、SHA-256、重新编码和 Markdown 无关的内容安全校验迁入 `<business>_validation.go`。
4. 将图片元数据和引用表 GORM persistence Record 迁入各自 `<business>_record.go`。
5. 将图片查询、行锁、状态更新、清理候选、引用计数和数据库错误映射按业务主体迁入 `<business>_repository.go` 或 `<business>_query_repository.go`。
6. 将暂存、提交、读取、删除最终文件与临时文件操作迁入具名 `<lifecycle>_blob.go`。
7. 将 pending/orphan 清理流程迁入具名 `<lifecycle>_cleanup.go`，并使用 bootstrap 注入的共享 Clock 与同一个 image.Policy。
8. 将当前 `ArticleImagePolicy`、`ArticleImagePolicyOptions`、默认值和 policy 校验迁入 `image`；新 Image Blob、Service、cleanup runner 与 REST 候选构造只使用 `image.Policy`。旧根 policy 只为旧 `content.Module` 运行路径暂留，阶段 6 连同该路径删除；不得以 type alias 连接新旧 policy。
9. 在 `image/<business>_validation.go` 定义 package-level、无授权、无 I/O 的 `ParseReferenceKeys`：它将 article 提取的原始 `[]string` key 转换并校验为 image 私有且对 application 不透明的 `ReferenceKeys`。Create 在文章授权前调用它；Patch 在第一次读取、版本校验和 Patch 重放的 transaction 回调返回前调用它，以保持现有错误和事务边界。package-level 形式允许 application 与只读对账工具复用同一校验，不要求构造 Service。
10. 定义 `PrepareReferences`：它接收由 application 提供的 `AuthorID` 与非空 `ReferenceKeys`，执行文件可用性预检并返回只由 image 读取的 `PreparedReferences`。`ActionCommitArticleImages` 授权和 CurrentAuthor 获取仍由 `application.ArticleImages` 按第 2.4 节执行，image 不重复授权。无新引用时不调用本方法，`PreparedReferences` 的零值表示合法的空引用集合，允许 Patch 移除全部旧引用。
11. 将事务内引用替换迁入具名 `<business>_repository.go`：它只接收 `articleID`、`PreparedReferences`、`*gorm.DB` 与本步骤新读取的 `now`，不得导入 article package 或开启 transaction。`PreparedReferences` 不是授权或状态证明；方法必须在事务内重新读取旧引用，按当前稳定 ID 顺序锁定旧/新图片，对新引用重新验证 key、owner、状态和过期时间，再替换关系；对移除引用按锁定顺序重新统计引用，并将最后引用标记 orphan。空引用集合不要求图片确认授权或 CurrentAuthor。
12. 保持以下上传时序：

    ```text
    验证/重编码
        -> 暂存 Blob
        -> 写 pending 元数据
        -> 事务外提交最终 Blob
        -> 成功或补偿
    ```

13. 保持以下清理时序：

    ```text
    候选查询
        -> 单项加锁复核
        -> 删除最终 Blob
        -> 删除元数据
    ```

### 验收

```text
go test -run '^$' ./...
go test -tags=integration -run '^$' ./...
go test -tags=e2e -run '^$' ./...
go test ./internal/modules/content/image -count=1
task integration QA_CONFIG=<DISPOSABLE_QA_CONFIG> PACKAGES=./internal/modules/content/image
go test ./internal/transport/rest/content -count=1
go test -count=1 ./internal/architecture
task qa:contract
```

迁移并执行现有图片 Repository、上传补偿、清理复核和 Blob/数据库失败测试；不新增故障组合全排列或新的 E2E 叙事。

### 通过标准

```text
pending / committed / orphaned 状态转换不变
暂存、最终提交、补偿和清理重试不变
nil storage 的构造与稳定失败语义不变
图片 URL、Content-Type、ETag、304、Cache-Control 与下载响应不变
image 不导入 article 或 application
事务内重验证、引用计数与最后引用 orphan 语义由日常真实 PostgreSQL 测试证明；稳定锁顺序作为实现规则保持，不要求锁顺序全排列或高并发测试门禁
旧图片源码仅在重构分支内保留到阶段 6，以维持尚未切换的旧调用方编译；不得让 REST 或 worker 同时调用新旧实现。
```

## 7. 阶段 4：迁移 Article 与文章图片候选协作

### 目标

将文章自身操作留在 `content/article/`，并把文章正文图片引用的复杂写入集中到唯一的 `content/application/article_images.go`。

```text
internal/modules/content/article/
├── api.go
├── article_service.go
├── article_query_service.go / article_query_repository.go（仅在读取复杂时创建）
├── article_repository.go
├── article_record.go
├── article_mapper.go
├── article_validation.go
├── article_*.go                 # 领域聚合和值对象；按独立业务边界命名
├── errors.go
└── *_test.go

internal/modules/content/application/
├── article_images.go
└── article_images_test.go
```

### 工作项：Article feature

1. 将文章 Command、Query、Result、`PatchPreview` 与 `ManagedImageReferences` 迁入 `article/api.go`；REST 所需窄接口保留在 REST 消费方 package。`ManagedImageReferences(markdown string) ([]string, error)` 只验证 Markdown 中受控图片 URL 的位置与结构、去重并保留首次出现顺序；它返回未解析的 key candidate，不导入 image package，也不复制 image 的存储键校验。
2. 将文章查询、软删除、发布/归档规则、文章动作常量及 Create/Patch 的事务内重放迁入 `article/article_service.go` 与 `article/errors.go`。`CreateInTx` 执行创建，`PreviewPatchInTx` 在第一次 transaction 中读取、校验版本、重放并返回含原始图片 key candidate 的 `PatchPreview`，`PatchInTx` 在最终写事务中重新读取并完整重放。三者只执行文章规则、读取和必要保存，不自行开启 transaction 或重复授权；动作字符串值保持不变。
3. 将文章详情、列表、Tag 批量补齐、分页、排序与投影按读取职责迁入 `article_query_service.go` 与 `article_query_repository.go`；简单读取可留在 `article_service.go` 或 `article_repository.go`。
4. 将 Article、TagArticle 的 GORM persistence Record 移入 `article/article_record.go`。
5. 将文章读取、保存、乐观锁、TagArticle 替换、PostgreSQL sequence、错误映射移入 `article/article_repository.go`。
6. 将文章字段、状态和版本输入校验迁入 `article/article_validation.go` 与相应 `<business>_value.go` 文件。
7. Article Service 不调用 image Service；文章图片协作全部交给 `application.ArticleImages`。

### 工作项：`application/article_images.go`

1. `ArticleImages` 是目标 REST Create/Patch 的唯一入口。article 与 image 均不导入对方 package，image 不解析 Markdown；`ActionCommitArticleImages` 常量与 application 自己的稳定错误归此具名动作所有。
2. Create 严格保持：文章字段与创建状态解析、`ManagedImageReferences` 提取 -> `image.ParseReferenceKeys` 完整校验 key -> `ActionCreateArticle`、可选 `ActionPublishArticle` -> 如有受控图片，application 执行 `ActionCommitArticleImages`、取得 CurrentAuthor，`image.PrepareReferences` 执行 Blob 预检 -> 一次 transaction：分配文章 ID、创建文章；仅有受控图片时重新锁定并验证图片、替换引用与状态。
3. Patch 严格保持：

   ```text
   解析 ID、版本、状态并选择文章动作授权
       -> 第一次 transaction：读取文章、校验版本、应用 Patch、提取原始图片引用
            -> application 在同一回调返回前调用 image.ParseReferenceKeys
       -> 如有受控图片，application 执行图片确认授权、取得 CurrentAuthor，image 执行文件可用性预检
       -> 第二次 transaction：重新读取、重放 Patch、保存文章、重新锁定并验证图片、替换引用与状态
   ```

4. Create 的写入阶段和 Patch 的第二次事务均让 article 变更与图片引用状态转换使用同一个 transaction handle。保持当前 Clock 调用边界：article 创建或每次 Patch 重放自行读取 Clock；仅在现有路径执行图片引用转换时，application 才在 article 保存后另读一次 UTC `now`。Create 无图片时不读取引用转换时间，Patch 即使移除全部引用仍读取该时间。不把这些读取合并为动作级单一时间值。
5. `PreparedReferences` 不携带可跨越 transaction 信任的授权或数据库状态。`image.ReplaceReferencesInTx` 必须按当前稳定锁顺序对新引用重新验证 key、owner、状态和过期时间，对移除引用重新统计引用数；空引用集合继续允许在不执行图片确认授权和 CurrentAuthor 的情况下移除全部旧引用。
6. 迁移并执行现有版本冲突优先级、事务回滚、引用状态、正常绑定/移除和单一代表性故障测试；仅覆盖复杂并发、锁顺序排列或多故障组合的测试按 2.6 节删除，不新增并发规模或故障组合全排列。
7. `DeleteArticle` 继续只执行软删除，不在本阶段新增图片 orphan。
8. 保留旧根文章源码到阶段 6 统一删除；本阶段只验证新 article/application 候选实现，不建立运行时双路由或双写。

### 工作项：发布只读对账工具

1. 实现 `cmd/content-reconcile` 与 `task reconcile:content`，命令逻辑直接复用 `article.ManagedImageReferences` 和 package-level `image.ParseReferenceKeys`，不复制 Markdown 或 storage key 规则。
2. 命令只建立 `REPEATABLE READ READ ONLY` transaction，只读取活动及软删除文章、图片元数据和引用表；Blob 只读适配暴露最终 Blob 的打开、大小与 SHA-256 校验，以及按 cutoff/limit 有界、按修改时间与名称确定性排序的过期临时文件枚举。枚举结果只含临时名称和 `ModifiedAt`，并带有是否达到 limit 的 `truncated` 标记；日常场景只要求有界样例，不要求为本次重构实现完整分页或超大目录扫描。它不得暴露或复用提交、删除、状态转换及 cleanup 删除端口。
3. 按第 10.2 节输出清理候选、发布阻断不一致和有界脱敏样例；命令没有 repair/delete/rebuild/transition 参数。
4. 新增 `task reconcile:fixture QA_CONFIG=<DISPOSABLE_QA_CONFIG>` 作为可重复的命令级 smoke。fixture 编排器复用阶段 0 的 `qaconfig` 所有权辅助，生成不可预测的 run ID、数据库/角色允许前缀和专属临时 Blob 根。创建任何 fixture 资源前，它必须通过 admin 连接验证 `current_user` 同时具有 `rolcreatedb` 与 `rolcreaterole`；缺少任一能力立即失败。每个 fixture 的只读 role 名固定为 `<base-prefix><run-id>reconcile_ro<fixture-suffix>`，仅含安全字符，并在创建前验证 `len(base-prefix)+25+len("reconcile_ro")+13 <= max_identifier_length`。它执行 migration，用写身份准备一致数据及 integration 测试场景；随后在本 fixture 创建的目标数据库内撤销 `PUBLIC` 的全部数据库、`public` schema、现有表和 sequence 权限，再创建带随机一次性密码的随机 `LOGIN` 只读角色。该角色只被授予目标数据库 `CONNECT`、`public` schema `USAGE`，以及对 `"Article"`、`article_images`、`article_image_references` 的 `SELECT`；其凭据只物化到权限为 `0600` 的临时 `READ_ONLY_CONFIG`。该角色默认事务只读，并为完全由本 fixture 控制的实例生成测试专用静默证据，再调用实际构建的 `content-reconcile` 命令。数据库、角色和 Blob 根均由同一 fixture 进程创建，成功创建后立即加入进程内 registry，不存在跨进程追加；`defer` 按 Blob 根、数据库、角色的安全顺序清理并验证零残留，归属证明失败时不得尝试删除。测试专用静默证据必须标记非生产且不得被发布流程接受。integration 测试覆盖一致数据退出 `0`、各类发布阻断不一致退出非零、引用数为零的合法过期候选单独计数、过期临时文件的有界只读枚举及 `truncated` 标记；只读身份即使显式关闭事务只读，也无法对目标库执行一个代表性 DML 和一个 `CREATE TABLE` DDL；不展开高并发、超大临时目录或多故障组合。fixture 日志和失败输出不得包含 DSN、Blob 根凭据或正文。
5. 该角色必须显式为 `LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT`，并设置 `default_transaction_read_only=on`；除目标数据库 `CONNECT`、目标 schema `USAGE` 和上述三张表的 `SELECT` 外不授予任何权限。`PUBLIC` 撤权仅作用于 fixture 自己创建的目标数据库，不改变共享实例其他数据库的策略；默认事务只读不是权限边界，实际权限由 fixture 内的 `REVOKE`/`GRANT` 保证。验收必须先以该角色显式执行 `SET default_transaction_read_only = off`，随后验证一个代表性 DML 因缺少表权限失败、一个代表性 DDL 因缺少 schema 权限失败；不展开角色属性或权限组合的穷举。

### 验收

```text
go test -run '^$' ./...
go test -tags=integration -run '^$' ./...
go test -tags=e2e -run '^$' ./...
go test ./internal/modules/content/article -count=1
go test ./internal/modules/content/application -count=1
task integration QA_CONFIG=<DISPOSABLE_QA_CONFIG> PACKAGES="./internal/modules/content/article ./internal/modules/content/application ./cmd/content-reconcile"
go test ./internal/transport/rest/content -count=1
go test -count=1 ./internal/architecture
task qa:contract
task reconcile:fixture QA_CONFIG=<DISPOSABLE_QA_CONFIG>
```

integration 门禁必须覆盖文章与图片同事务回滚、正常引用替换、最后引用 orphan，以及版本冲突先于缺失图片文件返回；不要求并发引用转换或稳定锁顺序的全排列测试。

### 通过标准

```text
Create 使用一次写事务；Patch 使用一次短读取/预检事务和一次最终写事务；Create 写入阶段与 Patch 最终写事务均在同一个 transaction handle 内完成文章、引用和图片状态更新
Create 非法图片 key 与 Patch 版本冲突的错误优先级不变
Clock 调用边界、正常引用替换、事务内重验证、引用计数与最后引用 orphan 行为不变；稳定锁顺序作为实现约束保留，不以复杂并发测试作为门禁
article 与 image 不直接导入或调用对方 package；它们只经 application 协作
```

## 8. 阶段 5：切换 REST、bootstrap 与 worker

### 目标

让 transport 只接收所需 feature/application 接口；让 bootstrap 成为唯一具体对象组装点；从 REST、worker 与 bootstrap 的运行时对象图中完全移除 `content.Module`。

### 工作项

1. 将 REST 路由依赖改为：

   ```text
   application.ArticleImages                  创建 / Patch 文章（包括发布与归档）
   article.Service                            删除、详情与列表查询
   taxonomy.Service                           分类与标签
   image.Service                              上传、取消与读取
   image.CleanupRunner                        清理 worker
   image.Policy                               REST multipart 单文件暂存限制
   config.ArticleImages.UploadRequestBytes    POST /api/v1/manage/article-images 的 HTTP 请求体上限，仅传给 httpserver
   ```

2. 保留一个薄的 generated aggregate Handler 以实现完整 `StrictServerInterface`。它继续持有阶段 1 拆出的文章查询、文章创建/Patch、文章删除、taxonomy、image 与 image policy 独立字段，但在本切换提交中一次性将各消费方接口的方法名、Command/Query/Result 类型和 DTO mapper 从旧 content 根 API 改为 article/taxonomy/image/application API；bootstrap 同时改为直接注入新实现。删除通用 Command/Query 字段、image type assertion 和全部旧签名；每个生成方法只委托一个字段，不增加适配器，也不将业务移入 Handler。
3. 保持 `captureArticleTypeImagePatch` 在 OpenAPI contract middleware 前执行。
4. bootstrap 的数据库生命周期接口提供 `GORM() *gorm.DB`；bootstrap 直接以共享 security、Clock、该数据库 handle 与 Blob storage 构造 taxonomy Repository/Service、image Repository/Blob/Service、article Repository/Service、`application.ArticleImages`、cleanup worker 与 REST handler。将已验证的 `cfg.ArticleImages()` 单独传给 `httpserver.Options`，使 HTTP 层使用 `UploadRequestBytes`。不使用运行时类型断言，也不向 REST 或 worker 传递 `content.Module` 或等价聚合对象。
5. 保持数据库、Blob、worker、HTTP server 的资源所有权和逆序关闭顺序。
6. 将 REST、worker 与 bootstrap 全部切换到新 feature/application 实现；此时旧根源码仅为待删除的不可达代码，阶段 6 统一删除。
7. 更新所有测试 fixture，使 REST 与 worker 只构造所需能力，bootstrap 测试只替换对应 feature 构造函数，不再构造完整 content 门面。

### 验收

```text
go test ./internal/transport/rest/... -count=1
go test ./internal/bootstrap ./internal/platform/httpserver -count=1
go test -count=1 ./internal/architecture
task qa:static
task qa:database QA_CONFIG=<DISPOSABLE_QA_CONFIG>
task qa:container
```

本阶段是唯一生产路由切换点。真实 PostgreSQL 与 E2E 行为门禁、容器镜像构建和 live/ready smoke 必须全部通过，不能只用 Handler fake 或 integration/e2e 的仅编译结果代替。`qa:container` 不单独证明优雅关闭；有界 SIGTERM、readiness 撤回和逆序关闭由 bootstrap 测试与 E2E 证明。

### 通过标准

```text
REST 不接收 *content.Module、*gorm.DB、Repository 或 Blob filesystem
cleanup worker 只接收 image 清理窄接口
bootstrap 是唯一构造具体 Repository、Service 和 application 的位置
REST 路径、字段、错误、PATCH 三态、图片 HTTP 行为不变
`POST /api/v1/manage/article-images` 的 HTTP 请求体上限继续使用 `UploadRequestBytes`；Handler 对唯一 multipart file part 的上限继续使用 image.Policy 的 `MaxFileBytes`；非该精确 POST 路径继续使用默认请求体上限。
新运行时从 REST 到 PostgreSQL/Blob/worker 的完整路径由现有 integration 与 E2E 验证；容器 smoke 证明镜像可构建、服务可启动并通过 live/ready
```

## 9. 阶段 6：删除旧架构并固化文档

### 工作项

1. 删除已迁移的旧根 `content` 业务文件。
2. 删除：

   ```text
   internal/modules/content/internal/domain/
   internal/modules/content/internal/application/
   internal/modules/content/internal/postgres/
   internal/modules/content/postgres/
   ```

3. 删除旧 UnitOfWork、Transaction、Repository port、ReadModel port、根 `ApplicationError`/`ErrorCode`/`ErrorKind`、未实际用于 Authorizer 的 `ActionReadArticleImage`、兼容 alias，以及仅为这些抽象服务的测试 fake；保留或重写日常权限、状态、事务和 REST 行为测试。
4. 所有 `integration`、`e2e` 测试源码迁移到新公开构造与共享安全协作者，并作为最终门禁实际执行日常行为场景。仅断言旧目录、旧类型或旧接口转发的测试，以及仅覆盖超大并发、锁顺序全排列、多故障组合、极限输入或超大资源量的测试，可按阶段 0 清单删除；保护事务回滚、正常引用替换、故障补偿、数据库方言或生命周期日常合同的测试必须保留。
5. 删除仅服务旧架构的 scanner fixture；保留新 feature/application 依赖 fixture。
6. 更新 `backend/AGENTS.md`、`docs/architecture/current-state-architecture.zh-CN.md` 与 `docs/guides/module-extension.md`，记录最终导航、依赖规则、共享协作者与真实日常验证命令；这三处分别承担项目约束、当前架构和扩展操作的权威事实。
7. 在 `docs/architecture/go-backend-architecture.md` 与 `docs/architecture/ai-friendly-backend-refactoring-plan.zh-CN.md` 文首补充替代标记并链接到 Feature-first 方案；不重写历史方案正文。
8. 实施完成后，将 Feature-first 方案标记为“已实施的架构决策”，将本实施计划标记为“已完成的历史实施记录”。后续当前架构事实以 `current-state-architecture.zh-CN.md` 为准，扩展步骤以 `module-extension.md` 为准；本计划不作为运行现状的第二权威来源。

```text
go test ./internal/architecture ./internal/modules/content/... ./internal/transport/rest/... ./internal/bootstrap -count=1
task qa:static
task qa:database QA_CONFIG=<DISPOSABLE_QA_CONFIG>
task qa:container
```

`qa:database` 必须证明一次性数据库所有权并实际运行 integration 与 E2E；不得使用隐式 `config.local.yaml`，也不得用 `go test -tags=integration|e2e -run '^$'` 的仅编译结果替代。

完成前逐项确认：

```text
不存在 content.Module 业务门面；content 根仅保留共享安全与时间协作者
不存在旧 content/internal/domain、application、postgres
不存在长期 alias、双实现或双写
不存在业务 domain、ports、workflow、contract、generic repository、Service Locator
REST 只持有所需能力，bootstrap 是唯一具体对象组合点
所有保留的 unit、integration 与 E2E 测试均已迁移并实际运行
不存在未实际用于 Authorizer 的 `ActionReadArticleImage` 或其他过渡动作常量
```

## 10. 失败、回退与数据恢复

### 10.1 代码回退

- 开发分支只回退到最近一个已验证提交；生产只部署最近一个已发布、完整验证且与当前 schema 兼容的 release artifact，不得部署部分删除旧路径的中间提交。
- 生产回退前先阻断写流量、停止应用进程及 cleanup worker，并按第 10.2 节完成只读对账。只有旧 artifact 与当前 schema、状态语义兼容且不会扩大已发现的不一致时，才允许代码回退。
- 代码回退不是数据恢复。数据库事务内写入不手工逐行反向修改；发现数据或 Blob 不一致时先保留证据，再按下表选择既有清理或独立恢复方案。
- 既有幂等清理只适用于引用数为零的过期 pending、过期且无引用的 orphaned 以及过期临时文件。committed Blob 缺失/损坏、Markdown 与引用表不一致或状态/引用不变量破坏，均不得交给清理流程伪装修复。

### 10.2 运行期只读对账

阶段 0 只固定永久只读工具 `cmd/content-reconcile` 和统一入口的规格、权限、Blob 能力、外部静默证据、输出与验收合同；阶段 4 在 article/image 解析能力迁移后实现：

```text
task reconcile:content RECONCILE_CONFIG=<READ_ONLY_CONFIG> QUIESCE_EVIDENCE=<IMMUTABLE_JSON> EVIDENCE_ROOT=<path> RELEASE=<release> PHASE=before|after [PREVIOUS_ATTEMPT=<attempt-id>]
```

`RECONCILE_CONFIG` 只能配置最小权限数据库身份和 Blob 只读能力；该数据库身份除连接所需的目标数据库 `CONNECT`、schema `USAGE` 和目标表 `SELECT` 外，不得拥有 DML、DDL 或数据库对象所有权，并且默认事务只读。命令不提供修复、删除、状态转换或关系重建参数。数据库检查显式使用 `REPEATABLE READ READ ONLY` transaction；Blob 只允许打开最终文件、校验大小和 SHA-256，并按 cutoff/limit 有界、确定性枚举临时文件名称与 `ModifiedAt`，同时在结果中声明 `truncated`，不得暴露任何写入或删除能力。命令版本来自构建信息或源码版本，不由调用方自由填写。
`RECONCILE_CONFIG` 使用单一 YAML 配置，并且只允许以下字段：

| 字段 | 用途与验证 |
| --- | --- |
| `database.dsn` | 仅含最小权限 `LOGIN` 只读身份的目标 PostgreSQL DSN；不允许环境变量覆盖 |
| `article_images.directory` | 当前本地 Blob 根；只交给只读 Blob 适配器，绝不写入证据 |
| `article_images.pending_ttl` | 计算临时文件候选 cutoff；必须与当前发布 artifact 的图片 policy 一致 |
| `reconcile.environment` | 脱敏且不可变的环境标识；必须与静默证据完全相等 |
| `reconcile.postgres_instance` | 脱敏的目标 PostgreSQL 实例标识；必须与静默证据完全相等 |
| `reconcile.blob_instance` | 脱敏的目标 Blob 实例标识；必须与静默证据完全相等 |
| `reconcile.temp_scan_limit` | 正整数；限定临时文件样例数，并驱动 `truncated` 语义 |

缺失、空值、未知字段、非正 limit 或任一标识不匹配时，命令必须在打开数据库前失败。

静默窗口由发布环境的部署编排器或受控发布 runbook 建立并留证，不由 `content-reconcile` 从只读数据库推断：先从路由摘除 API，等待在途请求结束，停止全部应用实例及 cleanup worker，再生成不可变 JSON 证据并执行 `before`；替换 artifact 后、恢复应用进程和写流量前生成新的证据并执行 `after`。回退前同样建立新的静默窗口。仓库当前没有生产部署 workflow，因此该证据生产者及 runbook 是生产接入的外部前置；缺少时不得发布或回退。

静默证据 schema 至少包含 `schemaVersion`、不可复用的 `evidenceId`、`release`、`phase`、脱敏 `environment`、`deploymentRunId`、操作者或自动化主体、预期与已摘除的路由、预期与已排空的应用实例、预期与已停止的 cleanup worker、`windowStartedAt`、`drainCompletedAt`、`windowExpiresAt` 和生成时间。命令必须在打开数据库前验证 release/phase/environment 一致、预期集合与完成集合相等、时间顺序有效且当前时间未超过 `windowExpiresAt`；数据库快照和全部 Blob 检查也必须在窗口内完成。缺失、过期、集合不完整或字段不匹配时退出非零，且不得把结果作为发布证据。命令只能验证部署系统的声明及其时效，不能宣称数据库自身证明了没有其他写入者。
上述 `reconcile.environment`、`reconcile.postgres_instance` 与 `reconcile.blob_instance` 是 `RECONCILE_CONFIG` 和 `QUIESCE_EVIDENCE` 的共同期望值；命令不接受单独的环境或实例覆盖参数。`release` 只能是单一安全路径片段（允许字母、数字、点、下划线和连字符），`phase` 只能为 `before` 或 `after`；所有生成路径必须保持在 `EVIDENCE_ROOT` 内并拒绝目录穿越、绝对路径和符号链接。

每次运行由命令生成不可预测的 attempt ID，并保存到：

```text
<EVIDENCE_ROOT>/<release>/reconciliation/<phase>/<attempt-id>/
```

attempt 先写同级临时目录，完成后原子发布；目标目录已存在时失败，永不覆盖历史结果。复跑时必须传入同一 release/phase 下已存在的 `PREVIOUS_ATTEMPT`，首轮不得传入。每个 attempt 的不可变 manifest 记录 attempt ID、前一 attempt、命令版本、release、phase、脱敏环境、静默 `evidenceId`、静默证据原始字节的 SHA-256、数据库快照开始时间、命令开始/结束时间、真实退出码、各检查总数、清理候选总数或样例数（由 `truncated` 区分）、临时文件候选是否被 `truncated`、不一致数量、有界脱敏样例及每个结果文件的 SHA-256；静默证据原始字节一并复制到 attempt 目录。`manifest.json` 不计算自身 SHA-256，结果文件按最终字节计算；可选 `latest` 只能是指向 attempt ID 的可重建索引，不得作为唯一证据。

只有所有“发布阻断不一致”为零时退出码才为 `0`。满足前置条件的合法清理候选单独计数，不伪装成不一致、不由对账命令处理，也不阻断发布或回退；临时文件候选达到 limit 时只报告有界样例和 `truncated=true`，候选数量字段此时表示样例数，不声称已完成超大目录的完整盘点。发布恢复正常服务后由既有 cleanup runner 按日常策略处理，不要求为了候选清理而复跑对账。若操作者因独立恢复或清理动作主动复跑，必须创建新 attempt 并关联前一 attempt。

判定顺序固定为：先检查 Markdown/关系集合、状态和引用数等发布不变量，再分类清理候选，最后检查候选所需的 Blob 条件。任何不变量失败都属于发布阻断，不得因为行已过期而降级成清理候选。

| 对账对象 | 发布成功条件 | 合法候选 |
| --- | --- | --- |
| 正文图片引用 | 同时包含活动和软删除文章；Markdown 受控 storage key 映射到图片 ID 后，与 `article_image_references` 集合完全一致 | 无 |
| pending 图片 | 引用数为零；未过期行的最终 Blob 可读，大小与 SHA-256 和元数据一致 | 仅引用数为零的已过期行；无论最终 Blob 是否仍存在，均只报告为既有清理候选 |
| committed 图片 | 引用数至少为一；最终 Blob 可读，大小与 SHA-256 和元数据一致 | 无 |
| orphaned 图片 | 引用数为零；未过期行的最终 Blob 可读，大小与 SHA-256 和元数据一致 | 已过期且仍无引用的行只报告为既有清理候选 |
| 临时文件 | 未超过 pending TTL 的临时文件不参与失败判定 | 超过 pending TTL 的临时文件只报告为既有清理候选 |

| 结果类别 | 允许动作 |
| --- | --- |
| 满足上表前置条件的过期 pending/orphaned 或临时文件候选 | 不阻断发布或回退；恢复正常服务后仅由既有 cleanup runner 按日常策略处理。对账命令不处理候选，也不强制为候选清理复跑；若独立运维动作要求再次确认，则创建新 attempt 并关联前一 attempt |
| committed Blob 缺失、大小或摘要不一致 | 停止发布/回退；从可信备份恢复或创建独立数据恢复方案，不改变 committed 状态掩盖故障 |
| Markdown 与引用表不一致 | 停止发布/回退；创建独立恢复方案，不由对账工具自动重建或批量删除关系 |
| pending/committed/orphaned 与引用数不符合上表 | 停止发布/回退；保留行、Blob 和日志证据，创建独立业务恢复方案 |
| schema、约束、状态含义或删除语义需要变化 | 停止本计划，创建独立业务/数据库迁移方案 |

不得直接伪造 committed 状态、批量删除关系表，或绕过清理流程删除 Blob。发现发布阻断不一致时不得用代码回退宣称数据已恢复；代码回退和数据恢复必须分别决策、执行和留证。

## 11. 交付证据

每个阶段保留：

```text
迁移的文件清单与测试处置清单
删除的旧文件、删除理由和替代行为证据
执行的验证命令、真实退出码与完整脱敏输出
命令的环境前置、脱敏执行身份与日志位置
阶段 5/6 的 qa:static、qa:database、qa:container 结果
发布前后只读对账的 attempt manifest、命令版本、静默证据及其 SHA-256、结果路径与汇总
发生的已知行为差异；没有差异时明确声明验证范围
```

证据与对应 release artifact 一起保留；若项目尚无更长的统一保留策略，至少保留到下一个 release 完成验证且当前 release 退出回退窗口。任何证据不得包含数据库 DSN、Blob 根路径凭据、作者标识或正文内容；有界样例只记录脱敏 ID、状态和不一致类别。

最终交付应可回答：

```text
改文章该从哪里开始？       content/article/
改正文图片引用该从哪里开始？ content/application/article_images.go
改图片上传或清理在哪里？     content/image/
改分类与标签在哪里？         content/taxonomy/
谁构造对象？                 internal/bootstrap/
如何执行发布对账？         外部静默证据 + cmd/content-reconcile + task reconcile:content
哪些规则阻止不当依赖？       internal/architecture/
```
