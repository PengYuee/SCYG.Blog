# SCYG Agent 后端 API 需求文档（修订版）

## 1. 文档状态

- 状态：已确认设计，待实施。
- 读者：Blog 后端、Agent 服务和前端 API 实现者。
- 维护位置：本文维护 Blog 对外 HTTP/SSE 边界和 Blog↔Agent 集成合同；Agent 内部 Run、Recipe、Tool、HITL、模型、事件持久化和 checkpoint 由 Agent 项目文档维护。
- 版本：v1。

本文不是当前代码实现说明。当前 `backend/` 尚未实现本文的 Agent HTTP API、Blog↔Agent gRPC 集成和 BlogContentService；实现完成前不得将本文内容视为已上线能力。

## 2. 目标和边界

调用链：

```text
Web 前端 → Blog HTTP/SSE → AgentControlService gRPC → Agent
Agent → BlogContentService gRPC → Blog 业务能力
```

Blog 是浏览器唯一的 Agent 业务入口。Agent 不向浏览器暴露创建 Run、查询 Run、SSE、Resume 或 Cancel 接口。

Blog 负责：

- 验证用户 JWT，并取得 `user_id`；
- 将 `user_id` 传给 Agent；
- 把 HTTP/SSE 请求映射为 AgentControlService 调用；
- 把 Agent 结果和事件映射回 HTTP/SSE；
- 对 Agent→Blog 的文章、分类和标签调用执行 Blog 自身的资源权限校验；
- 为 Blog 业务写入提供幂等。

Agent 负责：

- Run owner 校验、状态和生命周期；
- Recipe、Prompt、Tool、Skill、模型、HITL、Worker 和 checkpoint；
- Run 事件、最终结果和 AgentControlService 的最终幂等；
- 保证面向浏览器的结果和错误已经脱敏。

Blog 不解析或决定 Recipe、模型、Tool、Skill、middleware、Runtime、HITL 或 Agent 内部状态。

## 3. 公开 HTTP API

所有路径使用 `/api/v1`。所有需要用户身份的请求使用当前 Blog Bearer JWT。

### 3.1 路由

| 方法 | 路径 | 作用 | 成功状态 |
| --- | --- | --- | --- |
| POST | `/api/v1/ai/search` | 创建 search Run | 202 |
| POST | `/api/v1/ai/write` | 创建 write Run | 202 |
| POST | `/api/v1/ai/polish` | 创建 polish Run | 202 |
| POST | `/api/v1/ai/chat` | 创建 chat Run | 202 |
| GET | `/api/v1/runs/{runId}` | 查询 Run 和结果 | 200 |
| GET | `/api/v1/runs/{runId}/events` | 订阅 Run SSE | 流式 200 |
| POST | `/api/v1/runs/{runId}/resume` | 恢复交互中的 Run | 200 |
| POST | `/api/v1/runs/{runId}/cancel` | 取消 Run | 200 |

不提供独立的 `/result` 接口；最终结果由 `GET /api/v1/runs/{runId}` 返回。

### 3.2 创建 Run

创建请求必须携带：

```http
Authorization: Bearer <user-jwt>
Content-Type: application/json
Idempotency-Key: <uuid-v4>
```

请求体要求：

- `Content-Type` 必须为 `application/json`，允许 `charset` 参数；
- 请求体必须存在且是合法 JSON；
- 顶层可以是任意 JSON 值，包括 `null`；
- 最大 1 MiB；
- Blog 不解析业务字段，不向 body 注入 `user_id`；
- 具体 payload 由 Agent 负责校验和解释。

四个路径固定映射为以下 capability，客户端不能提交或覆盖：

| 路径 | capability |
| --- | --- |
| `/api/v1/ai/search` | `search` |
| `/api/v1/ai/write` | `write` |
| `/api/v1/ai/polish` | `polish` |
| `/api/v1/ai/chat` | `chat` |

Blog 将原始 JSON bytes、`user_id`、固定 capability 和 `Idempotency-Key` 发送给 Agent。

