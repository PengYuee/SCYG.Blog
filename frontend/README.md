# SCYG Blog

SCYG Blog 是基于 Vue 3、TypeScript、Vite、Tailwind CSS、Pinia 与 Vue Router 的桌面端博客前端。公共阅读使用真实后端只读接口；作者写入仅提供显式启用的本地 Fake 模式。

## 环境与命令

- Node.js 20 或更高版本
- pnpm 10（仓库固定 `pnpm@10.13.1`）

```bash
pnpm install
pnpm dev
pnpm typecheck
pnpm test:unit
pnpm test:component
pnpm test:e2e
pnpm build
pnpm preview
```

开发服务默认入口为 `http://localhost:5173/`。`public/config.json` 是 API 地址的唯一来源；应用会在导入路由和公共页面、挂载 Vue 之前加载并校验该文件。部署时直接修改已部署的 `config.json` 中 `serverUrl`，无需重新构建前端。配置加载或校验失败时，应用不会挂载，并会显示中文启动错误。

## 路由边界

- 公共域：`/`、`/articles`、`/articles/:id`，以及兼容旧链接的 `/main`、`/article/:id`。
- 作者域：`/author/articles/new`、`/author/articles/:id/edit`、`/author/taxonomy`。只有开发或测试环境显式开启 Fake 作者模式时提供编辑界面；默认呈现不可用边界。
- 管理域：`/admin` 及其全部后代由独立模块接管，当前统一呈现“管理后台暂不可用”，不会落入公共 404。
- 未知的非管理地址由公共 404 处理。

## 数据与写入模型

公共文章、分类、标签与搜索统一通过 `public/config.json` 中 `serverUrl` 指向的真实 REST API 读取，API 托管的相对图片也使用同一地址归一化，响应会在边界完成结构解析。项目没有真实认证接入，也不会持久化令牌或伪造后端登录态。

作者编辑、分类和图片操作仅在非生产环境且 `VITE_FAKE_AUTHOR=true` 时使用内存 Fake 仓储。Fake 数据会随页面刷新丢失，不代表后端写入成功。生产构建会强制关闭 Fake 作者能力；共享 mutation guard 会在调用适配器或网络层之前阻止写入。

## 环境变量

- `VITE_FAKE_AUTHOR`：Fake 作者与 Fake Auth 的显式联合开关；仅字面值 `true` 生效，生产环境始终关闭。安全默认值为 `false`。

当前不支持真实登录、会话恢复或后台管理能力。`/login` 只返回“登录暂不可用”的真实状态。

## 桌面支持

当前设计与验收范围为桌面端，布局支持下限为 `1024px`，目标浏览器为 Microsoft Edge，基准视口为 `1440 × 900`。移动端与平板端尚未纳入支持承诺。
