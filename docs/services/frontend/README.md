# SCYG Blog 前端服务

前端源码位于仓库的 [`frontend/`](../../../frontend/) 目录，是 SCYG Blog 的浏览器端单页应用。应用使用 Vue 3、TypeScript、Vite、Pinia 和 Vue Router，面向桌面端公共阅读场景，并通过浏览器运行时配置连接后端 HTTP API。

公共阅读与作者界面均可在生产环境使用。真实登录、Bearer 会话恢复和本地退出由 [`auth-session.ts`](../../../frontend/src/services/auth-session.ts) 提供，作者读写通过 [`author-runtime.ts`](../../../frontend/src/services/author-runtime.ts) 连接真实管理 API；不需要非生产环境开关。管理后台尚未启用。

## 最短启动路径

`frontend/package.json` 固定使用 `pnpm@10.13.1`。在仓库的 `frontend/` 目录执行：

```powershell
pnpm install
pnpm dev
```

开发服务器启动后访问终端输出的本地地址。应用挂载前会读取 [`frontend/public/config.json`](../../../frontend/public/config.json)，其中 `serverUrl` 必须是可访问的 HTTP(S) 后端地址：

```json
{
  "serverUrl": "http://127.0.0.1:8080"
}
```

配置获取失败、JSON 无效或字段不符合约束时，应用不会挂载，而是在 `#app` 中显示启动错误。

## 验证入口

```powershell
pnpm check
pnpm test:e2e
```

`pnpm check` 先重新生成并检查 API 类型与 Zod Schema 无漂移，再执行类型检查、单元测试、组件测试和生产构建，不包含端到端测试。端到端测试会自行启动开发服务器，本地默认使用 Microsoft Edge。

## 服务边界

- 公共域提供首页、文章列表、文章详情、登录页和公共错误页，通过配置的后端读取数据。
- 作者域使用后端短期 Bearer JWT，匿名访问会跳转 `/login` 并保留站内返回地址。
- 管理域保留 `/admin` 路由边界，当前统一呈现不可用状态。
- 登录会话在浏览器持久化；退出、过期或后端返回 `401` 时清除，不实现后端尚未提供的 refresh/logout API。
- 当前支持范围为桌面端，布局下限为 `1024px`，目标浏览器为 Microsoft Edge。

## 文档导航

- [服务架构](architecture.md)：系统边界、启动链路、模块职责和关键失败路径。
- [开发指南](development.md)：本地开发、目录职责、修改路径和验证命令。
- [部署与运行手册](operations.md)：构建产物、运行时配置、发布验证和故障处理。
- [设计系统](design.md)：主题、视觉 Token、布局和交互规范。该文档不承担服务运行说明。
