# 前端开发指南

## 使用范围

本文面向在 `frontend/` 内修改代码的开发者，给出从本地启动到验证的最短路径。页面业务规则和完整 API 字段不在这里维护；架构边界见 [服务架构](architecture.md)。

## 环境准备

- 安装 Node.js 与 pnpm；仓库通过 `package.json#packageManager` 固定 `pnpm@10.13.1`。
- 本地端到端测试默认调用 Microsoft Edge。设置 `CI` 时 Playwright 配置改用其管理的 Chromium；执行前需要安装对应浏览器，例如 `pnpm exec playwright install chromium`。仓库当前没有前端 CI 工作流。
- 公共页面联调需要 `public/config.json` 中的后端地址可访问。

仓库没有独立的 Node.js 版本文件。若团队需要固定 Node.js 版本，应新增统一工具链约束，而不是只在本文记录一个易漂移的数字。

## 首次启动

在 `frontend/` 目录执行：

```powershell
pnpm install
pnpm dev
```

`public/config.json` 默认指向 `http://127.0.0.1:8080`。需要连接其他后端时，直接修改该文件中的 `serverUrl`；配置会在应用挂载前加载，不需要把地址编译进 TypeScript。

配置必须是严格 JSON 对象：

```json
{
  "serverUrl": "https://api.example.test"
}
```

`serverUrl` 只接受 HTTP(S) URL。获取失败、非 2xx、JSON 无效或字段错误都会阻止应用挂载。

## 开发作者界面

作者界面默认关闭。需要在开发环境使用时，从模板创建本地环境文件：

```powershell
Copy-Item .env.example .env
```

将 `.env` 设置为：

```dotenv
VITE_FAKE_AUTHOR=true
```

`.env` 已被 Git 忽略。该开关同时启用开发作者路由和 Fake 身份，但页面运行时仍可能调用真实 API；执行写操作前先确认 `public/config.json` 指向的后端允许开发写入。生产构建无条件关闭该能力。

## 目录职责

```text
frontend/
├─ public/               # 原样复制到构建产物的运行时配置和静态资源
├─ src/
│  ├─ components/       # 可复用 UI 组件
│  ├─ layouts/          # 公共和作者布局
│  ├─ views/            # 路由页面
│  ├─ router/           # 路由域、守卫、查询解析和滚动行为
│  ├─ stores/           # Pinia 客户端状态
│  ├─ request/          # HTTP 客户端、API 适配器和响应 Schema
│  ├─ services/         # 跨组件运行机制与能力门禁
│  ├─ security/         # Markdown 内容安全边界
│  ├─ config/           # 浏览器运行时配置
│  ├─ theme/            # 主题状态
│  ├─ assets/           # 全局样式与主题 Token
│  ├─ bootstrap.ts      # 配置优先的启动协调
│  └─ application.ts    # Vue 应用装配
└─ tests/
   ├─ unit/             # Node 环境的纯逻辑和边界测试
   ├─ component/        # jsdom 中的组件与页面行为测试
   └─ e2e/              # Playwright 浏览器验收
```

## 常见修改路径

### 修改页面或交互

1. 在 `views/` 或 `components/` 修改可观察行为。
2. 复用 `stores/`、`services/` 和注入的 API Services，不在组件内另建请求客户端。
3. 仅为稳定的用户行为、边界或失败结果补充相应层级测试。
4. 运行类型检查、对应测试和实际页面验证。

### 修改后端接口适配

1. 在 `src/types/` 维护前端内部模型。
2. 在 `src/request/api/schemas.ts` 或对应适配器定义外部响应边界。
3. 在 `src/request/api/` 转换 URL、参数、请求体和响应字段。
4. 通过 `createApiServices` 暴露能力；页面只消费服务，不读取原始响应。
5. 验证成功响应、主要错误和不兼容响应，不在页面层兼容错误结构。

### 修改路由

- 公共、作者和管理路由分别修改 `src/router/modules/` 下对应模块。
- 公共 catch-all 必须继续排在管理域和作者域之后。
- 新路由应提供 `meta.title`；旧地址兼容集中在路由层处理。
- 涉及滚动行为时同时检查普通导航、前进后退和刷新恢复。

### 修改主题或视觉基础

主题、语义 Token、布局域和交互约束以 [`../DESIGN.md`](../DESIGN.md) 为权威来源。组件应消费语义变量，不在局部引入第二套颜色或尺寸约定。

## 验证命令

| 命令 | 覆盖范围 | 外部依赖 |
| --- | --- | --- |
| `pnpm typecheck` | Vue 与 TypeScript 静态检查 | 无后端 |
| `pnpm test:unit` | `tests/unit/**/*.test.ts`，Node 环境 | 无后端 |
| `pnpm test:component` | `tests/component/**/*.test.ts`，jsdom 环境 | 默认使用测试替身或注入依赖 |
| `pnpm build` | 类型检查并生成 `dist/` 生产产物 | 无后端 |
| `pnpm check` | 类型检查、单元测试、组件测试、生产构建 | 无后端、无浏览器 |
| `pnpm test:e2e` | Playwright 桌面视口，自动启动开发服务器 | 本地 Microsoft Edge；设置 `CI` 时需要 Playwright Chromium |
| `pnpm test:e2e:production` | `tests/e2e/t13` 对预构建产物执行验收 | 先执行 `pnpm build`；本地 Edge，CI 环境使用 Chromium |
| `pnpm test:coverage` | Vitest V8 覆盖率报告 | 无浏览器 |

日常完整静态门禁：

```powershell
pnpm check
```

涉及真实浏览器行为时再执行：

```powershell
pnpm test:e2e
```

生产产物验收顺序：

```powershell
pnpm build
pnpm test:e2e:production
```

## 常见失败

### 页面只显示运行时配置错误

检查浏览器 Network 中 `/config.json` 的状态、响应是否为 JSON，以及 `serverUrl` 是否为 HTTP(S) URL。配置加载有 10 秒超时。

### 公共页面请求失败

确认后端正在运行、`serverUrl` 指向正确基础地址，并检查浏览器跨域错误。请求适配器会在该地址下访问其声明的 API 路径。

### 作者地址显示不可用

确认当前是 Vite 开发模式，且 `.env` 中只有字面值 `VITE_FAKE_AUTHOR=true`。修改环境变量后重新启动开发服务器。生产模式不能启用作者界面。

### E2E 无法启动浏览器

未设置 `CI` 时，Playwright 配置使用系统安装的 Microsoft Edge channel，应确认 Edge 可用。设置 `CI` 后配置使用 Playwright 管理的 Chromium；缺少可执行文件时运行 `pnpm exec playwright install chromium`。不要把配置改成与项目桌面支持范围无关的移动端项目。

## 相关文档

- [服务入口](../README.md)
- [服务架构](architecture.md)
- [部署与运行手册](operations.md)
