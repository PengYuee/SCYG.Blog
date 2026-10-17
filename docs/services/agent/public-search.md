# 公共搜索运行控制

本文供 Agent 和 Blog 维护者定位配置、接纳事务、Redis 控制和故障取证。匿名身份、公开内容、固定接纳、创建次数及补偿的业务语义统一由[公共 AI 搜索模块](../../modules/public-ai-search.md)维护；共同 Run 行为见 [AI 任务模块](../../modules/ai-assistance.md)。HTTP 与内部传输分别以 [OpenAPI](../../../backend/api/openapi.yaml)和 [Proto](../../../contracts/proto/scyg/agent/v1/common.proto)为准。

## 运行接点与配置所有权

| 接点 | 技术职责与权威入口 |
| --- | --- |
| Blog 公共边界 | [handler](../../../backend/internal/transport/rest/public_search_handler.go)签发独立用途凭证、按公开输入限制规范化创建，并向 Agent 传入服务端确定的 scope。 |
| Blog 分布式控制 | [publicsearch](../../../backend/internal/platform/publicsearch/)通过 Redis GCRA 和 token 租约实施 IP 桶及 SSE 连接控制；参数维护在 [Blog 模板](../../../backend/config.example.yaml)和[配置校验](../../../backend/internal/platform/config/public_search.go)。 |
| Agent 创建 | [ControlApplication](../../../agent/src/scyg_agent/application/control.py)在同一创建路径中处理成功键、SQL 接纳仲裁和额度预占/消费；不能由 HTTP 副本各自维护本地计数。 |
| Agent Redis 控制 | [public_controls](../../../agent/src/scyg_agent/adapters/redis/public_controls.py)使用 Redis 时间、Lua 原子操作和 token 比对，数据位于同一 hash tag；策略参数维护在 [Agent 模板](../../../agent/agent.toml.example)与[配置校验](../../../agent/src/scyg_agent/config.py)。 |
| Worker | [worker](../../../agent/src/scyg_agent/worker/)在认领前取得公共执行 token，并以冻结 scope 区分认领；生命周期退出后才释放该 token。 |
| 模型和读取预算 | [production](../../../agent/src/scyg_agent/agents/production.py)、[public_policy](../../../agent/src/scyg_agent/agents/public_policy.py)、[tool_gateway](../../../agent/src/scyg_agent/agents/tool_gateway.py)挂载模型输出、图步数、派发和授权读取预算。 |

同一部署的 Blog 副本共享 IP/SSE 前缀、Redis 和策略，Agent/Worker 共享公共控制前缀、Redis 和策略。开发、验收与生产使用不同环境前缀；前缀不能代替资源权限。不要把正文、Bearer 或 Provider key 放入 Redis key。

Blog 向浏览器宣布的输入限制不能高于 Agent 实际限制；Worker 本地容量必须满足公共执行名额的配置校验。没有 admission_capacity 可调项，不为未知字段增加看似生效的兼容配置。具体值在模板维护，本文不再复制参数默认表。

## 接纳、认领与恢复的装配要求

接纳使用现有 PostgreSQL 创建事务和共享仲裁锁；相关迁移必须先完成，不能只启动新 Worker 而让旧 scope/default 或旧认领 SQL 继续绕过约束。

Worker 认领应保持三个接点的顺序：先在本地有空槽时非阻塞取得公共 token，再只认领对应 public 候选，最后按剩余槽位认领 author。没有候选、认领异常或未开始执行时释放自己取得的 token；runner 结束、失权、取消和关闭路径都必须完成子任务退出。数据库执行 lease 与 Redis token 不同，不能互相代替或只按固定 owner 标签判断物理 Worker。

首次执行截止时间以数据库时钟持久保存，恢复沿用该值。公开 checkpoint 恢复还要在进入图之前重新验证持久来源；验证只复用已有 public GetArticle，丢弃新正文并保持原截止时间和派发围栏。完整的可见性及结果快照边界由业务模块说明，不在此重新定义。

## Provider 与工具配置

实际 Provider 协议由 models.protocol 决定；公共模型 middleware 显式覆盖对应的输出上限参数，图调用显式传入步数限制，不依赖框架默认值。工具预算在 RPC 派发前预占，完整授权结果序列化后才计入读取量；策略中止不应被 SDK 当普通工具异常吞掉继续执行。

模型可见工具 Schema 必须准确解释枚举含义，不能只给数字范围；搜索状态说明的实现位于 `_SearchArgs`。这只是模型输入可用性，执行层、公开 SQL 及授权仍按模块规则校验，不能用改提示词代替权限边界。

## Redis 持久化与故障处理

根 [Compose](../../../compose.yaml)使用 AOF、持久卷及 noeviction。正常 Redis 重启前后应核对额度/op 记录而不是只看进程启动；不要把删除卷作为常规恢复手段，也不能清空 key 后宣布额度验证成功。

额度预占由 owner/key 的操作身份与原日/token 关联；每日计数和操作记录是不同数据。补偿只调用现有 token 校验的转换，不从笼统异常推断未提交，也不人工把已消费记录改成退款。业务扣次、保守补偿和跨午夜规则见模块页。

故障时先确认 Redis 连接、配置前缀、Lua 返回及当前租约，再核对持久 Run。现有 GET/Cancel handler 的容错只作用于轻量频率适配器，不跳过身份和 owner 校验；新凭证、创建或订阅不能退回无限放行的本地实现。SSE 续租失败会停止本地订阅；只释放本连接的 token，不操作其他连接。

## 维护与验证入口

- 配置及初始化：按[运行手册](operations.md)选择根集成或独立拓扑，检查展开后的实际配置和 readiness，不输出完整私有环境。
- 创建与额度：核对输入拒绝、成功键接续、明确回滚补偿和结果不明不补偿，以及原日退款的真实存储状态。
- 副本控制：观察跨 Blog 副本共享 IP/SSE 限制、双 Worker 的实际认领与 token，不能以单进程 mock 或静态配置存在证明分布式效果。
- 模型限制：观察实际 Provider 请求参数、工具派发和持久截止时间，不能仅断言配置值被读取。
- 重启和故障：核对 AOF 保留、Redis 暂停时各入口的真实结果、失权停止和持久终态保持；操作只针对隔离测试环境。

相关测试位于 [Blog Redis 集成](../../../backend/internal/platform/publicsearch/redis_control_integration_test.go)、[Agent Redis 控制](../../../agent/tests/adapters/redis/test_public_controls.py)及[公共恢复](../../../agent/tests/agents/test_public_recovery.py)。执行方法由[开发指南](development.md)维护；源码、测试或配置存在都不能替代对应环境的真实验收回执。
