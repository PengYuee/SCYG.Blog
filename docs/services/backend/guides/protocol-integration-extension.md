# 未来协议与外部集成扩展指南

本文规定现有 Agent 集成与未来协议扩展的边界。当前已启用共享 Proto、AgentControlService client 与独立 BlogContentService listener；尚未启用 WebSocket、Kafka 或 Outbox，不能为未来需求预建空壳。

## 独立 gRPC Proto

当前 Blog↔Agent 合同位于根 `contracts/proto/scyg/{agent,blog}/v1/`，由固定 Buf 插件生成至 `backend/internal/generated/proto/`，执行 `task proto:generate` 重建。Blog 对外 HTTP/SSE 仍以 OpenAPI 为源契约；gRPC DTO 与 canonical status/details 由各自 adapter 映射，不共用传输 envelope。BlogContent listener 纳入 readiness、标准 Health 和独立有界 graceful-stop；Agent 下线不影响 Blog 启动。现行接入入口与网络边界见[架构](../architecture/current-state-architecture.zh-CN.md)。

新增独立 RPC 仅在出现真实服务调用或强类型流式需求时实施，沿用共享 `contracts/` 与版本化 package；删除字段 reserve，不创建第二套 `api/proto`、生成器或兼容客户端。

## Binary Protobuf WebSocket

只有真实双向实时交互存在时才在现有 HTTP server 挂载 `wss://.../ws/v1`。subprotocol 固定为 `scyg.realtime.protobuf.v1`，只接受 binary frame；方向分离的 `ClientMessage`/`ServerMessage` 各自使用 `oneof`，携带 message/correlation/causation ID、UTC Timestamp，服务端消息携带 sequence 与 typed Error。必须限制 frame、建立 backpressure、deadline、Ping/Pong，并以 REST 获取重连后的权威最终状态。Go/TypeScript 消息统一由 Buf 生成。

## External ACL

当前 Agent ACL 位于 `internal/adapters/agent/`，翻译五 RPC 的 Run、事件和公开错误；领域/application 不导入 gRPC 生成 DTO。新增确定的外部集成才创建相应窄适配器；即时调用必须 deadline-bound，双方不得读取对方数据库。BlogContent 写入先在事务外预备 Blob 与图片校验，再通过 `operation_id` 仲裁业务事务，不在持锁事务中执行 Blob 或网络 I/O。

成功重放读取文章和标签时使用同一条 SQL 的快照，不能拼接不同提交版本。REST 文章与分类摘要的跨 feature 快照规则统一由[当前架构](../architecture/current-state-architecture.zh-CN.md#http-与-openapi)维护，mapper 不再二次查询分类。

## Outbox

仅当已提交本地状态必须可靠触发跨服务副作用、事件不可丢失，或工作必须脱离请求重试/扇出时启用。届时业务行和 Outbox 行在同一 PostgreSQL 事务提交；publisher 至少一次投递，event ID 是消费者幂等键，并补齐 claim、重试、可观测性、停机清理与真实 broker/DB 测试。未满足触发条件时不得创建表、worker 或 broker 依赖。

## DTO 与错误边界

REST、gRPC、WebSocket 各自定义 DTO、成功形状、状态和错误表达：REST 使用 bare resource/list page、HTTP status/header 与 RFC 9457；gRPC 使用 typed Proto response 和 canonical codes/details；WebSocket 使用方向安全消息与 typed Error。它们只共享 stable semantic codes、UTC 时间、版本、correlation/causation ID 和 retryability，不共享 universal envelope、`ContentAPI` 或生成 API。
