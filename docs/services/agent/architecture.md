# SCYG Agent 架构

## 范围与当前状态

本文依据组合根、传输层和运行配置描述当前实现，不替代具体环境的验收证据。Agent 内部 gRPC、health HTTP 与 Worker 在同一 Python 进程运行；Blog 是浏览器业务、作者 JWT 和独立匿名搜索凭证的唯一入口，不向 Agent 传递 service JWT。

整体服务关系见[项目架构](../../architecture.md)。本页维护 Agent 内部职责与承载映射；业务规则由 [AI 任务](../../modules/ai-assistance.md)及[公共搜索](../../modules/public-ai-search.md)维护。HTTP 与游标技术合同见[流式说明](streaming.md)，配置和启动见[服务入口](README.md)与[开发指南](development.md)。

## 组件与数据归属

| 组件 | 职责 | 源码入口 |
| --- | --- | --- |
| 组合与生命周期 | 构造数据库、checkpoint、Redis、运行时、gRPC、Worker、HTTP；负责 readiness 和有界逆序关闭 | [composition.py](../../../agent/src/scyg_agent/composition.py)、[lifecycle.py](../../../agent/src/scyg_agent/lifecycle.py) |
| gRPC 控制面 | 五个 owner＋内容范围授权 RPC：CreateRun、GetRun、StreamRunEvents、ResumeRun、CancelRun；四个 unary RPC 返回统一 Run 快照 | [transport/grpc](../../../agent/src/scyg_agent/transport/grpc/)；正式合同为根 [contracts](../../../contracts/) |
| HTTP 面 | 仅 `/health/live` 与 `/health/ready`，无 Run 业务端点 | [transport/http](../../../agent/src/scyg_agent/transport/http/) |
| 应用与领域 | 所有权、幂等入口、Run 状态、revision、lease 和命令规则 | [application](../../../agent/src/scyg_agent/application/)、[domain](../../../agent/src/scyg_agent/domain/) |
| Worker | 认领、续租、执行、终态提交、恢复与 drain | [worker](../../../agent/src/scyg_agent/worker/) |
| Recipe 执行 | 四类结构化能力的服务器端目录与 LangChain AgentRunner | [agents/recipes.py](../../../agent/src/scyg_agent/agents/recipes.py)、[agents/production.py](../../../agent/src/scyg_agent/agents/production.py) |
| Blog 只读工具 | 封闭工具目录、Recipe allowlist 和逐次执行授权 | [agents/tool_catalog.py](../../../agent/src/scyg_agent/agents/tool_catalog.py)、[agents/tool_gateway.py](../../../agent/src/scyg_agent/agents/tool_gateway.py) |
| 持久化适配器 | PostgreSQL Run、事件、命令、交互、审计与终态事务 | [adapters/database](../../../agent/src/scyg_agent/adapters/database/) |
| Redis 适配器 | Worker 瞬时流、公共日额度及幂等补偿、公共执行租约 | [adapters/redis](../../../agent/src/scyg_agent/adapters/redis/) |
| Checkpoint | LangGraph 执行恢复存储，不替代业务状态或 SSE 流 | [adapters/langgraph](../../../agent/src/scyg_agent/adapters/langgraph/) |
| 初始化 | Agent truth migrations 与 checkpoint setup | [deployment.py](../../../agent/src/scyg_agent/deployment.py)、[migrations](../../../agent/migrations/) |

Agent 不直接访问 Blog 数据库，也不持有 Blog DSN。Agent truth 与 checkpoint 使用同一应用数据库、应用账号及 DSN；`public`、`langgraph` 是数据组织边界，不是当前的账号安全隔离边界。Redis 的流过期不能被解释为 Run 状态或业务结果被删除。

## 当前执行链路

Recipe AgentRunner 是唯一生产路径，Worker 不再调用旧 RuntimeRouter、Registry 或 producer。客户端只选择搜索、写作、润色或聊天，服务端持久冻结 Recipe/version、模型档位、输入摘要和内容范围；Provider 配置由服务端运行时装配。一般图由 create_agent 与结构化输出 Schema 构造，写作的可选准备阶段由固定节点和相同 checkpoint 执行，不新增可选 Recipe 或通用工作流。

生产组合根构造 Redis stream store 并注入 Worker；公开事件订阅由 gRPC 应用门面读取 PostgreSQL 持久事件，编码完整 SSE 帧后由 Blog 透明转发。Redis 不再决定浏览器 SSE cursor，详见[流式合同](streaming.md)。

默认 Worker 总容量为 `worker_concurrency = 4`，以[配置模板](../../../agent/agent.toml.example)为准。失去 lease 的 Worker 不得提交终态；正常关闭应使用进程的有界 drain，不能把杀进程作为常规停机。

公共接纳、分范围认领、Redis 执行名额、首次执行截止时间和单次预算共同约束 public search；author 不经过公共桶。配置、补偿与故障边界集中见 [公共搜索控制](public-search.md)。

## Blog 只读工具

只读能力由组合根拥有的 BlogGrpcClient 和 Gateway 承载；Recipe allowlist、可信上下文、调用身份与执行派发围栏位于服务端，不能依赖模型工具可见性。普通图与固定写作准备节点的装配以 [production.py](../../../agent/src/scyg_agent/agents/production.py)和 [tool_gateway.py](../../../agent/src/scyg_agent/agents/tool_gateway.py)为准。

各能力开放哪些读取、候选与正文来源怎样进入生成，以及匿名用途的内容范围统一见业务模块。本文只维护执行层和跨服务 adapter 的技术落点，不把存在写 RPC 等同于已开放模型写工具。

授权拒绝映射为 `FORBIDDEN`；Blog 故障映射为 `DEPENDENCY` 并保留已封闭的 retryable 属性，不透传原始 RPC 详情，也不转旧 Runtime。只读请求使用 Run ID 关联执行，合法 SDK 调用 ID 直接复用，其余调用 ID 由 Run ID 和 SDK ID 确定性转换为领域 ToolCallId；每次 RPC 生成新的 RequestId。

## 修改与验证入口

传输层只处理协议与转换，业务身份、内容范围、成功幂等和交互消费由 application/domain 执行，网络与存储由 adapters 承载，组合根统一装配。修改这些接点时，分别核对 [AI 任务](../../modules/ai-assistance.md)、[公共搜索](../../modules/public-ai-search.md)及正式合同中的对应规则，不在架构页维护第二套幂等或状态条件。

测试入口为 [agent/tests](../../../agent/tests/)。静态检查与动态拓扑检查的命令和前置条件见[开发指南](development.md)。静态测试通过不证明外部依赖、模型调用或浏览器链路可用。