创建成功返回 `202 Accepted` 和统一 Run 响应。

### 3.3 查询 Run

```http
GET /api/v1/runs/{runId}
Authorization: Bearer <user-jwt>
```

`runId` 是 Agent 生成的不透明 URL-safe ASCII 字符串，长度为 1–128 个字符，允许字符为：

```text
[A-Za-z0-9._~-]
```

Blog 将当前 `user_id` 和 `runId` 传给 Agent。Agent 负责 Run owner 校验；Run 不存在和当前用户不是 owner 均映射为 HTTP 404。

### 3.4 Resume

```http
POST /api/v1/runs/{runId}/resume
Authorization: Bearer <user-jwt>
Content-Type: application/json
Idempotency-Key: <uuid-v4>
```

请求体必须是 JSON object：

```json
{
  "interactionId": "interaction_123",
  "decision": "approve",
  "payload": {}
}
```

规则：

- `interactionId` 必须为非空字符串，长度为 1–128 个 UTF-8 字节；
- `decision` 必须为非空字符串，Blog 不限制其枚举值；
- `payload` 可省略，省略与显式 JSON `null` 由 Agent 区分；
- Blog 不解释 decision 或 payload；
- Agent 负责交互类型、决定内容和恢复条件校验。

成功返回 `200` 和最新 Run 响应。

同一 key 重试返回首次操作绑定 Run 的当前状态。interaction 已处理后，使用新的 key 再次 Resume 返回冲突；不允许覆盖已经处理的决定。

### 3.5 Cancel

```http
POST /api/v1/runs/{runId}/cancel
Authorization: Bearer <user-jwt>
Idempotency-Key: <uuid-v4>
```

请求没有 body。成功返回 `200` 和最新 Run 响应。

取消规则由 Agent 负责：

- Cancel 设置取消栅栏后立即返回 `cancelled`；
- Agent 不再发起新的 Tool 调用；
- 已经在途的 Blog 业务写请求可以完成；
- 已完成的 Blog 业务结果保留，但不得把 Run 推进为成功；
- 已 `cancelled` 的 Run 使用新 key 再次 Cancel，返回当前 Run；
- 已 `succeeded` 或 `failed` 的 Run 不可改为 `cancelled`，返回冲突。

## 4. 统一 Run HTTP 响应

Create、Get、Resume、Cancel 成功响应使用同一外层：

```json
{
  "runId": "run-safe-id",
  "status": "queued",
  "capability": "write",
  "recipeId": "writing",
  "recipeVersion": "v1",
  "createdAt": "2026-10-06T08:00:00Z",
  "updatedAt": "2026-10-06T08:00:01Z",
  "failure": null,
  "result": null,
  "pendingInteraction": null,
  "streamUrl": "/api/v1/runs/run-safe-id/events"
}
```

字段规则：

- HTTP 字段使用 camelCase；
- `status` 只允许：`queued`、`running`、`waitingForApproval`、`succeeded`、`failed`、`cancelled`；Blog 不实现状态机；
- `capability` 只允许：`search`、`write`、`polish`、`chat`；
- `recipeId` 和 `recipeVersion` 从创建成功开始必须非空，客户端只能读取；
- `createdAt`、`updatedAt` 使用 RFC 3339 UTC 字符串；
- `result` 是 Agent 提供的任意 JSON，无结果时为 `null`；
- `failure` 是可空对象，结构为 `{ "code": "...", "message": "..." }`；`code` 和 `message` 由 Agent 提供，Blog 不翻译或解释；
- `result` 与 `failure` 可以同时存在；
- `pendingInteraction` 无待处理交互时为 `null`，否则结构为：

```json
{
  "interactionId": "interaction_123",
  "type": "confirmation",
  "payload": {}
}
```

- `pendingInteraction.type` 只允许 `confirmation`、`selection`、`textInput`；
- `pendingInteraction.payload` 是 Agent 定义的任意 JSON；
- `streamUrl` 是 Blog 根据 `runId` 生成的相对路径。

## 5. SSE

