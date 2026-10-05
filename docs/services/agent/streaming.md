# Agent HTTP 与流式合同

## 适用范围

本文描述当前 Agent HTTP 表面，不代表 Blog 或浏览器 AI 入口已接通。服务内部关系见[架构](architecture.md)，路由与编码的权威来源为 [router.py](../../../agent/src/scyg_agent/transport/http/router.py) 和 [sse.py](../../../agent/src/scyg_agent/transport/http/sse.py)。

## HTTP 表面

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/api/runs/{run_id}` | 已授权 Run 快照与持久状态 |
| GET | `/api/runs/{run_id}/events` | `text/event-stream`，按注入的 stream store 选择事件来源 |
| POST | `/api/runs/{run_id}/input` | 提交绑定 interaction 的输入 |
| POST | `/api/runs/{run_id}/commands` | 提交公开命令 |
| POST | `/api/runs/{run_id}/cancel` | 提交持久化取消请求 |

Agent 没有 HTTP 创建 Run 入口，控制面合同在根 [contracts](../../../contracts/) 中维护。路由要求 `Authorization: Bearer <run-jwt>`，校验签名、issuer、audience、用户、Run 和 scope；令牌不得进入 URL、查询参数、日志或 SSE data。

## 生产 Redis 流

当前[组合根](../../../agent/src/scyg_agent/composition.py)向 HTTP 注入 Redis stream store，SSE 使用 Redis 原生 Stream ID，而不是 PostgreSQL 数值 cursor。

- 可从 `Last-Event-ID` 或 `?cursor=` 提供流游标；同时提供时必须一致。重复请求头、冲突值或无效 Redis 游标返回 `400`。
- 服务先验证 Run 所有权与快照，再读取 Redis 流；不存在的 Run 返回 `404`。
- 帧 `id:` 保留 Redis Stream ID，data 包含 `run_id`、`attempt`、`sequence`、`occurred_at`、`kind` 与 `payload`。
- 流过期输出 `stream_expired` 帧，提供持久快照地址与状态提示；Redis 故障输出经过脱敏的 `error` 帧，代码为 `redis_unavailable`。
- Redis 流属于瞬时数据，保留长度与 TTL 由[配置](../../../agent/src/scyg_agent/config.py)控制。流过期不意味着 Run 被删除，客户端应查询持久快照，不能重跑模型或工具来补事件。

## 未注入 Redis 的持久事件分支

`HTTPDependencies.stream_store` 为 `None` 时，路由使用 PostgreSQL 持久事件与数值 cursor。该分支仍存在，但不是当前生产组合的默认 SSE 路径。

- cursor 为非负整数，来源同样是 `Last-Event-ID` 或 `?cursor=`；两者同时提供必须相同。
- 游标格式错误、重复请求头或冲突返回 `400`；持久回放边界冲突返回 `409`。
- 服务回放数值 cursor 之后的 `agent_events`，再跟随通知唤醒后的查询。通知不是事件真相。
- 帧 `id:` 为数值序列，data 包含稳定的 `event_id`、Run、revision、时间、事件类型和 payload。

**两类 cursor 不可互换。** 快照中的持久事件 cursor 不能直接作为 Redis Stream ID 使用；断线不取消 Run，重连也不应重新执行模型或工具。

## 背压、取消与幂等

SSE 订阅具有有界缓冲、心跳和慢客户端超时。断开连接只关闭当前订阅，不代表任务取消。缓存与代理缓冲响应头在传输层维护。

命令和输入通过持久命令、交互与审计处理。首次 mutation 返回 `Idempotency-Replayed: false`，相同身份与不可变输入重放时返回 `true`；冲突或前置条件失败返回 `409`，不能通过重复请求制造第二次业务副作用。取消请求与业务终态的区别以应用门面和领域状态机为准。

## 验证边界

T34 驱动定义了 SIMPLE 终态 SSE、游标重连与幂等重放场景。它们的存在或历史静态检查结果不能证明当前 Redis/Recipe 路径已通过动态验收。更改事件来源或 cursor 合同后，应在隔离的真实拓扑中验证对应路径，再记录命令、结果和 receipt；本次文档迁移不声称这些场景已执行通过。
