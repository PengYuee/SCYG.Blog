# Agent 服务运行手册

## 适用范围

本手册面向 Agent 单进程、PostgreSQL 真相库、checkpoint 及 Redis 流。Blog 与前端 AI 集成尚未接通；当前路径与源码依据见[架构](architecture.md)。

## 运行前检查

1. 在仓库根目录运行 `task qa:agent:static`，确认输出 `agent QA: STATIC PASS`。当前静态门禁已通过；它不覆盖真实 PostgreSQL、Redis、Provider 或 Compose。
2. 动态部署前确认 Docker、Compose、获批 Agent PostgreSQL 拓扑、完整 `agent/.env`、公钥文件和 Provider 配置可用。
3. 在 `agent/` 目录运行 `docker compose config`，仅在配置可展开且未显示真实密钥时继续。
4. 运行 `docker compose up --build --wait`，等待 `agent-setup` 成功和 Agent readiness。
5. 正常停止运行 `docker compose down --remove-orphans`，保留数据库卷。只有确认要删除本地状态时才追加 `--volumes`。

当前完整门禁在 T32 因主机缺少 Docker 阻断；Compose、真实拓扑和 T34 尚未执行。动态验收结果、运行日志和 receipt 保留在对应本地任务目录。

## 密钥生成与轮换

Agent 仅消费 RSA 公钥。私钥不能进入 `agent/.env`、容器卷、Agent 配置或日志。由拥有 Blog 签发边界的受控系统生成并保存私钥，再把对应公钥文件作为 `SCYG_AGENT_JWT_PUBLIC_KEY_PATH` 挂载给 Agent。

轮换按以下顺序进行：

1. 在受控签发系统生成新的 RSA 密钥对，并保留旧公钥直到旧短期 Run JWT 自然过期。
2. 更新 Agent 挂载的公钥与 Blog 签发方，重启单进程 Agent，检查 readiness。
3. 用真实获批拓扑执行受影响的认证和 SSE 验收后，再撤销旧公钥和私钥。

若没有真实拓扑或签发端，记录尚未验证的范围；配置或静态检查不能代替认证和 SSE 验收。密钥内容、token 和私钥路径不进入普通日志或证据。

## SSE 重连与流过期

当前生产组合使用 Redis Stream ID；未注入 Redis 的持久事件分支才使用非负整数 cursor。两者不可互换，格式、错误及 `stream_expired` 恢复提示见[流式合同](streaming.md)。

遇到断线、流过期或游标错误时，记录 Run、cursor、状态和时间，不保存 Authorization 值或完整 SSE data。读取已授权 Run 快照判断持久状态，再按对应流的合同恢复订阅；不得重跑模型、工具或 mutation 来补流。checkpoint 也不是 SSE 事件日志。

## 租约恢复和受控关闭

Worker 的租约围栏归 Agent truth 数据库所有。失去 lease 的 Worker 不能提交终态。租约过期且外部 RPC 尚未开始时可被后续 Worker 回收。`rpc_started_at` 后的外部结果未知，不能自动重放。

正常关闭让进程在预算内 drain。杀进程、等待 lease 过期并由替代进程恢复，只用于隔离的真实拓扑演练；必须验证旧 lease 提交被拒绝，不能用源码存在来声明恢复已通过。

## Provider 故障

1. 记录 Provider 的稳定错误类别和 HTTP 状态，不记录请求正文或模型返回正文。
   URL must not contain secrets.
   Authorization must not appear in logs.
2. 提供方失败时先核对实际运行器的重试与输出边界；已输出内容或外部副作用后，不得仅为补流而重复模型或工具请求。
3. 确认 Run 是否已产生终态 `run_failed` 或 `run_cancelled`，通过 SSE cursor 回放观察，不依赖通知消息。
4. 恢复 Provider 后只处理新的或明确可恢复的 Run。未知外部工具结果必须保持未知，不能猜测成功。

## checkpoint 兼容性

LangGraph 或 checkpoint 依赖升级是兼容性变更，不是常规重启。先在隔离拓扑验证已有 checkpoint 的恢复，再部署。readiness 因 checkpoint schema、对象、迁移序列或依赖元数据不兼容而失败时，停止 rollout，保留数据并回退经验证的版本。不要删除 checkpoint、手工修表或用事件表替代执行状态。

## 扩容触发条件

先使用当前单进程拓扑。只有持续容量不足，且真实 PostgreSQL/Redis 验证认领无重叠、lease fencing、流过期恢复、drain 和 checkpoint 恢复后，再评估多进程；容量与运行器以现行配置和组合根为准。

## T34 运行边界

T34 首版仅包含：S1 认证 SIMPLE Run 到终态 SSE，S2 两连接 cursor 重连且严格递增并无重复 `event_id`，S3 幂等 cancel 或 command 重放。完整入口 `task qa:agent` 会在 Docker、Compose 和拓扑前置满足后运行它。

历史静态 QA 或 T34 harness 结果不代表当前 Redis/Recipe 路径的动态验收。每次验证保留实际命令、对应代码状态、退出码和 receipt，未执行或环境阻塞时明确记录，不宣称 E2E 已通过。
