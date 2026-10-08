# SCYG Agent 架构

## 范围与当前状态

本文依据当前工作区的组合根、传输层和运行配置描述服务，不代表已通过真实模型、数据库或浏览器端到端验收。Agent 的内部 gRPC、health HTTP 与 Worker 在同一 Python 进程运行；Blog 是浏览器业务与 JWT 认证的唯一入口，通过内部网络调用 Agent，不签发 Run JWT，也不向 Agent 传递 service JWT。

整体服务关系见[项目架构](../../architecture.md)。本页维护 Agent 内部职责；HTTP 与游标合同维护在[流式说明](streaming.md)，配置和启动维护在[服务入口](README.md)与[开发指南](development.md)。

## 组件与数据归属

| 组件 | 职责 | 源码入口 |
| --- | --- | --- |
| 组合与生命周期 | 构造数据库、checkpoint、Redis、运行时、gRPC、Worker、HTTP；负责 readiness 和有界逆序关闭 | [composition.py](../../../agent/src/scyg_agent/composition.py)、[lifecycle.py](../../../agent/src/scyg_agent/lifecycle.py) |
| gRPC 控制面 | 五个 owner-authorized RPC：CreateRun、GetRun、StreamRunEvents、ResumeRun、CancelRun；四个 unary RPC 返回统一 Run 快照 | [transport/grpc](../../../agent/src/scyg_agent/transport/grpc/)；正式合同为根 [contracts](../../../contracts/) |
| HTTP 面 | 仅 `/health/live` 与 `/health/ready`，无 Run 业务端点 | [transport/http](../../../agent/src/scyg_agent/transport/http/) |
| 应用与领域 | 所有权、幂等入口、Run 状态、revision、lease 和命令规则 | [application](../../../agent/src/scyg_agent/application/)、[domain](../../../agent/src/scyg_agent/domain/) |
| Worker | 认领、续租、执行、终态提交、恢复与 drain | [worker](../../../agent/src/scyg_agent/worker/) |
| Recipe 执行 | 四类结构化能力的服务器端目录与 LangChain AgentRunner | [agents/recipes.py](../../../agent/src/scyg_agent/agents/recipes.py)、[agents/production.py](../../../agent/src/scyg_agent/agents/production.py) |
| Blog 只读工具 | 封闭工具目录、Recipe allowlist 和逐次执行授权 | [agents/tool_catalog.py](../../../agent/src/scyg_agent/agents/tool_catalog.py)、[agents/tool_gateway.py](../../../agent/src/scyg_agent/agents/tool_gateway.py) |
| 持久化适配器 | PostgreSQL Run、事件、命令、交互、审计与终态事务 | [adapters/database](../../../agent/src/scyg_agent/adapters/database/) |
| Redis 适配器 | 瞬时流及其游标、保留和过期行为 | [adapters/redis](../../../agent/src/scyg_agent/adapters/redis/) |
| Checkpoint | LangGraph 执行恢复存储，不替代业务状态或 SSE 流 | [adapters/langgraph](../../../agent/src/scyg_agent/adapters/langgraph/) |
| 初始化 | Agent truth migrations 与 checkpoint setup | [deployment.py](../../../agent/src/scyg_agent/deployment.py)、[migrations](../../../agent/migrations/) |

Agent 不直接访问 Blog 数据库，也不持有 Blog DSN。Agent truth 与 checkpoint 使用同一应用数据库、应用账号及 DSN；`public`、`langgraph` 是数据组织边界，不是当前的账号安全隔离边界。Redis 的流过期不能被解释为 Run 状态或业务结果被删除。

## 当前执行链路

`AgentRunnerResource.start` 仅构造 `LangChainAgentRunner`，Worker 仅通过 Recipe AgentRunner 执行生产任务；旧 RuntimeRouter、Registry、producer 和 `runtimes/` 执行入口已删除。四类 capability 为搜索、写作、润色、聊天；客户端只选择 capability，服务器选择并持久冻结 Recipe/version、输入摘要与模型配置。图由 `create_agent` 配合结构化输出 Schema 和共享 checkpoint 构造。

生产组合根构造 Redis stream store 并注入 Worker；公开事件订阅由 gRPC 应用门面读取 PostgreSQL 持久事件，编码完整 SSE 帧后由 Blog 透明转发。Redis 不再决定浏览器 SSE cursor，详见[流式合同](streaming.md)。

默认 Worker 总容量为 `worker_concurrency = 4`，以[配置模板](../../../agent/agent.toml.example)为准。失去 lease 的 Worker 不得提交终态；正常关闭应使用进程的有界 drain，不能把杀进程作为常规停机。

## Blog 只读工具

Search Recipe 注册 `search_articles` 与 `get_article`；写作、润色和聊天 Recipe 不注册 Blog 工具。工具复用组合根拥有的 `BlogGrpcClient`，通过 `BlogContentService` 携带可信 `user_id` 读取管理端可见的草稿、已发布和归档文章，排除已删除文章，不接管客户端生命周期。四类 capability 均不注册 Blog 写 Tool。

Runner 从持久 Run 的 `owner_user_id`、Run ID 和已解析 Recipe 构造 `AgentRequestContext`，通过 LangGraph runtime context 注入。middleware 在 SDK 执行工具前重新校验上下文类型、Recipe/version/capability、allowlist、thread ID、实际调用 ID 和注册工具实例；恢复 checkpoint 中的未知或写工具也必须先拒绝，不发 Blog RPC。模型参数、graph state 和 checkpoint 不提供授权依据。

模型只看到查询、管理筛选、页码/页大小或文章 ID 参数，额外控制字段会被拒绝。查询最多 500 字符，页大小限 1–100，文章 ID 为正 `int64`；可信用户 ID 来自 runtime context，不来自模型参数。文章读取核对响应 ID。

授权拒绝映射为 `FORBIDDEN`；Blog 故障映射为 `DEPENDENCY` 并保留已封闭的 retryable 属性，不透传原始 RPC 详情，也不转旧 Runtime。只读请求使用 Run ID 关联执行，合法 SDK 调用 ID 直接复用，其余调用 ID 由 Run ID 和 SDK ID 确定性转换为领域 ToolCallId；每次 RPC 生成新的 RequestId。

## 修改与验证入口

传输层只处理协议与转换；浏览器认证留在 Blog，Agent application 按 `user_id` 校验所有权。Create/Resume/Cancel 共享 `(owner, UUID v4 key)` 的成功操作记录，成功后保留 24 小时，跨 RPC 命中返回原 Run 的当前快照，失败不占 key。状态合法性在 domain，数据库及网络实现留在 adapters，由组合根注入。新增功能按实际责任落位，不预建插件、多租户或兼容抽象。

测试入口为 [agent/tests](../../../agent/tests/)。静态检查与动态拓扑检查的命令和前置条件见[开发指南](development.md)。静态测试通过不证明外部依赖、模型调用或浏览器链路可用。
