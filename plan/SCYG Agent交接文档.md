# SCYG Agent 交接文档

> 交接对象：在新电脑上继续 SCYG.Blog Agent Harness 工作的开发者或 Coding Agent
>
> 文档目的：提供当前事实、任务计划、验证证据和下一步入口。本文是交接索引，不替代源码、合同、正式服务文档和 scratch 计划。
>
> 当前状态：Agent 静态门禁已通过；真实 PostgreSQL、Redis、Provider、Compose 和浏览器端到端验收尚未完成。下一实施阶段为 P4：Tool Gateway 与 Blog 只读 Tool。

## 1. 新电脑的阅读顺序

先读取以下规则和正式文档：

1. 根 [`AGENTS.md`](../AGENTS.md)，其中包含 Agent 开发约束。
2. Agent 正式入口 [`docs/services/agent/README.md`](../docs/services/agent/README.md)。
3. 当前架构 [`docs/services/agent/architecture.md`](../docs/services/agent/architecture.md)。
4. 开发与验证 [`docs/services/agent/development.md`](../docs/services/agent/development.md)。
5. 运行手册 [`docs/services/agent/operations.md`](../docs/services/agent/operations.md)。
6. 流式合同 [`docs/services/agent/streaming.md`](../docs/services/agent/streaming.md)。
7. 本文引用的 scratch 计划和工作流记录。

正式文档描述当前实现；scratch 计划描述未完成目标。不得把计划目标当成已实现能力。

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
| 未完成目标和阶段验收 | `.scratch/documentation-migration/preserved/agent/agent-harness-implementation-plan.md` |
| 设计审查风险 | `.scratch/documentation-migration/preserved/plan/SCYG Agent第一版框架重构实施计划-审查报告.md` |
| 已执行验证与任务状态 | `.scratch/documentation-migration/workflow.md` 及对应日志 |

不要依赖 `.omo/` 或旧对话替代上述来源；`.omo/` 仅用于查找既有 QA 历史。

## 3. 当前已确认状态

### 已落地

当前工作区已包含以下能力或基础设施：

- Agent Python 3.12 项目和锁定依赖。
- Recipe、Capability、输入输出合同和结构化结果边界。
- AgentRunner 的执行结果边界，并已接入 Worker 的成功、失败和终态提交路径。
- Redis Stream 适配、Stream ID、TTL、attempt fencing、SSE Redis 游标和流过期语义。
- PostgreSQL Run、事件、命令、交互、审计、lease 和终态持久化代码。
- LangGraph checkpoint 适配代码和部署初始化路径。
- Agent gRPC 控制面、HTTP/SSE、命令与取消入口。
- 旧 SIMPLE/DEEP RuntimeRouter 仍在源码中，与 Recipe AgentRunner 同时存在。

### 已验证

最近一次静态门禁命令：

```powershell
uv run --locked --project agent python agent/scripts/qa_agent.py --mode static
```

结果：

- contracts：通过，生成合同为最新状态；
- Ruff check：通过；
- Ruff format：通过；
- basedpyright：`0 errors, 0 warnings, 0 notes`；
- pytest：`700 passed, 108 skipped`；
- 覆盖率、no-excuse、scope scan、secret scan、dependency audit 和 deployment contracts：通过；
- 成功标记：`agent QA: STATIC PASS`。

### 尚未验证或被环境阻断

完整门禁命令已执行，但当前环境在 T32 前置检查因缺少 Docker 退出：

```text
T32 topology prerequisites: missing tool(s): docker
```

因此以下内容不能声称已通过：

- Docker Compose 配置和启动；
- PostgreSQL/Redis 真实拓扑；
- Agent setup、真实 migration 和 checkpoint setup/read/write；
- Provider 请求和真实 Agent 执行；
- Agent gRPC/HTTP 真实联调；
- T34 E2E；
- 浏览器 AI 链路。

没有 `SCYG_TEST_CONFIG_FILE` 时，PostgreSQL 端点测试会跳过；这不等于端点验收通过。

## 4. 当前不应宣称为已完成的内容

以下目标仍属于计划或未完成阶段：

