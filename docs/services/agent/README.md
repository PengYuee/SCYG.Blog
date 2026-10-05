# SCYG Agent

Agent 是 Python 3.12 服务，提供 gRPC 控制面、HTTP/SSE、Worker 和持久化执行。当前源码同时装配 Recipe AgentRunner 与 SIMPLE/DEEP RuntimeRouter；生产流式面使用 Redis，Run 状态与持久事件保存在 PostgreSQL。服务存在不代表 Blog 或浏览器 AI 路径已接通，详见[架构](architecture.md)。

本页命令均从仓库根目录进入 `agent/` 后执行。

## 首次本机启动

1. 安装项目所需的 Python 3.12 和 uv，准备 PostgreSQL、Redis、公钥及 Provider 配置。
2. 复制配置模板，填写必填值，执行不监听、不连接数据库的配置检查：

   ```powershell
   Set-Location agent
   Copy-Item agent.toml.example agent.toml
   uv run --locked python -m scyg_agent --check
   ```

3. 初始化数据库与 checkpoint，再启动完整服务：

   ```powershell
   uv run --locked scyg-agent setup
   uv run --locked scyg-agent run
   ```

`setup` 从 `database_url` 推导默认管理员地址，交互输入管理员密码，准备数据库与单一应用角色、执行 Agent truth migration 并初始化 LangGraph checkpoint。它不删除已有 Agent 表，但会同步应用角色密码和 schema 权限。管理员端点不同时使用：

```powershell
uv run --locked scyg-agent setup --admin-url "postgresql://admin@localhost:5432/postgres"
```

管理员 URL 不带密码。失败结果区分 `database`、`migration`、`checkpoint` 阶段，并在可用时给出 SQLSTATE，不回显凭据。长期运行的 `run` 不使用管理员连接，也不自动迁移。

配置文件优先于同名 `SCYG_AGENT_*` 环境变量，再回退到类型默认值。默认读取当前工作目录的 `agent.toml`；其他位置用 `SCYG_AGENT_CONFIG_FILE` 指定。不要提交本地 TOML、`.env` 或凭据。配置字段以[配置模型](../../../agent/src/scyg_agent/config.py)和[模板](../../../agent/agent.toml.example)为准。

## 合同生成与检查

```powershell
uv run --locked scyg-agent-contracts generate
uv run --locked scyg-agent-contracts check
```

合同源位于根 `contracts/`。检查通过临时生成比较已保存的绑定，不依赖 Git 状态。

## PostgreSQL 验收

```powershell
Copy-Item tests/test-agent.toml.example tests/test-agent.toml
$env:SCYG_TEST_CONFIG_FILE = (Resolve-Path tests/test-agent.toml)
uv run --locked pytest -q
```

`normal_database_url` 用于仓储、事件、命令、Worker 和 checkpoint 验收；`migration_database_url` 必须是隔离的迁移测试数据库，测试会执行 `downgrade base` 和 `upgrade head`。没有测试配置时，端点验收会跳过；离线单元测试通过不代表真实数据库验收通过。

## 本地 Compose

从 `.env.example` 创建本地 `.env`，填写必填值。数据库 DSN 指向 `postgres:5432/scyg_agent`，使用同一个 `scyg_agent` 应用角色；DSN 密码需 URL 编码，配对密码变量保留原值。并行实例使用不同的 `SCYG_COMPOSE_PROJECT_NAME`。

```powershell
docker compose config
docker compose up --build --wait
```

拓扑包含 PostgreSQL、Redis、一次性 `agent-setup` 和常驻 `agent`。setup 成功后启动服务；PostgreSQL、HTTP、gRPC 宿主端口只绑定 `127.0.0.1`。Docker 检视者可以看到 Compose 环境配置，本地环境不要求加密配置。普通日志不应输出凭据。

正常停止使用 `docker compose down --remove-orphans`。仅在确认要删除本地数据库状态时追加 `--volumes`；它不是日常停机命令。

## 文档导航

- [架构](architecture.md)：当前组件、源码入口与集成边界。
- [流式合同](streaming.md)：Redis 与持久事件游标、鉴权和幂等行为。
- [开发指南](development.md)：质量检查、配置与初始化。
- [运行手册](operations.md)：运行前检查、故障处理与关闭。
