# SCYG Agent 交接文档

> 交接对象：在新电脑上继续 SCYG.Blog Agent Harness 工作的开发者或 Coding Agent
>
> 文档目的：提供当前事实、任务计划、验证证据和下一步入口。本文是交接索引，不替代源码、合同、正式服务文档和 scratch 计划。
>
> 当前状态：Blog↔Agent 主要合同与代码已执行 clean cutover：内部五 RPC、统一 Run 快照、Blog 浏览器 JWT/HTTP/SSE 入口、Recipe AgentRunner 唯一生产路径与 BlogContent 只读工具。真实验证、代码审查和合同基线的观察结果统一见 `.scratch/agent-api-requirements/workflow.md`；本文不作为部署证明，业务需求以 `plan/SCYG Agent后端API需求文档.md` 为准。

## 1. 新电脑的阅读顺序

先读取以下规则和正式文档：

1. 根 [`AGENTS.md`](../AGENTS.md)，其中包含 Agent 开发约束。
2. Agent 正式入口 [`docs/services/agent/README.md`](../docs/services/agent/README.md)。
3. 当前架构 [`docs/services/agent/architecture.md`](../docs/services/agent/architecture.md)。
4. 开发与验证 [`docs/services/agent/development.md`](../docs/services/agent/development.md)。
5. 运行手册 [`docs/services/agent/operations.md`](../docs/services/agent/operations.md)。
6. 流式合同 [`docs/services/agent/streaming.md`](../docs/services/agent/streaming.md)。
7. 本文引用的 scratch 计划和工作流记录。

正式文档描述当前实现；当前需求与任务记录维护 cutover 目标及验收状态。历史 scratch 计划不替代现行源码，也不代表当前仍保留旧入口。

## 2. 权威来源和事实优先级

发生冲突时按以下顺序核对：

```text
当前源码、Proto、配置和测试
    > 正式 docs/
    > .scratch 中的任务记录、日志和验证证据
    > 历史计划、审查报告和旧文档
    > 旧聊天记录
```

具体归属：

| 内容 | 权威来源 |
| --- | --- |
| Agent 当前行为 | `agent/src/`、`agent/tests/` |
| Agent gRPC 合同 | `contracts/` 与生成漂移检查 |
| Agent 配置与部署 | `agent/pyproject.toml`、`agent/compose.yaml`、`agent/.env.example`、`agent/scripts/` |
| 当前运行与验证说明 | `docs/services/agent/` |
| 本轮业务需求 | `plan/SCYG Agent后端API需求文档.md` |
| 本轮已执行验证与任务状态 | `.scratch/agent-api-requirements/workflow.md` 及对应证据 |
| 历史计划与审查 | `.scratch/documentation-migration/`、`.scratch/agent-p4/`；仅作历史索引 |

不要依赖 `.omo/` 或旧对话替代上述来源；`.omo/` 仅用于查找既有 QA 历史。

## 3. 当前已确认状态

### 已落地

当前工作区已包含以下能力或基础设施：

