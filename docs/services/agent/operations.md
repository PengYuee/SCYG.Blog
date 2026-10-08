# Agent 服务运行手册

## 适用范围

本手册面向 Agent 单进程、PostgreSQL 真相库、checkpoint 及 Redis 流。Blog 统一承接浏览器 JWT、业务 HTTP 与透明 SSE，Agent 仅提供内部 gRPC 和 health HTTP；当前路径与源码依据见[架构](architecture.md)。

## 运行前检查

1. 在仓库根目录运行 `task qa:agent:static`，按实际退出码与输出记录结果；静态门禁不覆盖真实 PostgreSQL、Redis、Provider 或 Compose。
2. 动态部署前确认 Docker、Compose、获批 Agent PostgreSQL 拓扑、完整 `agent/.env`、Blog 内部 gRPC 地址和 Provider 配置可用；Agent 不需要 JWT 公钥。
3. 在所选拓扑目录运行 `docker compose config --quiet`，确认配置可展开，不输出包含真实凭据的完整配置。
4. 运行 `docker compose up --build --wait`，等待 `agent-setup` 成功和 Agent readiness。
5. 正常停止运行 `docker compose down --remove-orphans`，保留数据库卷。只有确认要删除本地状态时才追加 `--volumes`。

配置展开、readiness 与业务冒烟分别记录实际结果；静态检查或进程存活不能代替完整动态验收。原生安装与运行步骤见[首次本机启动](README.md#首次本机启动)；Docker 构建不接收宿主机 `BUF_TOKEN`，原生生成设置的 token 不适用于容器生成。

## 认证与内部网络边界

Blog 管理浏览器登录 JWT 的签发、校验与轮换。Agent 不消费 RSA 公钥、Run JWT 或 service JWT，也不承接浏览器业务认证。AgentControl 与 BlogContent 两个方向的 gRPC 只通过内部网络使用，携带显式可信 `user_id` 并逐次检查所有权或管理权限；不得暴露到公网。

本地 [Agent Compose](../../../agent/compose.yaml) 只映射 PostgreSQL 与 health HTTP 的 loopback 端口，不映射 Agent gRPC；根 [Compose](../../../compose.yaml) 提供 Blog↔Agent 集成，Blog 指向 `agent:9090`，Agent 指向 `blog:9091`，两个 gRPC 端口均不映射宿主。运行 JWT 的旧公钥挂载及环境变量已不属于当前配置。

根集成拓扑使用根 `.env.example`，独立 Agent 拓扑使用 `agent/.env.example`，两者的数据库主机名与 Blog 配置不同，不互换模板。根目录运行 `Copy-Item .env.example .env`，填写其中必填值后执行 `docker compose up --build --wait`。本机 Provider 的容器 URL 使用 `host.docker.internal`；模型名称须来自该 Provider 的实际支持列表。

根拓扑先以一次性 `blog-setup` 和 `agent-setup` 完成各自数据库初始化，两个任务成功后才启动对应常驻服务。Blog 初始化复用镜像内 `/migrate -config= up`；重复启动应用尚未完成的迁移，不清除已有业务数据。Blog 常驻进程仍只检查迁移版本，不自动修改 schema。

## SSE 重连与流过期

当前浏览器 SSE 使用 Agent 编码的 Run-bound opaque cursor，由 Blog 透明转发；不是 Redis Stream ID 或公开数值 cursor。格式与 subscription-ready 边界见[流式合同](streaming.md)。

遇到断线或游标错误时，记录 Run、cursor、状态和时间，不保存 Authorization 值或完整 SSE data。通过 Blog 读取已授权 Run 快照判断持久状态，再按合同恢复订阅；不得重跑模型、工具或 mutation 来补流。checkpoint 也不是 SSE 事件日志。

## 租约恢复和受控关闭

Worker 的租约围栏归 Agent truth 数据库所有，失去 lease 的 Worker 不能提交终态。Tool 成功账本仅在已有 `rpc_started_at` 标记时将恢复结果视为未知；当前生产读取 Gateway 使用 Run dispatcher，事务退出异常会取消已创建的本地 gRPC Call，但不能据此断言远端未执行或自动重放外部写入。

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

## 完整门禁与运行证据

从仓库根运行 `task qa:agent`，检查范围与前置条件见[开发指南](development.md#本地拓扑)。该入口执行当前 Blog↔Agent gRPC/Recipe 路径的完整门禁，静态 QA 不能替代真实 Redis、PostgreSQL、Provider 与 SSE 的动态验收。

每次验证保留实际命令、对应代码状态、退出码和 receipt，未执行或环境阻塞时明确记录，不宣称 E2E 已通过。