前端使用 `fetch` 和 `ReadableStream` 读取 SSE，以便携带 Bearer JWT；不使用 Cookie、URL token 或原生 `EventSource`。

```http
GET /api/v1/runs/{runId}/events
Authorization: Bearer <user-jwt>
Last-Event-ID: <opaque-cursor>
```

`Last-Event-ID`：

- 可省略；省略时由 Agent 使用默认订阅位置；
- 提供时限制为 1–256 个 ASCII 字节；
- Blog 不解析、不比较、不持久化；
- Blog 将其作为不透明 `after_event_id` 原样传给 Agent。

建立 SSE 前，Blog 先调用一次 `GetRun(user_id, run_id)` 完成授权；成功后再调用 `StreamRunEvents`。Agent 对 Stream RPC 再次执行 owner 校验。

Agent 的流消息是已经编码好的完整 SSE frame bytes。Blog：

- 原样写入 `event`、`id`、`data` 和注释行；
- 不生成业务序号；
- 不重新排序或重建事件；
- 不保存完整事件；
- 每条消息写入后 flush；
- 不自行生成 heartbeat；
- Agent 如发送 heartbeat，Blog 原样转发。

Blog 固定 `Content-Type: text/event-stream` 等必要 SSE 传输头；Agent 可通过 gRPC initial metadata 提供额外 HTTP response headers。Blog 不转发 gRPC trailers；非法 header 和 HTTP 禁止的 hop-by-hop header 不得写入响应。

Agent 流在 HTTP 200 建立后断开且没有终态事件时，Blog 直接关闭 SSE；前端调用 `GET Run` 获取最新状态。Blog 不自行生成 `stream_error` 事件。

## 6. AgentControlService gRPC

Proto source of truth 位于：

```text
contracts/proto/scyg/agent/v1/
```

package 保持：

```protobuf
package scyg.agent.v1;
```

服务只保留：

```protobuf
service AgentControlService {
  rpc CreateRun(CreateRunRequest) returns (Run);
  rpc GetRun(GetRunRequest) returns (Run);
  rpc StreamRunEvents(StreamRunEventsRequest) returns (stream RunEventFrame);
  rpc ResumeRun(ResumeRunRequest) returns (Run);
  rpc CancelRun(CancelRunRequest) returns (Run);
}
```

请求最小字段：

```text
CreateRun:
  user_id
  idempotency_key
  capability
  json_payload

GetRun:
  user_id
  run_id

StreamRunEvents:
  user_id
  run_id
  optional after_event_id

ResumeRun:
  user_id
  run_id
  idempotency_key
  interaction_id
  decision
  optional payload_json

CancelRun:
  user_id
  run_id
  idempotency_key
```

约束：

- `capability` 使用 Proto enum；
- JSON payload 使用原始 `bytes`；
- `run_id` 使用 URL-safe ASCII 字符串；
- `interaction_id` 使用不透明字符串；
- `Run` 使用统一 Proto message；
- 时间字段使用 RFC 3339 字符串；
- `result_json` 使用 optional bytes，缺失表示没有结果，存在时可以是任意合法 JSON；
- 不包含 Recipe、模型、Tool、Skill、Runtime、HITL 选择字段；
- `x-request-id` 只通过 gRPC metadata 传递，用于日志，不参与业务。

Create、Resume、Cancel 的最终幂等由 Agent 持久化保证，作用域为：

```text
user_id + idempotency_key
```

三类 RPC 共用该作用域。只持久化成功操作，成功幂等记录保留 24 小时。重复 key 返回绑定 Run 的当前状态；请求内容不做摘要比较，因此调用方必须为每个新的逻辑操作生成新的 UUID v4。

## 7. BlogContentService gRPC

Proto source of truth 位于：

```text
contracts/proto/scyg/blog/v1/
```

服务文件由 `blog_tool_service.proto` clean cutover 为：

```text
blog_content_service.proto
```

服务名：

