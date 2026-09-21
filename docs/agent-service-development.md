# Agent 服务开发

## 已验证的静态入口

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
docker compose down --volumes --remove-orphans
```

`agent-setup` 是有限的一次性任务。它准备单一 `scyg_agent` 应用角色、执行 Agent Alembic 并使用同一个 DSN 初始化 checkpoint。只有它成功后，常驻 `agent` 服务才启动。

当前 Windows 主机没有 Docker、Compose 或获批 Agent PostgreSQL 端点。因此上面的拓扑命令是前置条件命令，未在本机执行，也不能据此声称 `task qa:agent`、Compose、迁移或 T34 通过。

完整入口为：

```powershell
task qa:agent
```

它先要求 Docker 及 `.env` 或完整拓扑环境变量，再运行静态门禁、Compose config、Compose up 和 T34。当前主机预期在 `T32 topology prerequisites: missing tool(s): docker` 失败，且不输出 PASS。

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

当前代码存在固定 SIMPLE 和 DEEP 运行时目录、Agent HTTP/SSE、命令和取消面、gRPC 控制面、租约 Worker 与恢复代码。Blog 登录、Run 创建 REST、Run JWT 签发和刷新、BlogTool 真实服务、Vue Run 页面尚未交付。开发文档不将这些 T22 至 T31 工作描述为可运行功能。
