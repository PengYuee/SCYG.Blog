# 业务模块扩展指南

新增模块只应发生在已有业务需求出现后，不提前创建空目录。模块组织遵循一个可机械检查的原则：**目录 = 技术层，前缀 = 业务主体，后缀 = 职责**。

## 固定结构

1. 在 `internal/modules/<module>/module.go` 定义具体 façade 与手工构造函数；禁止通用服务接口、DI 框架和服务定位器。
2. 在 `api.go` 定义协议中立的公共契约；不得出现 Gin、HTTP 状态、OpenAPI、GORM、Proto 或 WebSocket 类型。
3. `internal/domain` 只拥有聚合、实体、值对象、不变量和领域错误。
4. `internal/application` 只拥有用例及其消费的 repository/read model/UoW/Clock 等窄端口。接口由消费者拥有。
5. `internal/postgres` 私有实现 application 端口，显式表名/列名，使用 SQL migration，禁止 `AutoMigrate` 与跨模块表访问。
6. 每个 transport 在自身目录声明最小消费接口，将自身 DTO、状态、Header 和错误映射到模块根类型。

模块 Go package 只允许五个固定位置：模块根、`internal/domain`、`internal/application`、`internal/postgres`、`postgres`。这些目录表达技术层；所有位置下都禁止任何 Go 子 package，因此也禁止实体 Go 子包；根级 `article/`、`internal/domain/article/`、`internal/application/article/`、`internal/postgres/article/` 均被拒绝。技术性子目录也需要独立架构决策，不得绕过当前扁平层契约；业务主体必须体现在文件名前缀中。

## 文件命名契约

### 强制机械底线

生产文件的 stem 不能为空，必须使用小写 snake_case。名称只能由小写字母 token 和单个下划线组成，不得连续使用下划线，也不得在开头或结尾使用下划线。

任何文件名 token 都不得使用 generic token：`common`、`shared`、`utils`、`utility`、`helpers`、`models`、`usecases`、`results`。这些词不能充当业务主体、职责或补充说明。`api.go`、`module.go` 是模块根专属 anchors，不得放入 `internal/domain`、`internal/application`、`internal/postgres` 或 `postgres`。

PostgreSQL 行结构统一称为“数据库数据模型”，文件必须使用 `<subject>_model.go`。禁止 `*_record.go`，也禁止 `models.go` 等泛名。

模块 Go package 仍只允许五个固定位置：模块根、`internal/domain`、`internal/application`、`internal/postgres`、`postgres`。所有位置都禁止任何 Go 子 package，因此也禁止实体 Go 子包。技术性子目录也需要独立架构决策，不得绕过当前扁平层契约。

### 推荐职责命名

“层级用目录、主体用前缀、职责用名称表达”是推荐原则。优先使用 `<subject>_<role>.go`，职责需要进一步说明时可使用 `<subject>_<role>_<subrole>.go`。例如 `article_policy.go`、`article_query_usecase.go`、`article_result_mapper.go` 都能让读者直接看出业务主体与职责。

这些格式是可读性示例，不是职责白名单。Scanner 不枚举职责后缀，也不判断某个职责只能出现在哪一层；只要名称满足强制机械底线并准确表达内容，合理的新职责无需修改 Scanner。目录仍决定技术层，代码依赖与声明继续由其他架构规则约束。
## 完整示例树

```text
internal/modules/content/
├── api.go
├── module.go
├── article_command.go
├── article_command_usecase.go
├── article_query.go
├── article_query_usecase.go
├── article_result.go
├── article_result_mapper.go
├── authorization.go
├── application_error.go
├── internal/
│   ├── domain/
│   │   ├── article.go
│   │   ├── article_validation.go
│   │   ├── article_reconstitute.go
│   │   ├── article_type.go
│   │   ├── tag.go
│   │   ├── taxonomy_rule.go
│   │   ├── clock.go
│   │   └── status.go
│   ├── application/
│   │   ├── article_repository_port.go
│   │   ├── article_read_model_port.go
│   │   ├── article_view.go
│   │   └── transaction_port.go
│   └── postgres/
│       ├── article_model.go
│       ├── article_mapper.go
│       ├── article_repository.go
│       ├── article_read_model.go
│       ├── unit_of_work.go
│       └── error_translator.go
└── postgres/
    └── postgres.go
```

## 测试命名

测试跟随行为和职责，而不是机械跟随文件。测试使用业务主体与可观察行为命名，例如 `article_creation_validation_test.go`、`article_mutation_atomicity_test.go`、`article_repository_integration_test.go`。不要求与生产文件一对一；同一叙事可以覆盖多个协作者，但测试 stem 的任何职责 token 都不得使用 `helpers`、`utils` 或 `common`，因此 `helpers_test.go`、`utils_test.go`、`integration_helpers_test.go` 均被禁止。测试函数继续采用 Given/When/Then 结构，并由名称表达条件和可观察结果。

## 实施顺序

先以 Given/When/Then 单元测试锁定领域行为，再实现 application；随后用真实 PostgreSQL 验证 adapter，用 transport 测试验证映射，最后在 bootstrap 显式手工注入。跨模块调用只经过对方 façade；不得导入对方 `internal/**`。

每个变更必须补充架构失败夹具、单元测试、真实 adapter 集成测试和至少一个用户可观察 E2E 叙事。手写 Go 文件纯 LOC 不超过 250，`cmd/api/main.go` 不超过 50；导出标识符、方法签名、字段和关键逻辑使用中文注释。禁止 mutable global、通用 utility 包、反射 generic repository、协议万能 envelope 和超过三个无关参数的函数。

新模块应更新 bootstrap 的正向构造与反向清理、readiness 条件、OpenAPI（若暴露 REST）、migration、scope/review 门禁及 README。构造失败和关闭必须有 timeout，并证明已创建资源按相反顺序恰好清理一次。
