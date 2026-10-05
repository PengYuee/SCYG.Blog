# 未来协议与外部集成扩展指南

本文只规定触发条件和落地步骤。当前不得创建 gRPC、WebSocket、Buf、Kafka、Outbox 依赖、代码、监听器、表或空目录。

## 独立 gRPC Proto

只有出现真实服务调用、强类型 RPC 或 REST 无法满足的流式需求时才添加。届时在 `api/proto/<domain>/v1` 建立独立版本化 RPC contract，固定 Buf lint/breaking/generate，以及 `protoc-gen-go`、`protoc-gen-go-grpc`；删除字段必须 reserve。adapter 自己拥有窄消费接口，将 Proto DTO 与 canonical status/details 映射到协议中立用例。独立 listener 纳入共同取消、readiness 和有界 graceful-stop，超时后强制停止。

## Binary Protobuf WebSocket

只有真实双向实时交互存在时才在现有 HTTP server 挂载 `wss://.../ws/v1`。subprotocol 固定为 `scyg.realtime.protobuf.v1`，只接受 binary frame；方向分离的 `ClientMessage`/`ServerMessage` 各自使用 `oneof`，携带 message/correlation/causation ID、UTC Timestamp，服务端消息携带 sequence 与 typed Error。必须限制 frame、建立 backpressure、deadline、Ping/Pong，并以 REST 获取重连后的权威最终状态。Go/TypeScript 消息统一由 Buf 生成。

## External ACL

仅在接入确定的外部服务时创建 `internal/integration/<service>`。ACL 翻译外部术语、标识、DTO、错误、timeout 与 retry policy 到 application 消费端口；领域/application 不导入外部 SDK。即时调用必须 deadline-bound，严禁在本地数据库事务内发起网络请求，双方也不得访问对方数据库。

## Outbox

仅当已提交本地状态必须可靠触发跨服务副作用、事件不可丢失，或工作必须脱离请求重试/扇出时启用。届时业务行和 Outbox 行在同一 PostgreSQL 事务提交；publisher 至少一次投递，event ID 是消费者幂等键，并补齐 claim、重试、可观测性、停机清理与真实 broker/DB 测试。未满足触发条件时不得创建表、worker 或 broker 依赖。

## DTO 与错误边界

REST、gRPC、WebSocket 各自定义 DTO、成功形状、状态和错误表达：REST 使用 bare resource/list page、HTTP status/header 与 RFC 9457；gRPC 使用 typed Proto response 和 canonical codes/details；WebSocket 使用方向安全消息与 typed Error。它们只共享 stable semantic codes、UTC 时间、版本、correlation/causation ID 和 retryability，不共享 universal envelope、`ContentAPI` 或生成 API。
