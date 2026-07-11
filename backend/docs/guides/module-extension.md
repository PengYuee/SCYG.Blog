# 业务模块扩展指南

新增模块只应发生在已有业务需求出现后，不提前创建空目录。

## 固定结构

1. 在 `internal/modules/<module>/module.go` 定义具体 façade 与手工构造函数；禁止通用服务接口、DI 框架和服务定位器。
2. 在 `api.go` 定义协议中立的命令、查询、结果、稳定语义错误和授权动作；不得出现 Gin、HTTP 状态、OpenAPI、GORM、Proto 或 WebSocket 类型。
3. `internal/domain` 只拥有聚合、实体、值对象、不变量和领域错误。
4. `internal/application` 只拥有用例及其消费的 repository/read model/UoW/Clock 等窄端口。接口由消费者拥有。
5. `internal/postgres` 私有实现 application 端口，显式表名/列名，使用 SQL migration，禁止 `AutoMigrate` 与跨模块表访问。
6. 每个 transport 在自身目录声明最小消费接口，将自身 DTO、状态、Header 和错误映射到模块根类型。

## 实施顺序

先以 Given/When/Then 单元测试锁定领域行为，再实现 application；随后用真实 PostgreSQL 验证 adapter，用 transport 测试验证映射，最后在 bootstrap 显式手工注入。跨模块调用只经过对方 façade；不得导入对方 `internal/**`。

每个变更必须补充架构失败夹具、单元测试、真实 adapter 集成测试和至少一个用户可观察 E2E 叙事。手写 Go 文件纯 LOC 不超过 250，`cmd/api/main.go` 不超过 50；导出标识符、方法签名、字段和关键逻辑使用中文注释。禁止 mutable global、通用 utility 包、反射 generic repository、协议万能 envelope 和超过三个无关参数的函数。

新模块应更新 bootstrap 的正向构造与反向清理、readiness 条件、OpenAPI（若暴露 REST）、migration、scope/review 门禁及 README。构造失败和关闭必须有 timeout，并证明已创建资源按相反顺序恰好清理一次。
