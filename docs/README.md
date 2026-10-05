# 项目文档

本目录是 SCYG.Blog 正式文档的统一入口。读者从项目运行、服务开发或故障处理任务进入；源码、OpenAPI、Proto 与配置仍在各自职责目录。

## 阅读路径

| 任务 | 文档 |
| --- | --- |
| 了解项目与当前能力 | [根 README](../README.md) |
| 准备整套开发环境、启动与检查 | [项目开发指南](development.md) |
| 了解服务关系、通信与数据归属 | [项目架构](architecture.md) |
| 前端开发与部署 | [前端入口](services/frontend/README.md)、[架构](services/frontend/architecture.md)、[开发](services/frontend/development.md)、[运行](services/frontend/operations.md)、[设计系统](services/frontend/design.md) |
| 后端开发与扩展 | [后端入口](services/backend/README.md)、[现行架构](services/backend/architecture/current-state-architecture.zh-CN.md)、[开发](services/backend/guides/backend-development.md)、[模块扩展](services/backend/guides/module-extension.md)、[协议扩展](services/backend/guides/protocol-integration-extension.md) |
| Agent 配置、开发与运行 | [Agent 入口](services/agent/README.md)、[架构](services/agent/architecture.md)、[流式合同](services/agent/streaming.md)、[开发](services/agent/development.md)、[运行](services/agent/operations.md) |
| 编写或维护说明 | [文档编写方法](documentation-writing.md) |

## 现行、目标与历史

当前实现以源码、合同与配置为依据；正式需求和设计必须有确认依据并明确目标状态，不能视为已实现能力。后端 [旧绑定架构](services/backend/architecture/go-backend-architecture.md)是历史决策，现行约束以现行架构和指南为准；[Scalar 资产 ADR](services/backend/architecture/adr-010-scalar-asset-pin.md)维护资产选择依据。

项目级文档维护服务之间的关系与公共开发路径；服务文档维护各自内部职责和操作，不复制另一层正文。接口字段分别由 [OpenAPI](../backend/api/openapi.yaml)与 [Proto 合同](../contracts/proto/)维护。

文档归属、确认、转正和本地任务材料规则只在[根 AGENTS](../AGENTS.md)维护。本次整理未擅自把已有需求和计划认定为正式确认材料；原内容保留在本地任务目录，不作为本目录的权威说明。变更接口、配置、服务关系或运行方式时，由变更者同步维护对应正式文档及本导航。
