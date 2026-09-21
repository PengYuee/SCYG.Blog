# SCYG Blog 前端服务

该目录是 SCYG Blog 的浏览器端单页应用。应用使用 Vue 3、TypeScript、Vite、Pinia 和 Vue Router，面向桌面端公共阅读场景，并通过浏览器运行时配置连接后端 HTTP API。

生产构建当前只开放公共阅读能力。作者界面仅能在非生产环境通过显式开关启用；真实登录、会话恢复和管理后台尚未接入。

## 最短启动路径

`package.json` 固定使用 `pnpm@10.13.1`。在本目录执行：

```powershell
pnpm install
pnpm dev
```

开发服务器启动后访问终端输出的本地地址。应用挂载前会读取 [`public/config.json`](public/config.json)，其中 `serverUrl` 必须是可访问的 HTTP(S) 后端地址：

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

`pnpm check` 依次执行类型检查、单元测试、组件测试和生产构建，不包含端到端测试。端到端测试会自行启动开发服务器，本地默认使用 Microsoft Edge。

## 服务边界

- 公共域提供首页、文章列表、文章详情和公共错误页，通过配置的后端读取数据。
- 作者域只在非生产环境且 `VITE_FAKE_AUTHOR=true` 时开放。该开关提供开发身份，不代表所有数据都来自 Fake 仓储。
- 管理域保留 `/admin` 路由边界，当前统一呈现不可用状态。
- 生产模式始终关闭 Fake 作者能力；当前没有真实认证接入。
- 当前支持范围为桌面端，布局下限为 `1024px`，目标浏览器为 Microsoft Edge。

## 文档导航

- [服务架构](docs/architecture.md)：系统边界、启动链路、模块职责和关键失败路径。
- [开发指南](docs/development.md)：本地开发、目录职责、修改路径和验证命令。
- [部署与运行手册](docs/operations.md)：构建产物、运行时配置、发布验证和故障处理。
- [设计系统](DESIGN.md)：主题、视觉 Token、布局和交互规范。该文档不承担服务运行说明。
