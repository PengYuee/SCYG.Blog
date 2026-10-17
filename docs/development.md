# 项目开发指南

## 环境与工作目录

博客主路径要求前端、后端与 Blog PostgreSQL，不以 Agent 为前置条件。AI 功能额外要求 Agent、Redis 和可用 Provider。工具链与完整命令分别由[后端入口](services/backend/README.md)、[前端开发指南](services/frontend/development.md)及 [Agent 入口](services/agent/README.md)维护，根质量任务以 Taskfile.yml 为准。

下面的启动命令从仓库根目录执行；其他文档中的服务命令从相应服务目录执行。

## 博客最短启动顺序

1. 准备 PostgreSQL、Blog 数据库与应用账号，按[后端入口](services/backend/README.md)填写 backend/config.local.yaml。本机配置不要提交。
2. 首次创建配置后迁移并启动 API；已有配置不要覆盖：

   ```powershell
   Set-Location backend
   Copy-Item config.example.yaml config.local.yaml
   # 填写配置后执行
   task migrate:up CONFIG=config.local.yaml
   go run ./cmd/api -config config.local.yaml
   ```

   第 4 版迁移创建默认活动种子用户，只保存 bcrypt 哈希，见[迁移源](../backend/migrations/000004_users.up.sql)；迁移不会覆盖已有同名用户。本地联调账号统一见[根规则](../AGENTS.md#本地开发与联调账号)，不作为生产部署凭据。健康入口为 `/live` 与 `/ready`，接口定义见 [OpenAPI](../backend/api/openapi.yaml)。
3. 在另一个终端启动前端：

   ```powershell
   Set-Location frontend
   pnpm install
   pnpm dev
   ```

   前端挂载前读取 [public/config.json](../frontend/public/config.json)。让 serverUrl 指向后端实际 HTTP 地址；跨域和部署见[运行手册](services/frontend/operations.md)。
4. 打开首页、文章列表和有效详情，使用本地活动账号登录并确认返回作者页面。保存文章和图片会修改业务数据，应使用测试内容。空数据库没有可读文章不等于启动失败。

## Agent 独立运行

按 [Agent 入口](services/agent/README.md)准备 Agent PostgreSQL、Redis、Provider 和内部 Blog gRPC 地址，先完成合同生成与非 editable 安装，再在 agent/ 使用 --no-sync 执行 --check、setup、run。--check 不监听、不连接数据库，不是 readiness 证明；Agent 不再要求浏览器或 service JWT 公钥。

需要完整 AI 链路时，从仓库根使用 compose.yaml，按[联合运行说明](services/agent/operations.md)配置并启动 Blog、Agent 及依赖；不要把独立 Agent 的对端地址直接当作浏览器入口。正常停机保留数据库卷，不把 --volumes 数据删除作为日常操作。

前端 serverUrl 始终指向浏览器可访问的 Blog HTTP 地址。作者登录后进入 /author/ai，访客入口为 /ai/search；两者的凭证、内容范围和恢复规则见 [AI 使用说明](services/frontend/ai.md)。生成内容当前只提供预览和复制，不自动保存或回填编辑器。

## 按改动范围验证

| 改动范围 | 入口与前置条件 |
| --- | --- |
| 前端 | frontend 下 pnpm check；E2E 用 Microsoft Edge，先清除 CI 环境变量；见[前端开发](services/frontend/development.md) |
| 后端 | backend 下 task ci；数据库检查显式提供 QA_CONFIG；见[后端入口](services/backend/README.md) |
| Agent | 根 task qa:agent:static；完整 task qa:agent 需要 Docker、Compose 和完整拓扑；见[Agent 开发](services/agent/development.md) |
| 共享合同 | 修改源合同，运行对应生成与漂移检查并验证消费者；禁止手改绑定 |
| 文档与目录 | 检查导航、相对链接、源码入口、工作目录及工具引用；仅迁文档不重跑业务测试矩阵 |

根 Taskfile 默认只检查 Agent，不是整仓门禁。现有后端与 Agent CI 见[工作流](../.github/workflows/)；前端本地检查不代表已接入前端 CI。配置检查、进程存活与业务验收分别记录。

文档与任务材料归属遵循[根规则](../AGENTS.md)，运行失败处理由服务手册维护。
