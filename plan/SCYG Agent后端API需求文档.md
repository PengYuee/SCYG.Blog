# SCYG Agent 后端 API 需求文档（修订版）

## 1. 文档状态

- 状态：已确认设计，源码已实施；验证与部署状态另查交接记录。
- 读者：Blog 后端、Agent 服务和前端 API 实现者。
- 维护位置：本文维护 Blog 对外 HTTP/SSE 边界和 Blog↔Agent 集成合同；Agent 内部 Run、Recipe、Tool、HITL、模型、事件持久化和 checkpoint 由 Agent 项目文档维护。
- 版本：v1。

本文维护业务合同，不作为运行或上线证明。当前 `backend/` 已接入 Agent HTTP/SSE、双向 gRPC 与 BlogContentService；实现入口见[后端交接](后端项目交接文档.md)，未完成验证的部分不得视为已上线能力。

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

请求体遵循[第 8 节](#8-错误和权限)的 JSON 传输校验。顶层可以是任意 JSON 值，包括 `null`；Blog 不解析业务字段，不向 body 注入 `user_id`，具体 payload 由 Agent 校验和解释。

四个路径固定映射为以下 capability，客户端不能提交或覆盖：

| 路径 | capability |
| --- | --- |
| `/api/v1/ai/search` | `search` |
| `/api/v1/ai/write` | `write` |
| `/api/v1/ai/polish` | `polish` |
| `/api/v1/ai/chat` | `chat` |

Blog 将原始 JSON bytes、`user_id`、固定 capability 和 `Idempotency-Key` 发送给 Agent。

Blog 不提供 Recipe、模型、Tool、Skill、Runtime 或 HITL 的独立选择参数，不从 body 提取这些字段；body 中出现同名字段仍原样传给 Agent，由 Agent 校验。body 不能覆盖路由固定的 capability。

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

`runId` 不得等于 `.` 或 `..`，以免 URL 路径规范化改变请求目标。所有 Run HTTP 路径共用上述约束。

Blog 将当前 `user_id` 和 `runId` 传给 Agent。Agent 负责 Run owner 校验；Run 不存在和当前用户不是 owner 均映射为 HTTP 404。

### 3.4 Resume

```http
POST /api/v1/runs/{runId}/resume
Authorization: Bearer <user-jwt>
Content-Type: application/json
Idempotency-Key: <uuid-v4>
```

请求体遵循[第 8 节](#8-错误和权限)的 JSON 传输校验，且顶层必须是 JSON object：

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

重试遵循[第 6 节](#6-agentcontrolservice-grpc)的幂等窗口与重放规则。interaction 已处理后，使用新的 key 再次 Resume 返回冲突；不允许覆盖已经处理的决定。

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

SSE 建立顺序：

1. Blog 调用 `GetRun(user_id, run_id)` 完成授权，成功后调用 `StreamRunEvents`。
2. Agent 再次校验 owner、游标和订阅条件，通过后显式发送 gRPC initial metadata，作为订阅建立成功信号。
3. Blog 收到该信号后才提交 HTTP 200 和 SSE headers，不等待首条业务事件。

建立信号前收到 gRPC 错误，Blog 按[第 8 节](#8-错误和权限)返回 Problem Details，不提前提交 200；提交 200 后的流结束按本节断线规则处理。

Agent 的流消息是已经编码好的完整 SSE frame bytes。Blog：

- 原样写入 `event`、`id`、`data` 和注释行；
- 不生成业务序号；
- 不重新排序或重建事件；
- 不保存完整事件；
- 每条消息写入后 flush；
- 不自行生成 heartbeat；
- 启用有限 SSE idle timeout 时，Agent 必须以短于该 timeout 的间隔发送 heartbeat，Blog 原样转发。

Blog 自行设置 `Content-Type: text/event-stream` 等必要 SSE 传输头，不将 gRPC metadata 或 trailers 映射为 HTTP response headers；initial metadata 仅用于确认订阅建立。

SSE idle timeout 自订阅建立起，按连续未收到任何 Agent frame 的时长计算；业务事件和 heartbeat frame 都重置计时。

HTTP 200 提交后，Agent 流结束、异常断开或触发 idle timeout 时，Blog 直接关闭 SSE，不生成 `stream_error` 事件。前端调用 `GET Run` 获取当前状态：非终态重新订阅，终态停止订阅；Blog 不解析 SSE frame 判断业务终态。

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
- `run_id` 遵循[第 3.3 节](#33-查询-run)的长度、字符和路径安全约束；
- `interaction_id` 使用不透明字符串；
- `Run` 使用统一 Proto message；
- 时间字段使用 RFC 3339 字符串；
- `result_json` 使用 optional bytes，缺失表示没有结果，存在时可以是任意合法 JSON；
- 请求消息不提供 Recipe、模型、Tool、Skill、Runtime 或 HITL 的独立选择字段；Run 响应中的 Recipe 标识只读；
- `x-request-id` 只通过 gRPC metadata 传递，用于日志，不参与业务。

Create、Resume、Cancel 的最终幂等由 Agent 持久化保证，作用域为：

```text
user_id + idempotency_key
```

三类 RPC 共用该作用域，规则如下：

- 只持久化成功操作，幂等记录自操作成功提交时起保留 24 小时；有效期内重复 key 返回记录绑定 Run 的当前状态，不重复执行操作。
- 重放仍须满足 Run owner 校验，不因命中幂等记录绕过权限。
- 到期后同一 key 可能按新请求处理，但仍须遵守 interaction 已处理和 Run 终态等业务约束；幂等保证不扩展到记录有效期之外。
- 不比较请求内容摘要，也不增加 RPC 类型或目标 ID 的冲突检测。调用方必须为每个新的逻辑操作生成新的 UUID v4，仅重试同一操作时沿用原 key，不得跨 capability、RPC、Run 或 interaction 复用。

误用示例：Create 使用 key K 创建 Run A 后，若用 K Cancel Run B，命中原成功记录时会返回 A 的当前状态，不会取消 B。调用方不得把这样的重放视为新操作已执行。

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
- `UpdateArticle` 使用局部更新和 `expected_version`，不修改生命周期状态；标量字段使用 proto3 `optional`；
- `PublishArticle`、`ArchiveArticle` 使用 `article_id`、`expected_version` 和现有状态迁移规则；
- 不提供 `DeleteArticle`；
- taxonomy 只读，不提供标签或分类写入。

标签局部更新使用具有 presence 的 message 包装列表，不直接以 `repeated` 字段表示补丁：未提供该消息时保持标签不变，消息存在且列表为空时清空标签，非空列表替换现有标签。至少提供一个可更新字段，字段语义复用现有 ArticlePatch；不引入通用 Patch 框架。

所有写 RPC 必须携带全局唯一 UUID v4 `operation_id`，并遵循以下事务与重放规则：

- reservation、业务写入和成功幂等记录必须在同一 PostgreSQL 事务内；唯一约束保证记录有效期内的并发重复请求只实际写入一次，失败事务不留下成功记录。
- 成功记录自操作成功提交时起保留 24 小时，业务结果只保存关联 `article_id`；到期后同一 ID 可能按新请求处理，不保证窗口外的创建操作不会再次执行。
- 命中成功记录后，按当前 `user_id` 校验记录绑定文章的现时读取权限，通过后返回该文章当前状态；不再次执行写入，也不将原 `expected_version` 与当前版本比较。资源已删除时返回 `NOT_FOUND`，不重新执行原写入。
- 未命中成功记录时，才执行正常写入授权、`expected_version` 校验和事务写入；并发请求在事务内发现已有成功记录时同样按重放规则处理。
- 调用方跨所有写 RPC 为每个新的逻辑操作生成新的 ID，仅同一操作的重试沿用原 ID；窗口外不得盲目补发创建操作，应先核实已有结果。

误用示例：用 ID O 创建文章 X 后，若以 O 更新文章 Y，命中原成功记录时会返回 X 的当前状态，不会更新 Y。服务端不增加请求摘要或操作类型/目标 ID 冲突检测，调用方必须避免复用。

## 8. 错误和权限

Blog HTTP 复用当前 RFC 9457 Problem Details，并增加可选的非空 `code` 字段。

创建 Run 和 Resume 共用 JSON 传输规则：`Content-Type` 必须为 `application/json`，允许 `charset` 参数；body 必须存在且为合法 JSON，最大 1 MiB。顶层形状及字段约束分别由第 3.2、3.4 节规定。Blog 本地校验失败不调用 Agent，直接返回 Problem Details：

| 本地校验失败 | HTTP status |
| --- | ---: |
| Content-Type 缺失或不是支持的 JSON 媒体类型 | 415 |
| body 超过 1 MiB | 413 |
| 空 body、非法 JSON、不符合接口结构的请求或非法 ID | 400 |

来自 Agent 的 gRPC 错误统一映射为：

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
- HTTP/SSE shutdown timeout；
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
→ 有界排空 HTTP/SSE，超时取消剩余流及对应 Stream RPC
→ GracefulStop BlogContentService，超时强制 Stop
→ 停止幂等清理任务
→ 关闭 Agent gRPC ClientConn
→ 关闭数据库和遥测
```

HTTP/SSE 共用一个排空期限，持续收到业务事件或 heartbeat 不延长该期限；到期取消未结束的请求和对应 RPC 后继续关闭。BlogContentService 使用 gRPC shutdown timeout 限制 GracefulStop，不能无限等待活跃调用。

## 10. Proto 生成和兼容

共享 Proto 使用当前 `contracts/` 目录和 Buf：

- Buf 负责 lint、breaking 和 generate；
- Go 生成到 `backend/internal/generated/proto/`；
- Python 生成到 `agent/src/scyg_agent/generated/proto/`；
- 生成物不提交；
- 本地标准测试和构建入口自动执行生成；
- Docker 以仓库根为 build context，在构建阶段执行生成；
- CI 从干净源码生成后再编译和测试。

本次是 clean cutover。删除旧 RPC、旧 service、旧生成代码、旧客户端、旧测试和旧配置，不保留兼容别名、双读双写或旧 Runtime 入口。

兼容检查策略：

- 本次旧合同到新合同的破坏性变更须单独审阅确认，Blog 与 Agent 同步切换；不要求这些已确认的删除或 RPC 签名变更通过相对旧合同的 Buf `FILE` 检查。
- 完成切换后，以新合同建立后续 breaking 基线，CI 继续执行 Buf 兼容检查，不永久关闭或放宽检查。
- 已发布的字段编号和枚举值不得复用于其他语义；删除字段和枚举值须 reserve 编号与名称。reserve 防止复用，不代表 RPC、service 或生成 API 的删除满足 `FILE` 兼容。

## 11. 验收标准

### HTTP 和创建

- 8 个路由全部使用 `/api/v1`；
- 四个创建接口固定 capability；
- Blog 不从 body 提取 Agent 选择参数，原始 JSON 透传与固定 capability 符合第 3.2 节；
- 创建与 Resume 覆盖第 8 节的媒体类型、结构及大小边界校验；所有 Run 路径校验 ID 字符、长度并拒绝 `.`、`..`；
- 同一用户同一 Idempotency-Key 在成功记录有效期内重试不会创建第二个 Run；
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
- Resume 在成功幂等记录有效期内重试不会重复处理；已处理 interaction 即使记录到期也不可被新操作覆盖；
- Cancel 后 Agent 不再发起新的 Tool 调用；在途 Blog 写入可以完成但不改变 cancelled 终态；
- 已完成 Run 不可被 Cancel 覆盖。

### SSE

- SSE 使用 Bearer JWT；
- Blog 在返回 200 前完成 GetRun owner 校验并收到 Agent 的订阅建立信号；建立前错误返回 Problem Details，不等待首条业务事件；
- Last-Event-ID 原样转发为不透明游标；
- Agent SSE frame 原样转发；
- Blog 不生成业务事件、不排序、不保存事件；
- Blog 不透传 gRPC metadata/trailers 为 HTTP headers，heartbeat、idle timeout 按第 5 节配合；
- 流断开后前端通过 GET Run 获取当前状态，非终态重新订阅，终态停止；

### BlogContentService

- 8 个业务 RPC 按用户权限工作；
- 查询复用现有管理端分页、筛选和排序规则；
- 写入操作使用数据库事务和全局 `operation_id` 幂等；
- 成功记录有效期内的并发重复写入只产生一次业务写入；
- 成功重放不因文章版本推进而冲突，也不绕过当前用户的读取权限；
- 新更新、发布、归档操作遵守 `expected_version`，局部更新区分未提供标签列表与显式空列表；
- 不提供删除或 taxonomy 写入 RPC。

### 生命周期和交付

- Agent 不在线不阻止 Blog 启动；
- BlogContentService 监听失败阻止 Blog 启动；
- 标准 gRPC Health 可用；
- Proto 可从干净源码生成；
- 活跃 SSE 或 gRPC 调用不会使关闭超过对应的排空或停止期限；
- 已确认的协议破坏性变更同步切换，后续 Buf breaking 检查以新合同为基线；
- 旧协议、旧生成物、旧客户端和旧配置不残留；
- HTTP、gRPC、SSE、migration、bootstrap、Compose 和文档均与本文一致。
