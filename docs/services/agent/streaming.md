# Agent gRPC 与 Blog 流式合同

## 适用范围

本文描述当前浏览器流式边界；Blog 是浏览器 JWT 认证与业务 HTTP 的唯一入口，Agent HTTP 仅提供 health。协议以 [AgentControlService](../../../contracts/proto/scyg/agent/v1/agent_control_service.proto) 为准，帧编码以 [event_subscription.py](../../../agent/src/scyg_agent/application/event_subscription.py) 为准。

## 当前传输表面

Agent 内部 gRPC 提供 `CreateRun`、`GetRun`、`StreamRunEvents`、`ResumeRun`、`CancelRun`。四个 unary RPC 返回同一 `Run` 合同，流 RPC 返回完整 SSE 帧字节。内部网络信任 Blog 传入的 `user_id`，应用门面逐次校验 Run 所有权，不使用 Run JWT 或 service JWT。

Agent HTTP 只有 `/health/live` 与 `/health/ready`。浏览器从 Blog `/api/v1/ai/{search|write|polish|chat}` 创建 Run，从 `/api/v1/runs/{runId}` 查询快照，并通过该 Run 下的 `/events`、`/resume`、`/cancel` 订阅或操作。浏览器使用 Blog 登录 JWT，不直接连接 Agent 业务端点。

## 透明 SSE 与 opaque cursor

当前生产订阅读取 PostgreSQL 持久事件；Redis 仍服务 Worker 瞬时流，但不作为浏览器公开 SSE 的事件来源。

- 浏览器通过 `Last-Event-ID` 传入 1–256 字节 ASCII opaque cursor；Blog 只校验传输格式并原样传递到 `after_event_id`，不解析、翻译或以 query 参数替代。
- Agent 先验证 Run 所有权，再校验绑定该 Run 的 cursor 并打开持久订阅；cursor 不是 Redis Stream ID，也不是供浏览器解释的数值序号。
- 只有订阅完成 listener 注册和竞态回放准备后，Agent 才发送 `scyg-subscription-ready: 1` 初始 metadata。Blog 等待该信号后才提交 HTTP `200 text/event-stream`；就绪前的失败仍返回 HTTP 错误。
- Agent 编码包含 `id`、`event`、`data` 及帧终止空行的完整 SSE 帧；data 包含稳定 `event_id`、Run、revision、时间、kind 和 payload。Blog 直接写出帧字节，不重命名事件或重排 payload。
- 空闲时 Agent 每 10 秒发送 `: heartbeat` 注释帧。断线只释放订阅，不取消 Run，也不重新执行模型或工具。
- 无效 cursor 按参数错误处理，超过持久回放边界的 cursor 按 `FAILED_PRECONDITION` 处理。恢复时读取已授权持久快照；checkpoint 不是 SSE 事件日志。

Blog 在建立订阅及返回错误期间保持有限 HTTP 写期限，ready 后逐帧设置写期限并检查底层 flush 错误；接收超时或写入失败都会释放上游订阅。可信内部 gRPC 的接收设置不使用默认 4 MiB 上限，合法的大文章查询页、Run 结果和 SSE 帧不应因此被截断；创建及 Resume payload 仍遵守 Proto 的输入限制。

## 取消、恢复与成功幂等

Create、Resume、Cancel 的 `Idempotency-Key` 为 UUID v4。Agent 在三个 RPC 间共享 `(owner, key)` 成功记录，成功后保留 24 小时；同一用户命中既有 key 时返回原 Run 的当前一致快照，不重新执行业务输入。失败不占 key，不使用旧 HTTP mutation 的 `Idempotency-Replayed` 响应头合同。

Resume 绑定当前 interaction，并区分缺失 payload 与显式 JSON null。成功键重放优先于新业务校验；未命中时，无法以当前 JSON/UTF-8 表示落盘的 NUL、孤立 surrogate 或非有限数值返回参数错误，不占用成功键，且保留待处理交互。Cancel 持久建立取消围栏；已启动调用的本地取消不承诺远端未执行，业务终态仍由持久化围栏决定。

## 验证边界

对应代码变化后，应在隔离的真实拓扑验证状态、完整帧及 `Last-Event-ID` 排他回放，入口见[开发指南](development.md#本地拓扑)。旧 HTTP/Redis 数值游标场景不能证明当前 gRPC/Recipe 路径；记录实际命令与结果，未执行或未通过的门禁不能视为验收通过。
