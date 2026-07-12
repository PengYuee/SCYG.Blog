# 业务模块扩展指南

新增模块只应发生在已有业务需求出现后，不提前创建空目录。模块组织遵循一个可机械检查的原则：**目录 = 技术层，前缀 = 业务主体，后缀 = 职责**。

## 固定结构

1. 在 `internal/modules/<module>/module.go` 定义具体 façade 与手工构造函数；禁止通用服务接口、DI 框架和服务定位器。
2. 在 `api.go` 定义协议中立的公共契约；不得出现 Gin、HTTP 状态、OpenAPI、GORM、Proto 或 WebSocket 类型。
3. `internal/domain` 只拥有聚合、实体、值对象、不变量和领域错误。
4. `internal/application` 只拥有用例及其消费的 repository/read model/UoW/Clock 等窄端口。接口由消费者拥有。
5. `internal/postgres` 私有实现 application 端口，显式表名/列名，使用 SQL migration，禁止 `AutoMigrate` 与跨模块表访问。
6. 每个 transport 在自身目录声明最小消费接口，将自身 DTO、状态、Header 和错误映射到模块根类型。

`domain`、`application`、`postgres` 是技术层目录。`domain` 与 `application` 下禁止任何 Go 子 package，因此也禁止实体 Go 子包，例如 `internal/domain/article/`、`internal/domain/tag/` 或 `internal/application/article/`；实体或业务主题必须体现在文件名前缀中。技术性子目录也需要独立架构决策，不得绕过当前扁平层契约。新增 adapter 应位于模块既定技术层，而不是嵌套到 domain/application。

## 文件命名契约

生产文件默认使用 `<subject>_<role>.go`；同一职责需要进一步区分时使用 `<subject>_<role>_<subrole>.go`。例如 `article_command.go`、`article_command_usecase.go`、`article_result_mapper.go`。前缀回答“处理哪个业务主体”，后缀回答“承担什么职责”。共享代码优先使用准确语义名，不得退化为 `helpers.go`、`utils.go` 或 `common.go`。

固定职责后缀至少包括：`command`、`query`、`result`、`usecase`、`port`、`view`、`model`、`repository`、`read_model`、`mapper`、`validation`、`error`。

模块根锚点 `api.go`、`module.go` 是固定例外。以下准确单词语义名也属于有限例外：`clock.go`、`status.go`、`handler.go`、`problem.go`、`etag.go`；现有聚合实体文件、`authorization.go`、`application_error.go`、`unit_of_work.go`、`error_translator.go` 与组合入口 `postgres.go` 仅在其名称准确覆盖单一职责时允许。此清单不是创建新泛名的入口。

| 固定 role | 用途 | 示例 |
| --- | --- | --- |
| `command` / `query` / `result` | 模块输入与输出契约 | `article_command.go`、`tag_result.go` |
| `usecase` | 命令或查询用例实现；通常作为复杂职责的 subrole | `article_query_usecase.go` |
| `port` / `view` | application 消费的窄端口与只读视图 | `article_repository_port.go`、`taxonomy_view.go` |
| `model` | **数据库数据模型**，即持久化行结构 | `article_model.go` |
| `repository` / `read_model` | 写仓储与读取投影 | `tag_repository.go`、`article_read_model.go` |
| `mapper` | 边界、持久化或结果映射 | `article_type_result_mapper.go` |
| `validation` / `error` | 校验规则与稳定错误 | `article_validation.go`、`application_error.go` |

允许的补充精确职责包括 `reconstitute`、`rule`、`handler`、`etag`、`pagination`、`response`、`translator`、`sort`、`parser`、`validator` 和 `value`，例如 `article_type_reconstitute.go`、`taxonomy_rule.go`、`pagination_mapper.go`、`response_mapper.go`。它们仍须带业务主体或准确共享语义，不能变成无限豁免。

PostgreSQL 行结构统一称为“数据库数据模型”，文件只能使用 `<subject>_model.go`。禁止 `*_record.go` 和复数泛名 `models.go`。模块根或任意技术层同时禁止 `usecases.go`、`results.go`、`helpers.go`、`utils.go`、`common.go`。

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

测试跟随行为和职责，而不是机械跟随文件。测试文件使用可识别的业务主体与行为，例如 `article_creation_validation_test.go`、`article_mutation_atomicity_test.go`、`article_repository_integration_test.go`。不要求与生产文件一对一；同一叙事可以覆盖多个协作者，但测试 stem 的任何职责 token 都不得使用 `helpers`、`utils` 或 `common`，因此 `helpers_test.go`、`utils_test.go`、`integration_helpers_test.go` 均被禁止。测试函数继续采用 Given/When/Then 结构，并由名称表达条件和可观察结果。

## 实施顺序

先以 Given/When/Then 单元测试锁定领域行为，再实现 application；随后用真实 PostgreSQL 验证 adapter，用 transport 测试验证映射，最后在 bootstrap 显式手工注入。跨模块调用只经过对方 façade；不得导入对方 `internal/**`。

每个变更必须补充架构失败夹具、单元测试、真实 adapter 集成测试和至少一个用户可观察 E2E 叙事。手写 Go 文件纯 LOC 不超过 250，`cmd/api/main.go` 不超过 50；导出标识符、方法签名、字段和关键逻辑使用中文注释。禁止 mutable global、通用 utility 包、反射 generic repository、协议万能 envelope 和超过三个无关参数的函数。

新模块应更新 bootstrap 的正向构造与反向清理、readiness 条件、OpenAPI（若暴露 REST）、migration、scope/review 门禁及 README。构造失败和关闭必须有 timeout，并证明已创建资源按相反顺序恰好清理一次。