```protobuf
service BlogContentService {
  rpc SearchArticles(SearchArticlesRequest) returns (SearchArticlesResponse);
  rpc GetArticle(GetArticleRequest) returns (GetArticleResponse);
  rpc ListTags(ListTagsRequest) returns (ListTagsResponse);
  rpc ListArticleTypes(ListArticleTypesRequest) returns (ListArticleTypesResponse);
  rpc CreateArticle(CreateArticleRequest) returns (CreateArticleResponse);
  rpc UpdateArticle(UpdateArticleRequest) returns (UpdateArticleResponse);
  rpc PublishArticle(PublishArticleRequest) returns (PublishArticleResponse);
  rpc ArchiveArticle(ArchiveArticleRequest) returns (ArchiveArticleResponse);
}
```

规则：

- 所有请求直接携带 `user_id`；
- Blog 根据该用户复用现有资源权限；
- 不查询 Agent capability、Recipe、Tool 或 HITL；
- 查询使用管理端语义，可读取用户有权限的草稿、已发布和归档资源；
- `SearchArticles` 是现有关键词、状态、文章类型、标签、分页和排序查询，不是语义搜索；
- 资源 ID 使用 `int64`；
- Article 状态使用 Proto enum；
- Proto 字段使用 snake_case；
- Proto DTO 独立于 REST DTO，但业务字段语义复用 Blog 现有规则。

方法范围：

- `CreateArticle` 复用现有创建字段，允许创建草稿或直接发布；
- `UpdateArticle` 使用局部更新、proto3 optional 字段和 `expected_version`，不修改生命周期状态；
- `PublishArticle`、`ArchiveArticle` 使用 `article_id`、`expected_version` 和现有状态迁移规则；
- 不提供 `DeleteArticle`；
- taxonomy 只读，不提供标签或分类写入。

所有写 RPC 必须携带全局唯一 UUID v4 `operation_id`。Blog PostgreSQL 只持久化成功写入的幂等记录，保留 24 小时。reservation、业务写入和成功记录必须在同一数据库事务内；唯一约束保证并发重复请求只实际执行一次。幂等记录只保存关联 `article_id`，重复请求重新返回该文章当前状态；资源已经删除时返回 `NOT_FOUND`，不重新执行原写入。

## 8. 错误和权限

Blog HTTP 复用当前 RFC 9457 Problem Details，并增加可选的非空 `code` 字段。Blog 只做 gRPC canonical status 到 HTTP 的统一映射：

| gRPC status | HTTP status |
| --- | ---: |
| `INVALID_ARGUMENT` | 400 |
| `UNAUTHENTICATED` | 401 |
| `NOT_FOUND` | 404 |
| `PERMISSION_DENIED` | 404 |
| `ALREADY_EXISTS` | 409 |
| `ABORTED` | 409 |
| `FAILED_PRECONDITION` | 409 |
| `RESOURCE_EXHAUSTED` | 429 |
| `UNAVAILABLE` | 503 |
| `DEADLINE_EXCEEDED` | 503 |
| 其他 | 500 |

Agent 在 gRPC status details 中提供结构化公开错误 `{code, message}`；Blog 读取该 detail，缺失时使用本地通用消息。不向浏览器透传 Provider 原始错误、数据库信息、密钥或敏感 Tool 参数。

Agent owner 校验失败、Run 不存在和跨用户访问统一返回 `NOT_FOUND`，由 Blog 映射为 HTTP 404。

## 9. 部署、配置和生命周期

Blog 继续使用当前 YAML + `SCYG_` 环境变量配置系统。新增配置包括：

- Agent 集成开关；
- Agent gRPC target；
- BlogContentService gRPC listen address；
- unary RPC timeout；
- SSE stream idle timeout；
- gRPC shutdown timeout。

`enabled=true` 时配置缺失或非法导致 Blog 启动失败。Blog 创建非阻塞 Agent gRPC client，不要求 Agent 在启动时在线；Agent 离线时 Agent HTTP API 返回 503，不影响 Blog `/ready`。

BlogContentService：

