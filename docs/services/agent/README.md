# SCYG Agent

Agent 是 Python 3.12 服务，提供内部五 RPC gRPC 控制面、health HTTP、Worker 和持久化执行。Recipe AgentRunner 是唯一生产执行路径；PostgreSQL 保存 Run、持久事件和公开订阅，Redis 服务 Worker 瞬时流。Blog 是浏览器 JWT 与业务 HTTP/SSE 的唯一入口，不向 Agent 传递运行或 service JWT；职责详见[架构](architecture.md)，验证入口见[开发指南](development.md)。

首次生成和安装从仓库根目录执行；安装完成后，本页其余本机命令从 `agent/` 执行，使用 `--no-sync` 保留已安装的非 editable 包。Compose 命令也从 `agent/` 执行；根集成拓扑另有说明。

## 首次本机启动

1. 安装 Python 3.12、uv 和 Task，准备 PostgreSQL、Redis、Blog 内部 gRPC 地址及 Provider 配置。原生合同生成还需要 Node.js/npx，或用 `SCYG_BUF_BIN` 指向可用的 Buf。
2. 从仓库根目录生成绑定并安装非 editable 包：

   ```powershell
   task agent:install
   ```

   该任务先以 `uv run --no-project --python 3.12.13` 执行生成脚本，再运行 `uv sync --locked --project agent --no-editable --reinstall-package scyg-agent`。首次安装不能先同步项目：wheel 构建需要生成好的绑定。迁移配置和 migrations 也随 wheel 安装；不要使用会默认重新同步为 editable 的裸 `uv run --locked`。
3. 进入 `agent/`，复制配置模板并填写必填值，执行不监听、不连接数据库的配置检查：

   ```powershell
   Set-Location agent
   Copy-Item agent.toml.example agent.toml
   uv run --locked --no-sync python -m scyg_agent --check
   ```

4. 初始化数据库与 checkpoint，再启动完整服务：

   ```powershell
   uv run --locked --no-sync scyg-agent setup
   uv run --locked --no-sync scyg-agent run
   ```

`setup` 从 `database_url` 推导默认管理员地址，交互输入管理员密码，准备数据库与单一应用角色、执行 Agent truth migration 并初始化 LangGraph checkpoint。它不删除已有 Agent 表，但会同步应用角色密码和 schema 权限。管理员端点不同时使用：

```powershell
uv run --locked --no-sync scyg-agent setup --admin-url "postgresql://admin@localhost:5432/postgres"
```

管理员 URL 不带密码。失败结果区分 `database`、`migration`、`checkpoint` 阶段，并在可用时给出 SQLSTATE，不回显凭据。长期运行的 `run` 不使用管理员连接，也不自动迁移。

配置文件优先于同名 `SCYG_AGENT_*` 环境变量，再回退到类型默认值。默认读取当前工作目录的 `agent.toml`；其他位置用 `SCYG_AGENT_CONFIG_FILE` 指定。不要提交本地 TOML、`.env` 或凭据。配置字段以[配置模型](../../../agent/src/scyg_agent/config.py)和[模板](../../../agent/agent.toml.example)为准。

## 合同生成与检查

从仓库根目录重新生成并刷新已安装包（首次安装同样使用此入口）：

```powershell
task agent:install
```

随后从 `agent/` 检查绑定漂移：

```powershell
uv run --locked --no-sync scyg-agent-contracts check
```

合同源位于根 `contracts/`，Python 绑定生成到 `agent/src/scyg_agent/generated/proto/`，导入命名空间为 `scyg_agent.generated.proto`。检查通过临时生成比较已保存的绑定，不依赖 Git 状态。`pyproject.toml` 明确将绑定纳入 wheel/sdist；Docker builder 先生成绑定，再以非 editable 方式安装项目。仓库生成与漂移检查需生成工具，安装后的运行时不依赖仓库根合同源。

`setup` 与部署初始化从已安装的 `scyg_agent` 包内读取 `alembic.ini` 和 migrations；不要求配置目录包含源码或迁移文件。本机修改源码、绑定或迁移后，从仓库根重新执行 `task agent:install`，再使用 `--no-sync` 运行；非 editable 安装不会自动消费源码修改。远程 Buf 插件依赖 BSR 配额；匿名生成被限流时，有效 `BUF_TOKEN` 仅能用于原生生成/漂移检查进程。当前 Docker 构建不接收宿主机的 `BUF_TOKEN`，本机设置 token 不能解决容器构建的 BSR 限流；不能跳过生成或漂移检查。

## PostgreSQL 验收

```powershell
Copy-Item tests/test-agent.toml.example tests/test-agent.toml
$env:SCYG_TEST_CONFIG_FILE = (Resolve-Path tests/test-agent.toml)
uv run --locked --no-sync pytest -q
```

`normal_database_url` 用于仓储、事件、命令、Worker 和 checkpoint 验收；`migration_database_url` 必须是隔离的迁移测试数据库，测试会执行 `downgrade base` 和 `upgrade head`。没有测试配置时，端点验收会跳过；离线单元测试通过不代表真实数据库验收通过。

## 本地 Compose

从 `.env.example` 创建本地 `.env`，填写必填值。数据库 DSN 指向 `postgres:5432/scyg_agent`，使用同一个 `scyg_agent` 应用角色；DSN 密码需 URL 编码，配对密码变量保留原值。并行实例使用不同的 `SCYG_COMPOSE_PROJECT_NAME`。

```powershell
docker compose config --quiet
docker compose up --build --wait
```

拓扑包含 PostgreSQL、Redis、一次性 `agent-setup` 和常驻 `agent`。setup 成功后启动服务；仅 PostgreSQL 与 health HTTP 宿主端口绑定 `127.0.0.1`，gRPC 不映射宿主。该独立拓扑需另备网络可达的 Blog gRPC；完整 Blog↔Agent 集成从仓库根使用 `compose.yaml`，按其中声明提供环境变量或本地 `.env`，Agent/Blog 的 gRPC 仅在 Compose 内部网络调用。Docker 检视者可以看到 Compose 环境配置，本地环境不要求加密配置。普通日志不应输出凭据。

正常停止使用 `docker compose down --remove-orphans`。仅在确认要删除本地数据库状态时追加 `--volumes`；它不是日常停机命令。

## 文档导航

- [架构](architecture.md)：当前组件、源码入口与集成边界。
- [流式合同](streaming.md)：内部 gRPC、Blog 透明 SSE、opaque cursor、ready 与成功幂等行为。
- [开发指南](development.md)：质量检查、配置与初始化。
- [运行手册](operations.md)：运行前检查、故障处理与关闭。
