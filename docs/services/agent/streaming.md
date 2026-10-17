# Agent gRPC 与 Blog 流式合同

## 适用范围

本文描述浏览器流式边界；Blog 是作者 JWT、匿名搜索凭证与业务 HTTP 的唯一入口，Agent HTTP 仅提供 health。内部协议以 [AgentControlService](../../../contracts/proto/scyg/agent/v1/agent_control_service.proto) 为准，帧编码以 [event_subscription.py](../../../agent/src/scyg_agent/application/event_subscription.py) 为准。

## 当前传输表面

Agent 内部 gRPC 提供 CreateRun、GetRun、StreamRunEvents、ResumeRun、CancelRun。四个 unary RPC 返回同一 Run 合同，流 RPC 返回完整 SSE 帧；可信请求明确携带主体及内容范围，应用门面逐次核对持久 owner 和范围，不使用 Run JWT 或 service JWT。

Agent HTTP 只有 /health/live 和 /health/ready，浏览器只连接 Blog。作者/匿名的入口与业务差异分别见 [AI 任务](../../modules/ai-assistance.md)和[公共搜索](../../modules/public-ai-search.md)；HTTP 路径、安全方案和状态码以 [OpenAPI](../../../backend/api/openapi.yaml)为准。

## 透明 SSE 与 opaque cursor

当前生产订阅读取 PostgreSQL 持久事件；Redis 仍服务 Worker 瞬时流，但不作为浏览器公开 SSE 的事件来源。

- 浏览器通过 `Last-Event-ID` 传入 1–256 字节 ASCII opaque cursor；Blog 只校验传输格式并原样传递到 `after_event_id`，不解析、翻译或以 query 参数替代。
- Agent 先验证主体、Run owner 和范围，再校验绑定该 Run 的 cursor 并打开持久订阅；cursor 不是 Redis Stream ID，也不是浏览器可解释的数值序号。
- 只有订阅完成 listener 注册和竞态回放准备后，Agent 才发送 `scyg-subscription-ready: 1` 初始 metadata。Blog 等待该信号后才提交 HTTP `200 text/event-stream`；就绪前的失败仍返回 HTTP 错误。
- Agent 编码包含 `id`、`event`、`data` 及帧终止空行的完整 SSE 帧；data 包含稳定 `event_id`、Run、revision、时间、kind 和 payload。Blog 直接写出帧字节，不重命名事件或重排 payload。
- 空闲时 Agent 每 10 秒发送 `: heartbeat` 注释帧。断线只释放订阅，不取消 Run，也不重新执行模型或工具。
- 无效 cursor 按参数错误处理，超过持久回放边界的 cursor 按 `FAILED_PRECONDITION` 处理。恢复时读取已授权持久快照；checkpoint 不是 SSE 事件日志。

Blog 在建立订阅及返回错误期间保持有限 HTTP 写期限，ready 后逐帧设置写期限并检查底层 flush 错误；接收超时或写入失败都会释放上游订阅。可信内部 gRPC 的接收设置不使用默认 4 MiB 上限，合法的大文章查询页、Run 结果和 SSE 帧不应因此被截断；创建及 Resume payload 仍遵守 Proto 的输入限制。

匿名 SSE 在授权后还取得跨 Blog 副本共享的 Redis 连接租约；名额、续租和 Redis 故障策略见 [公共搜索控制](public-search.md)。连接租约不改变事件来源，也不保存第二份 Run 真相。

## 控制操作的业务引用

创建、恢复、取消、成功幂等窗口及未知操作处理统一由 [AI 任务模块](../../modules/ai-assistance.md)维护；匿名用途及额度差异由[公共搜索模块](../../modules/public-ai-search.md)维护。传输实现必须保留原身份、key、原始业务输入及 payload 缺失与显式 JSON null 的区别，不自行合并或补造回应。

具体交互请求/响应由 [agents/contracts.py](../../../agent/src/scyg_agent/agents/contracts.py)和固定生产节点定义，HTTP 包络由 OpenAPI 定义。SSE 负责转发持久事实，不负责重新执行业务操作或保存第二份最终结果。

## 验证边界

对应代码变化后，应在隔离的真实拓扑验证状态、完整帧及 `Last-Event-ID` 排他回放，入口见[开发指南](development.md#本地拓扑)。旧 HTTP/Redis 数值游标场景不能证明当前 gRPC/Recipe 路径；记录实际命令与结果，未执行或未通过的门禁不能视为验收通过。
