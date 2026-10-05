# SCYG Agent 架构

## 范围与当前状态

本文依据当前工作区的组合根、传输层和运行配置描述服务，不代表已通过真实模型、数据库或浏览器端到端验收。Agent 的 gRPC、HTTP/SSE 与 Worker 在同一 Python 进程运行；Blog 到 Agent 的公开业务集成及前端 AI 页面尚未接通。Blog 自身已有用户名密码登录，不能把“Agent 集成未完成”写成“博客登录未实现”。

整体服务关系见[项目架构](../../architecture.md)。本页维护 Agent 内部职责；HTTP 与游标合同维护在[流式说明](streaming.md)，配置和启动维护在[服务入口](README.md)与[开发指南](development.md)。

## 组件与数据归属

| 组件 | 职责 | 源码入口 |
| --- | --- | --- |
| 组合与生命周期 | 构造数据库、checkpoint、Redis、运行时、gRPC、Worker、HTTP；负责 readiness 和有界逆序关闭 | [composition.py](../../../agent/src/scyg_agent/composition.py)、[lifecycle.py](../../../agent/src/scyg_agent/lifecycle.py) |
| gRPC 控制面 | 认证、请求转换与 Run 创建/查询，用例委托应用门面 | [transport/grpc](../../../agent/src/scyg_agent/transport/grpc/)；正式合同为根 [contracts](../../../contracts/) |
| HTTP 面 | Run 快照、SSE、输入、命令和取消，不提供 HTTP 创建 Run | [transport/http](../../../agent/src/scyg_agent/transport/http/) |
| 应用与领域 | 所有权、幂等入口、Run 状态、revision、lease 和命令规则 | [application](../../../agent/src/scyg_agent/application/)、[domain](../../../agent/src/scyg_agent/domain/) |
| Worker | 认领、续租、执行、终态提交、恢复与 drain | [worker](../../../agent/src/scyg_agent/worker/) |
| Recipe 执行 | 四类结构化能力的服务器端目录与 LangChain AgentRunner | [agents/recipes.py](../../../agent/src/scyg_agent/agents/recipes.py)、[agents/production.py](../../../agent/src/scyg_agent/agents/production.py) |
| SIMPLE/DEEP 运行时 | 当前仍装配的任务注册、画像和 RuntimeRouter | [composition_runtime.py](../../../agent/src/scyg_agent/composition_runtime.py)、[runtimes](../../../agent/src/scyg_agent/runtimes/) |
| 持久化适配器 | PostgreSQL Run、事件、命令、交互、审计与终态事务 | [adapters/database](../../../agent/src/scyg_agent/adapters/database/) |
| Redis 适配器 | 瞬时流及其游标、保留和过期行为 | [adapters/redis](../../../agent/src/scyg_agent/adapters/redis/) |
| Checkpoint | LangGraph 执行恢复存储，不替代业务状态或 SSE 流 | [adapters/langgraph](../../../agent/src/scyg_agent/adapters/langgraph/) |
| 初始化 | Agent truth migrations 与 checkpoint setup | [deployment.py](../../../agent/src/scyg_agent/deployment.py)、[migrations](../../../agent/migrations/) |

Agent 不直接访问 Blog 数据库，也不持有 Blog DSN。Agent truth 与 checkpoint 使用同一应用数据库、应用账号及 DSN；`public`、`langgraph` 是数据组织边界，不是当前的账号安全隔离边界。Redis 的流过期不能被解释为 Run 状态或业务结果被删除。

## 当前执行链路

`RuntimeFacadeResource.start` 当前既构造 SIMPLE/DEEP Router，也构造 `LangChainAgentRunner`。Recipe 的四类能力为搜索、写作、润色、聊天；图由 `create_agent` 配合结构化输出 Schema 和共享 checkpoint 构造。Worker 获得 AgentRunner、Router 与 Redis stream store。不能把尚未完成的重构计划写成“旧运行时已经完全删除”。

生产组合根总是构造 Redis 资源，并把 stream store 注入 HTTP、gRPC 和 Worker。HTTP 支持未注入 stream store 时的 PostgreSQL 事件分支，但它不是当前生产组合的默认流式路径。两种事件流使用不同游标，详见[流式合同](streaming.md)。

默认 Worker 容量仍为 SIMPLE 4、DEEP 1，实际值以[配置](../../../agent/src/scyg_agent/config.py)为准。失去 lease 的 Worker 不得提交终态；正常关闭应使用进程的有界 drain，不能把杀进程作为常规停机。

## 修改与验证入口

传输层只处理协议、认证和转换；跨请求用例在 application，状态合法性在 domain，数据库及网络实现留在 adapters，由组合根注入。新增功能按实际责任落位，不预建插件、多租户或兼容抽象。

已有测试按 domain、application、adapters、transport、worker、runtimes、deployment、production、integration 分类，入口为 [agent/tests](../../../agent/tests/)。静态检查与动态拓扑检查的命令和前置条件见[开发指南](development.md)。静态测试通过不证明外部依赖、模型调用或浏览器链路可用。
