# Agent 服务运行手册

## 适用范围

本手册面向 Agent 单进程、其 `scyg_agent` 应用账号拥有的 PostgreSQL 真相库和 checkpoint schema。Blog API 与前端集成仍延期，不能用本手册声明登录、创建 Run 或浏览器交互已经可用。

## 运行前检查

1. 在仓库根目录运行 `task qa:agent:static`，确认输出 `agent QA: STATIC PASS`。
2. 动态部署前确认 Docker、Compose、获批 Agent PostgreSQL 拓扑、完整 `agent/.env`、公钥文件和 Provider 配置可用。
3. 在 `agent/` 目录运行 `docker compose config`，仅在配置可展开且未显示真实密钥时继续。
4. 运行 `docker compose up --build --wait`，等待 `agent-setup` 成功和 Agent readiness。
5. 停止或演练结束后运行 `docker compose down --volumes --remove-orphans`。

当前主机没有第 2 步所需的 Docker、Compose 和获批拓扑。不得在这里宣称 Compose、PostgreSQL、迁移、T34 或 `task qa:agent` 已执行通过。

## 密钥生成与轮换

Agent 仅消费 RSA 公钥。私钥不能进入 `agent/.env`、容器卷、Agent 配置或日志。由拥有 Blog 签发边界的受控系统生成并保存私钥，再把对应公钥文件作为 `SCYG_AGENT_JWT_PUBLIC_KEY_PATH` 挂载给 Agent。

轮换按以下顺序进行：

1. 在受控签发系统生成新的 RSA 密钥对，并保留旧公钥直到旧短期 Run JWT 自然过期。
2. 更新 Agent 挂载的公钥与 Blog 签发方，重启单进程 Agent，检查 readiness。
3. 用真实获批拓扑执行受影响的认证和 SSE 验收后，再撤销旧公钥和私钥。

不能在当前主机完成第 3 步。密钥内容、token 和完整公钥外的私钥路径都不进入 incident 工单或证据。

## SSE 游标缺口

症状是 `GET /api/runs/{run_id}/events` 返回 `409`，或客户端发现 cursor 不连续。

1. 保留 `run_id`、数值 cursor、HTTP 状态和时间，不保存 Authorization 值或 SSE data。
2. 读取 `GET /api/runs/{run_id}` 获取当前 snapshot 和 cursor，继续使用相同的 Bearer 鉴权规则。
3. 若 cursor 过旧或未来，按客户端恢复策略重新建立状态后从有效 cursor 重连。不要重复命令、模型调用或工具调用来补事件。
4. 在获批 PostgreSQL 拓扑中调查 `agent_events` 的保留和顺序。checkpoint 不能用于修复 SSE 缺口。

游标可通过 `Last-Event-ID` 或 `?cursor=<非负整数>` 传递。两种输入同时存在时必须相同，JWT must not appear in URL.

## 租约恢复和受控关闭

Worker 的租约围栏归 Agent truth 数据库所有。失去 lease 的 Worker 不能提交终态。租约过期且外部 RPC 尚未开始时可被后续 Worker 回收。`rpc_started_at` 后的外部结果未知，不能自动重放。

先使用 SIGTERM 让进程在关闭预算内 drain 到 checkpoint。若需要验证恢复，只能在隔离的获批 PostgreSQL 拓扑中杀掉 Worker，等待 lease 过期，再由替代进程回收并检查围栏拒绝旧 token。当前主机不能执行这项动态演练。

## Provider 故障

1. 记录 Provider 的稳定错误类别和 HTTP 状态，不记录请求正文或模型返回正文。
   URL must not contain secrets.
   Authorization must not appear in logs.
2. 在首个内容 delta 前，运行时可按其有界策略处理可重试故障。提交首个 delta 后不能重复请求，以免重放用户可见输出或工具副作用。
3. 确认 Run 是否已产生终态 `run_failed` 或 `run_cancelled`，通过 SSE cursor 回放观察，不依赖通知消息。
4. 恢复 Provider 后只处理新的或明确可恢复的 Run。未知外部工具结果必须保持未知，不能猜测成功。

## checkpoint 兼容性

LangGraph 或 checkpoint 依赖升级是兼容性变更，不是常规重启。先在隔离拓扑验证已有 checkpoint 的恢复，再部署。readiness 因 checkpoint schema、对象、迁移序列或依赖元数据不兼容而失败时，停止 rollout，保留数据并回退经验证的版本。不要删除 checkpoint、手工修表或用事件表替代执行状态。

## 扩容触发条件

初始部署是一进程，SIMPLE 4、DEEP 1。只有持续容量不足且以下真实 PostgreSQL 验收完成后，才评估多进程：`SKIP LOCKED` 认领无重叠、lease fencing 拒绝旧提交、通知丢失后的回放、进程 drain、checkpoint 恢复、T34 S1 至 S3。不得通过增加动态 runtime、Redis 或共享内存改变现有真相边界。

## T34 运行边界

T34 首版仅包含：S1 认证 SIMPLE Run 到终态 SSE，S2 两连接 cursor 重连且严格递增并无重复 `event_id`，S3 幂等 cancel 或 command 重放。完整入口 `task qa:agent` 会在 Docker、Compose 和拓扑前置满足后运行它。

当前主机只完成静态 QA 和 T34 harness 合同确认。没有 S1 至 S3 receipt，也没有动态 E2E 完成结论。崩溃恢复、DEEP 等待输入、BlogTool deadline、浏览器、扩展 JWT 和相邻路由场景均延期，不能将它们写入首版执行结论。