- Agent Python 3.12 项目和锁定依赖。
- Recipe、Capability、输入输出合同和结构化结果边界。
- AgentRunner 的执行结果边界，并已接入 Worker 的成功、失败和终态提交路径。
- Agent 侧 Blog 只读 Tool Gateway、封闭目录、Recipe allowlist 与可信 runtime context；Search 通过 `BlogContentService` 读取管理端可见的草稿、已发布和归档文章，排除已删除文章，四 capability 均不注册 Blog 写 Tool，见[架构](../docs/services/agent/architecture.md#blog-只读工具)。
- Redis Stream 适配、TTL 与 attempt fencing 服务 Worker 瞬时流；公开 SSE 从 PostgreSQL 持久事件编码，使用 Run-bound opaque cursor，由 Blog 透明转发并等待 subscription-ready。
- PostgreSQL Run、事件、命令、交互、审计、lease 和终态持久化代码；Create/Resume/Cancel 共享 owner/key 的 24 小时成功重放，失败不占 key。
- LangGraph checkpoint 与 Agent truth 共用应用数据库、应用账号和 DSN，schema 不是账号隔离边界。
- AgentControl 五 RPC：CreateRun、GetRun、StreamRunEvents、ResumeRun、CancelRun；四 unary 返回统一 Run。浏览器只连接 Blog JWT/HTTP/SSE 入口，Agent HTTP 仅 health，内部 gRPC 无 service JWT。
- capability 由客户端选择，Recipe/version 与执行配置由服务器冻结；Recipe AgentRunner 是唯一生产执行路径，旧 RuntimeRouter/producer 与 `runtimes/` 执行代码已删除。
- Python 绑定统一位于 `generated/proto`，并按 wheel/sdist 与 Docker 构建配置纳入安装产物。

### 历史验证记录

静态门禁命令：

```powershell
uv run --locked --project agent python agent/scripts/qa_agent.py --mode static
```

历史交接结果（不代表本机复核通过）：

- contracts：通过，生成合同为最新状态；
- Ruff check：通过；
- Ruff format：通过；
- basedpyright：`0 errors, 0 warnings, 0 notes`；
- pytest：`700 passed, 108 skipped`；
- 覆盖率、no-excuse、scope scan、secret scan、dependency audit 和 deployment contracts：通过；
- 成功标记：`agent QA: STATIC PASS`。

上述历史成功标记不能替代本轮验收证据；本轮真实观察统一维护在[当前工作流](../.scratch/agent-api-requirements/workflow.md)，操作入口见[开发指南](../docs/services/agent/development.md)。

### 历史环境阻断与未验收范围

历史完整门禁曾在 T32 前置检查因缺少 Docker 退出；以下为旧环境记录，不是当前环境结论：

```text
T32 topology prerequisites: missing tool(s): docker
```

该次运行没有验证以下内容；本轮是否已执行及其结果以当前工作流为准，不能从历史记录推断：

- Docker Compose 配置和启动；
- PostgreSQL/Redis 真实拓扑；
- Agent setup、真实 migration 和 checkpoint setup/read/write；
- Provider 请求和真实 Agent 执行；
- Agent gRPC/HTTP 真实联调；
- T34 E2E；
- 浏览器 AI 链路。

没有 `SCYG_TEST_CONFIG_FILE` 时，PostgreSQL 端点测试会跳过；这不等于端点验收通过。

## 4. 历史计划中的未完成目标

以下列表保留旧交接阶段判断，不作为本轮当前实现清单；后端合同、AgentRunner/Worker、旧 Runtime 删除及 Blog 入口已发生 cutover，是否验收通过必须另查本轮记录：

- 当时待完成的 P4 后端 `BlogContentService`、草稿/归档管理查询和真实跨服务联调；当时 Agent 侧完成部分不能视为整体 P4 验收通过。
- `submit_outline_for_approval` 及完整 HITL 恢复；
- 完整 Search、Write、Polish、Chat 四类 Agent 的真实主路径；
- checkpoint、interaction、command 三方 ApprovalReconciler；
- AgentRunner/Worker 的完整新路径切换；
- P10 真实拓扑和 readiness 验收；
- P11 删除 `runtimes/`、旧 Registry/Router/Adapter 和旧执行路径；
- Blog API、Blog↔Agent 调用方和前端 AI 页面接通。

当前旧 Runtime 已删除，不再按上述历史阶段指引保留或恢复。不得添加兼容入口、双协议、双写路径或未经授权的权限系统；真实验收以本轮工作流收口。

## 5. 继续工作的计划入口

本轮继续工作入口为[Agent API 集成工作流](../.scratch/agent-api-requirements/workflow.md)与[权威业务需求](SCYG%20Agent后端API需求文档.md)。[Agent P4 工作流](../.scratch/agent-p4/workflow.md)仅为历史记录；以下历史材料曾未包含在交接工作区，不能据此推断当前实现状态：

- [Agent Harness 实施计划](../.scratch/documentation-migration/preserved/agent/agent-harness-implementation-plan.md)
- [框架重构计划审查报告](../.scratch/documentation-migration/preserved/plan/SCYG Agent第一版框架重构实施计划-审查报告.md)
- [任务工作流记录](../.scratch/documentation-migration/workflow.md)

历史计划阶段摘要（保留旧交接时判断，不代表本轮状态）：

| 阶段 | 内容 | 历史交接判断 |
| --- | --- | --- |
| P0 | 框架依赖冻结和构造探针 | 已完成主要冻结，真实 checkpoint 验收仍依赖拓扑 |
| P1 | 新合同和配置 | 已实现主要合同、配置和生成绑定 |
| P2 | 数据库迁移 | 源码已有迁移和持久化改动，完整真实数据库验收未完成 |
| P3 | Redis Streams | 适配器和测试已存在，真实 Redis 拓扑验收未完成 |
| P4 | Tool Gateway 与 Blog 只读 Tool | Agent 侧已实现并本地验证；后端合同、管理查询与真实联调待实施 |
| P5–P10 | 模型策略、四类 Agent、HITL、Worker 切换、接入和组合根 | 未完成或未完成真实验收 |
| P11 | 删除旧 Runtime | 必须在新路径完成并验证后进行 |
| P12 | 日常主路径验证和文档同步 | 当前仅完成部分文档同步，动态主路径未收口 |

## 6. 历史 P4 调查边界

以下为 P4 开始前的历史调查与范围记录，不作为当前操作步骤：

1. `agent/src/scyg_agent/agents/recipes.py` 的 Recipe、模型和工具声明边界。
2. `agent/src/scyg_agent/agents/production.py` 的 AgentRunner 图构造和调用入口。
3. `agent/src/scyg_agent/adapters/blog_grpc/` 的 Blog gRPC Client、命令合同和现有权限校验。
4. 当时的 `runtimes/profiles.py` ToolPermission 曾用于辨别旧 Runtime 画像；该旧入口已删除，当前授权以 Recipe 和 Tool Gateway 为准。
5. `agent/src/scyg_agent/domain/ports/` 中 Tool、审计、操作幂等和数据范围相关接口。
6. `agent/tests/` 中 Blog gRPC、Tool execution、Recipe、Worker 和生产构造测试，复用现有测试模式。
7. P4 的验收条件（单用户个人博客，登录用于管理文章）：
   - Research 能搜索和读取管理端可见的草稿、已发布和归档文章，排除已删除文章；
   - 复用 Blog 现有登录与管理权限，不新增文章作者隔离、多租户或权限矩阵；
   - checkpoint 中伪造的未授权 Tool call 在执行时被拒绝；
   - 第一版 Tool Catalog 不注册 Blog 写 Tool。

P4 只实现计划要求的 Blog 只读主路径和基本失败结果；不顺手实现外部 Web 搜索、MCP、Blog 写 Tool、复杂权限矩阵或极端故障注入。

Blog 当前合同以[已确认的后端 API 需求](SCYG%20Agent后端API需求文档.md#7-blogcontentservice-grpc)及根 Proto 为准：`BlogContentService`、显式 `user_id`、`int64` 资源 ID 和管理端查询语义。旧 P4 曾仅实施 Agent 侧已发布文章读取，此为历史边界；本轮已切换合同与调用方，不保留旧工具服务的兼容入口。源码切换不等同于完整管理查询场景已通过真实验收。

## 7. 新电脑的最短接续命令

在当前克隆的仓库根目录执行，不沿用历史电脑的绝对路径：

```powershell
git status --short

uv run --locked --project agent python agent/scripts/qa_agent.py --mode static
```

修改 `agent/` 前先阅读 `agent/AGENTS.md`。接续本轮先读取当前工作流、需求、源码、合同和测试，不按旧 P4 阶段重新保留 Runtime 或旧端点。

具备 Docker、Compose、PostgreSQL、Redis、Blog 内部 gRPC 和 Provider 配置后，再执行完整门禁：

```powershell
uv run --locked --project agent python agent/scripts/qa_agent.py --mode full
```

完整门禁失败时记录实际命令、退出码和第一失败阶段；不要将缺少依赖、配置展开或静态检查结果写成动态验收通过。

## 8. 交接时的修改边界

- 先保留 `git status --short` 中已有用户修改，不回退、不覆盖、不批量格式化无关文件。
- 修改合同源时运行对应生成和漂移检查，不手工修改生成绑定。
- 修改配置、部署、接口、流式行为或状态语义时同步更新对应 `docs/services/agent/` 文档。
- 计划、草稿、原始输出、验证日志和交接记录放在 `.scratch/<任务标识>/`；不要把过程日志写入正式 `docs/`。
- 正式文档只写已确认且有源码、合同、配置或真实运行证据支持的内容；目标和未完成工作必须标明状态。
- 不提交、推送、部署或写入 Wiki，除非获得单独授权。

## 9. 交接检查清单

- [ ] 已读取根和 Agent `AGENTS.md`。
- [ ] 已读取 Agent 正式架构、开发、运行和流式文档。
- [ ] 已读取当前业务需求、集成工作流，并区分历史计划与当前状态。
- [ ] 已运行 `git status --short` 并保留既有修改。
- [ ] 已运行 Agent 静态门禁并记录真实结果。
- [ ] 已确认当前 cutover 的源码入口、接口边界和验收条件。
- [ ] 已续接 `.scratch/agent-api-requirements/workflow.md`。
- [ ] 已按实际行为及真实观察更新正式文档，不以历史 QA 宣称当前通过。
- [ ] 已确认动态门禁是否具备 Docker、数据库、Redis 和 Provider 前置条件。
