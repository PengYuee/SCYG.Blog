# Agent 服务开发

## 当前交付状态

当前工作区已落地 Recipe 合同、配置路径、Redis Stream 适配、AgentRunner 边界和相关静态测试；源码仍同时保留 Recipe AgentRunner 与 SIMPLE/DEEP RuntimeRouter，旧 Runtime 尚未进入删除阶段。未完成的实施计划、需求草稿和审查材料保存在 `.scratch/`，不作为现行实现说明。

已验证：

- `qa:agent:static` 通过，包含合同漂移、Ruff、basedpyright、700 个通过的测试、覆盖率、范围扫描、密钥扫描、依赖审计和部署合同测试。
- PostgreSQL 端点验收未执行；未提供 `SCYG_TEST_CONFIG_FILE` 时相关测试跳过。

未验证或被环境阻断：

- `qa:agent` 完整门禁尚未通过；当前执行在 T32 因缺少 Docker 停止，Compose、真实拓扑和 T34 未执行。
- Provider、PostgreSQL checkpoint、Redis 拓扑、Agent gRPC/HTTP 真实联调和浏览器链路不能由静态门禁替代。

后续实施以 scratch 中的 Agent Harness 计划为准，下一阶段是 P4 Tool Gateway 与 Blog 只读 Tool；在该阶段完成并取得对应验收证据前，不把 Tool Gateway、HITL 恢复、完整四类 Agent 或旧 Runtime 清理写成当前能力。

## 静态检查入口

在仓库根目录运行以下命令。它们不要求 Docker、Compose、PostgreSQL、密钥或外部模型服务。

```powershell
task qa:agent:static
```

成功标记是 `agent QA: STATIC PASS`。该入口固定 Task `v3.49.1`、uv `0.11.28`，并以 `uv run --locked --project agent` 运行合同漂移、Ruff、basedpyright、pytest 覆盖率、范围和密钥扫描、依赖审计与部署合同测试。

单独执行 Agent 开发检查时，从 `agent/` 目录运行：

```powershell
uv run --locked scyg-agent-contracts check
uv run --locked ruff check .
uv run --locked ruff format --check .
uv run --locked basedpyright
uv run --locked pytest -q --cov=src/scyg_agent --cov-branch
```

生成 Python gRPC 绑定使用 `uv run --locked scyg-agent-contracts generate`。生成物属于 `agent/src/scyg_agent/generated/`，业务层不得以它们作为 domain 类型。

## 本地拓扑

本地一进程拓扑的唯一获批入口是 `agent/compose.yaml`。先从 `agent/.env.example` 创建未跟踪的 `agent/.env`，填写每个必填值，并为并行实例设置不同的 `SCYG_COMPOSE_PROJECT_NAME`。

```powershell
Set-Location agent
docker compose config
docker compose up --build --wait
docker compose down --remove-orphans
```

`agent-setup` 是有限的一次性任务。它准备单一 `scyg_agent` 应用角色、执行 Agent Alembic 并使用同一个 DSN 初始化 checkpoint。PostgreSQL 与 Redis 健康、setup 成功后，常驻 `agent` 服务才启动。日常停止保留数据库卷；只有确认要删除本地数据库状态时才给 `down` 追加 `--volumes`。

启动拓扑需要 Docker、Compose、数据库、公钥和 Provider 配置。历史任务记录中的环境阻塞不代表当前开发机状态；应以实际命令结果判断，不把配置展开或静态检查称为动态验收通过。

完整入口为：

```powershell
task qa:agent
```

它检查拓扑前置条件，并执行静态门禁、Compose 和 T34。当前源码已引入 Redis 与 Recipe 路径，历史 T34 结果不能替代对新路径的实际验证。

## 配置和脱敏

Agent 进程默认读取当前工作目录的 `agent.toml`，模板位于 `agent/agent.toml.example`。在 `agent/` 目录复制模板、填写必填值，再执行检查：

```powershell
Copy-Item agent.toml.example agent.toml
uv run --locked python -m scyg_agent --check
```

配置按 TOML 文件、`SCYG_AGENT_` 环境变量、类型默认值的顺序取值，前者优先；文件可以只覆盖部分字段，其余字段继续从环境变量或默认值补齐。默认文件是当前工作目录的 `agent.toml`，仅在文件位于其他位置时设置 `SCYG_AGENT_CONFIG_FILE`。未知字段、错误类型和格式错误的 TOML 都会拒绝启动。`agent.toml` 已被 Git 和 Docker build context 忽略。

Compose 拓扑使用未跟踪的 `agent/.env` 为 PostgreSQL 初始化、setup 和容器注入完整拓扑配置；这是部署输入，不改变 Agent 进程自身的合并优先级。配置文件可以明文保存数据库密码和 Provider API key，不需要额外的密钥管理服务。

`--check` 只解析合并后的配置和组合 FastAPI，不绑定端口、不连接 PostgreSQL，也不启动 gRPC 或 Worker。它适合验证配置形状，不是运行时就绪证明。

普通日志和错误详情不要主动拼接数据库密码、Provider API key 或 JWT；配置明文存储不等于把敏感值复制到日志。

## 数据库、迁移和 checkpoint

一个 `scyg_agent` 应用角色连接 `scyg_agent` 数据库，同时访问 Agent truth 和 `langgraph` checkpoint schema。已有本机 PostgreSQL 时，在 `agent/` 目录执行 `uv run --locked scyg-agent setup`；命令从 `database_url` 推导默认管理员地址，提示输入管理员密码，创建或同步应用角色、执行 Alembic 并初始化 checkpoint。管理员地址不同时使用 `--admin-url "postgresql://<admin>@<host>:<port>/postgres"`。

长期运行的 `scyg-agent run` 不持有管理员凭据，也不自动迁移。Compose 通过一次性 `agent-setup` 使用同一个 DSN 完成初始化。checkpoint 库升级、LangGraph 升级或 schema 变更仍应先确认兼容性；checkpoint 不兼容时 readiness 必须失败，不能删除 checkpoint 或把它当作 SSE 事件日志。

## 交付边界

当前代码装配 Recipe AgentRunner、SIMPLE/DEEP RuntimeRouter、Redis 流、Agent HTTP/SSE、gRPC 控制面及租约 Worker。Blog 自身已经有用户登录，但 Blog 的 Run REST、Agent 控制面调用、Run JWT 签发与前端 AI 页面尚未接通。源码入口与职责见[架构](architecture.md)，事件来源和 cursor 区别见[流式合同](streaming.md)。