- 默认仅绑定 `127.0.0.1`；
- 容器部署显式配置为内部网络地址；
- 默认 Compose 不发布 gRPC 宿主机端口；
- listener 绑定失败导致 Blog 启动失败；
- 运行中 listener 异常退出会撤回 readiness 并触发 Blog 整体有界关闭。

Blog 和 Agent 都注册标准 `grpc.health.v1.Health`，不新增自定义健康 RPC。

仓库根目录提供集成 Compose，编排 Blog、Agent 及其数据库；`backend/compose.yaml` 和 `agent/compose.yaml` 可继续用于独立开发。

优雅关闭顺序：

```text
撤回 readiness
→ 排空 HTTP/SSE
→ GracefulStop BlogContentService
→ 停止幂等清理任务
→ 关闭 Agent gRPC ClientConn
→ 关闭数据库和遥测
```

## 10. Proto 生成和兼容

共享 Proto 使用当前 `contracts/` 目录和 Buf：

- Buf 负责 lint、breaking 和 generate；
- Go 生成到 `backend/internal/generated/proto/`；
- Python 生成到 `agent/src/scyg_agent/generated/proto/`；
- 生成物不提交；
- 本地标准测试和构建入口自动执行生成；
- Docker 以仓库根为 build context，在构建阶段执行生成；
- CI 从干净源码生成后再编译和测试。

本次是 clean cutover。删除旧 RPC、旧 service、旧生成代码、旧客户端、旧测试和旧配置，不保留兼容别名、双读双写或旧 Runtime 入口。Proto 已发布字段编号和枚举值仍遵守现有兼容规则；删除的字段和枚举值必须按 Buf 规则 reserve。

## 11. 验收标准

### HTTP 和创建

- 8 个路由全部使用 `/api/v1`；
- 四个创建接口固定 capability；
- Blog 不接受或解释 Recipe、模型、Tool、Skill、Runtime 参数；
- 非法 Content-Type、空 body、非法 JSON、超过 1 MiB 和非法 ID 返回明确错误；
- 同一用户同一 Idempotency-Key 的成功重试不会创建第二个 Run；
- Blog 不引入 Redis。

### 权限

- Blog 验证用户 JWT 并传递 `user_id`；
- Agent 对每个 Run RPC 校验 owner；
- Run 不存在和非 owner 统一返回 404；
- Agent→Blog 资源访问复用 Blog 现有用户权限；
- Agent 服务身份不扩大用户权限。

### 查询、Resume 和 Cancel

- `GET Run` 返回统一外层和 Agent 结构化结果；
- `result` 和 `failure` 可以同时存在；
- `pendingInteraction` 可在断线后通过 GET Run 恢复；
- Resume 同一 key 重试不会重复处理；已处理 interaction 不可被新 key 覆盖；
- Cancel 后 Agent 不再发起新的 Tool 调用；在途 Blog 写入可以完成但不改变 cancelled 终态；
- 已完成 Run 不可被 Cancel 覆盖。

### SSE

- SSE 使用 Bearer JWT；
- Blog 在返回 200 前完成 GetRun owner 校验；
- Last-Event-ID 原样转发为不透明游标；
- Agent SSE frame 原样转发；
- Blog 不生成业务事件、不排序、不保存事件；
- 流断开后前端可以通过 GET Run 获取最终状态。

### BlogContentService

- 8 个业务 RPC 按用户权限工作；
- 查询复用现有管理端分页、筛选和排序规则；
- 写入操作使用数据库事务和全局 `operation_id` 幂等；
- 并发重复写入只产生一次业务写入；
- 发布、归档和更新遵守 `expected_version`；
- 不提供删除或 taxonomy 写入 RPC。

### 生命周期和交付

- Agent 不在线不阻止 Blog 启动；
- BlogContentService 监听失败阻止 Blog 启动；
- 标准 gRPC Health 可用；
- Proto 可从干净源码生成；
- 旧协议、旧生成物、旧客户端和旧配置不残留；
- HTTP、gRPC、SSE、migration、bootstrap、Compose 和文档均与本文一致。