- P4 Tool Gateway 与 Blog 只读 Tool；
- Tool Catalog、Recipe allowlist 和每次调用的执行时权限/用户/数据范围校验；
- `SearchArticles` 和文章读取的生产 Tool Gateway 包装；
- `submit_outline_for_approval` 及完整 HITL 恢复；
- 完整 Search、Write、Polish、Chat 四类 Agent 的真实主路径；
- checkpoint、interaction、command 三方 ApprovalReconciler；
- AgentRunner/Worker 的完整新路径切换；
- P10 真实拓扑和 readiness 验收；
- P11 删除 `runtimes/`、旧 Registry/Router/Adapter 和旧执行路径；
- Blog API、Blog↔Agent 调用方和前端 AI 页面接通。

不要为了“完成计划”提前删除旧 Runtime，也不要添加未经计划和规则确认的兼容层、双写路径、复杂恢复抽象或权限系统。

## 5. 继续工作的计划入口

主要计划：

- [Agent Harness 实施计划](../.scratch/documentation-migration/preserved/agent/agent-harness-implementation-plan.md)
- [框架重构计划审查报告](../.scratch/documentation-migration/preserved/plan/SCYG Agent第一版框架重构实施计划-审查报告.md)
- [任务工作流记录](../.scratch/documentation-migration/workflow.md)

计划阶段摘要：

| 阶段 | 内容 | 当前判断 |
| --- | --- | --- |
| P0 | 框架依赖冻结和构造探针 | 已完成主要冻结，真实 checkpoint 验收仍依赖拓扑 |
| P1 | 新合同和配置 | 已实现主要合同、配置和生成绑定 |
| P2 | 数据库迁移 | 源码已有迁移和持久化改动，完整真实数据库验收未完成 |
| P3 | Redis Streams | 适配器和测试已存在，真实 Redis 拓扑验收未完成 |
| P4 | Tool Gateway 与 Blog 只读 Tool | **下一步** |
| P5–P10 | 模型策略、四类 Agent、HITL、Worker 切换、接入和组合根 | 未完成或未完成真实验收 |
| P11 | 删除旧 Runtime | 必须在新路径完成并验证后进行 |
| P12 | 日常主路径验证和文档同步 | 当前仅完成部分文档同步，动态主路径未收口 |

## 6. P4 开始前的调查边界

开始编码前先核对：

1. `agent/src/scyg_agent/agents/recipes.py` 的 Recipe、模型和工具声明边界。
2. `agent/src/scyg_agent/agents/production.py` 的 AgentRunner 图构造和调用入口。
3. `agent/src/scyg_agent/adapters/blog_grpc/` 的 Blog gRPC Client、命令合同和现有权限校验。
4. `agent/src/scyg_agent/runtimes/profiles.py` 中现有 ToolPermission，判断哪些是旧 Runtime 画像、哪些可迁移到新 Tool Catalog。
5. `agent/src/scyg_agent/domain/ports/` 中 Tool、审计、操作幂等和数据范围相关接口。
6. `agent/tests/` 中 Blog gRPC、Tool execution、Recipe、Worker 和生产构造测试，复用现有测试模式。
7. 计划中 P4 的验收条件：
   - Research 能搜索已发布文章和当前用户自己的草稿；
   - 不能读取其他用户草稿；
   - checkpoint 中伪造的未授权 Tool call 在执行时被拒绝；
   - 第一版 Tool Catalog 不注册 Blog 写 Tool。

P4 只实现计划要求的 Blog 只读主路径和基本失败结果；不顺手实现外部 Web 搜索、MCP、Blog 写 Tool、复杂权限矩阵或极端故障注入。

## 7. 新电脑的最短接续命令

```powershell
Set-Location E:\gitproject\SCYG.Blog

git status --short

uv run --locked --project agent python agent/scripts/qa_agent.py --mode static
```

修改 `agent/` 前先阅读 `agent/AGENTS.md`。开始 P4 前读取源码、合同和测试，先形成明确的文件范围与验收方式，再实施。

具备 Docker、Compose、PostgreSQL、Redis、公钥和 Provider 配置后，再执行完整门禁：

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
- [ ] 已读取 Agent Harness 计划、审查报告和工作流记录。
- [ ] 已运行 `git status --short` 并保留既有修改。
- [ ] 已运行 Agent 静态门禁并记录真实结果。
- [ ] 已确认 P4 的源码入口、接口边界和验收条件。
- [ ] 已为本次实现建立或续接 `.scratch/<任务标识>/workflow.md`。
- [ ] 已完成 P4 行为验证后再更新正式文档。
- [ ] 已确认动态门禁是否具备 Docker、数据库、Redis 和 Provider 前置条件。
