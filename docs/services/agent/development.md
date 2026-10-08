# Agent 服务开发

## 当前运行路径

当前生产执行路径为 Recipe AgentRunner；旧 RuntimeRouter、producer、浏览器 Run HTTP/SSE 和 service JWT 入口已删除。Blog 是浏览器唯一 JWT 入口，经内部 AgentControlService 控制 Agent；Agent 通过 BlogContentService 访问管理端内容，生产 Tool 授权仍仅开放读取。源码职责见[架构](architecture.md)，公开流式合同见[streaming](streaming.md)。

## 静态检查入口

在仓库根目录运行以下命令。它们不要求 Docker、Compose、PostgreSQL、密钥或外部模型服务。

```powershell
task qa:agent:static
```

成功标记是 `agent QA: STATIC PASS`。该入口固定 Task `v3.49.1`、uv `0.11.28`，先执行 `task agent:install`，再以 `uv run --locked --no-sync --project agent` 启动 QA；子检查同样使用 `--no-sync`，执行合同漂移、Ruff、basedpyright、pytest 覆盖率、范围和密钥扫描、依赖审计与部署合同测试。

单独执行开发检查前，先从仓库根目录安装或刷新包：

```powershell
task agent:install
```

该任务先用独立于项目安装的 `uv run --no-project --python 3.12.13` 生成 Python 绑定，再执行 `uv sync --locked --project agent --no-editable --reinstall-package scyg-agent`。首次不能先同步非 editable 项目，因为 wheel 构建需要生成树；后续修改源码、绑定或迁移，也需重跑该任务才能更新已安装包。包内迁移资产要求非 editable 安装，以下命令统一用 `--no-sync`，避免 `uv run` 默认重新安装 editable 包。首次启动完整步骤见[服务入口](README.md#首次本机启动)。

安装完成后，从 `agent/` 目录运行：

```powershell
uv run --locked --no-sync scyg-agent-contracts check
uv run --locked --no-sync ruff check .
uv run --locked --no-sync ruff format --check .
uv run --locked --no-sync basedpyright
uv run --locked --no-sync pytest -q --cov=scyg_agent --cov-branch
```

Python gRPC 绑定生成树为 `agent/src/scyg_agent/generated/proto/`；需要重新生成时，从仓库根执行 `task agent:install`，同时刷新已安装包。业务层不得使用生成类型作为 domain 类型。90% 分支覆盖门槛统计可独立测试的业务模块，具体范围统一维护在 `pyproject.toml` 的 `tool.coverage.report.omit`；真实 PostgreSQL、checkpoint、Provider 与构建路径另做端点或产物验收。

生成物固定使用 LF，根 `.gitattributes` 对该目录强制 `eol=lf`，避免 Windows 检出换行导致字节级漂移。既有工作副本若已检出为 CRLF，重跑 `task agent:install` 后再执行 `check`，不手工修改绑定。原生生成与漂移检查需要 Node.js/npx 或 `SCYG_BUF_BIN` 指向的 Buf；有效 `BUF_TOKEN` 只适用于这些原生进程。当前 Docker 构建不接收宿主机 `BUF_TOKEN`，不能据此宣称容器生成支持 token，详见[合同生成与检查](README.md#合同生成与检查)。

## 本地拓扑

独立 Agent 拓扑使用 `agent/compose.yaml`；Blog↔Agent 完整集成使用根 `compose.yaml`，配置步骤见[运行手册](operations.md#认证与内部网络边界)。两者分别从对应目录的 `.env.example` 创建未跟踪的 `.env`，填写必填值，并为并行实例设置不同的 `SCYG_COMPOSE_PROJECT_NAME` 和宿主端口。

```powershell
Set-Location agent
docker compose config --quiet
docker compose up --build --wait
docker compose down --remove-orphans
```

`agent-setup` 是有限的一次性任务。它准备单一 `scyg_agent` 应用角色、执行 Agent Alembic 并使用同一个 DSN 初始化 checkpoint。PostgreSQL 与 Redis 健康、setup 成功后，常驻 `agent` 服务才启动。日常停止保留数据库卷；只有确认要删除本地数据库状态时才给 `down` 追加 `--volumes`。

启动拓扑需要 Docker、Compose、数据库和 Provider 配置，不需要 Agent JWT 公钥。配置展开或静态检查不等于动态验收通过，应按实际命令结果分别记录。

从仓库根目录执行完整门禁：

```powershell
task qa:agent
```

完整门禁检查拓扑前置条件，执行静态门禁、根集成 Compose 及真实 JWT→Agent→Provider→持久结果/SSE 冒烟；它创建独立 QA 项目并在结束时清理自己的临时资源。静态门禁与完整门禁的运行结果应分别记录。

## 配置和脱敏

Agent 进程默认读取当前工作目录的 `agent.toml`，模板位于 `agent/agent.toml.example`。先按上述顺序完成非 editable 安装，再在 `agent/` 目录复制模板、填写必填值并执行检查：

```powershell
Copy-Item agent.toml.example agent.toml
uv run --locked --no-sync python -m scyg_agent --check
```

配置按 TOML 文件、`SCYG_AGENT_` 环境变量、类型默认值的顺序取值，前者优先；文件可以只覆盖部分字段，其余字段继续从环境变量或默认值补齐。默认文件是当前工作目录的 `agent.toml`，仅在文件位于其他位置时设置 `SCYG_AGENT_CONFIG_FILE`。未知字段、错误类型和格式错误的 TOML 都会拒绝启动。`agent.toml` 已被 Git 和 Docker build context 忽略。

Compose 拓扑使用未跟踪的 `agent/.env` 为 PostgreSQL 初始化、setup 和容器注入完整拓扑配置；这是部署输入，不改变 Agent 进程自身的合并优先级。配置文件可以明文保存数据库密码和 Provider API key，不需要额外的密钥管理服务。

`--check` 只解析合并后的配置和组合 FastAPI，不绑定端口、不连接 PostgreSQL，也不启动 gRPC 或 Worker。它适合验证配置形状，不是运行时就绪证明。

普通日志和错误详情不要主动拼接数据库密码、Provider API key 或 JWT；配置明文存储不等于把敏感值复制到日志。

## 数据库、迁移和 checkpoint

一个 `scyg_agent` 应用角色连接 `scyg_agent` 数据库，同时访问 Agent truth 和 `langgraph` checkpoint schema。已有本机 PostgreSQL 时，先完成非 editable 安装，再在 `agent/` 目录执行 `uv run --locked --no-sync scyg-agent setup`；命令从 `database_url` 推导默认管理员地址，提示输入管理员密码，创建或同步应用角色、执行包内 Alembic 并初始化 checkpoint。管理员地址不同时使用 `--admin-url "postgresql://<admin>@<host>:<port>/postgres"`。

长期运行的 `uv run --locked --no-sync scyg-agent run` 不持有管理员凭据，也不自动迁移。Compose 通过一次性 `agent-setup` 使用同一个 DSN 完成初始化。checkpoint 库升级、LangGraph 升级或 schema 变更仍应先确认兼容性；checkpoint 不兼容时 readiness 必须失败，不能删除 checkpoint 或把它当作 SSE 事件日志。

## 交付边界

当前代码装配 Recipe AgentRunner、Redis 流、health-only HTTP、五 RPC AgentControlService、HITL 与租约 Worker；Blog 提供八个 JWT HTTP/SSE 入口和八 RPC BlogContentService。本页仅维护 Agent 与 Blog 后端集成，不覆盖前端 AI 页面实现。内部 gRPC 只信任回环或隔离网络，网络可达者可代填 `user_id`，不得直接公网暴露；浏览器 JWT、Run owner 和 Blog 资源权限校验保持不变。
