# 前端开发指南

## 使用范围

本文面向在 [`frontend/`](../../../frontend/) 内修改代码的开发者，给出从本地启动到验证的最短路径。除明确带仓库根路径的条目外，下文源码路径均相对于 `frontend/`。页面业务规则和完整 API 字段不在这里维护；架构边界见 [服务架构](architecture.md)。

## 环境准备

- 安装 Node.js 与 pnpm；仓库通过 `package.json#packageManager` 固定 `pnpm@10.13.1`。
 - 本地端到端测试统一调用 Microsoft Edge，Playwright 配置不根据 CI 环境变量切换浏览器。执行前需要确认本机已安装 Edge。
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

作者界面使用后端真实认证，不需要 Vite 环境开关。开始联调前：

1. 确认 `public/config.json` 的 `serverUrl` 指向已启用认证的后端。
2. 在后端数据库准备可登录的活动用户；前端不内置用户名或密码。
3. 打开 `/login`，成功后返回原作者地址或默认写作台。

浏览器将短期 Bearer 会话保存到 localStorage 的 `scyg.auth.session`。恢复时会校验生成的 `LoginResponse` Schema 和 `expiresAt`；过期、畸形或收到后端 `401` 的会话会被清除。后端当前没有 refresh 或 logout 端点，因此退出登录只删除浏览器会话。

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

## API 契约与代码生成

### 权威来源

- `backend/api/openapi.yaml` 是 HTTP API 的唯一契约来源，负责路径、方法、参数、请求体、响应体和状态码。
- OpenAPI 当前使用 camelCase 字段命名。前端不再为接口字段增加 snake_case 与 camelCase 的特殊转换层。
- 修改接口契约时，先更新 OpenAPI，再重新生成前端类型和 Zod Schema，并同步 mock、fixture、测试和调用方。
- `src/request/generated/` 由生成命令维护，禁止手工修改；`types.gen.ts` 和 `zod.gen.ts` 必须随契约一起提交。

### 生成类型、运行时校验和领域类型

- 请求、响应和参数类型由 `@hey-api/typescript` 生成；wire 响应的运行时 Schema 由 Hey API Zod 插件从同一份 OpenAPI definitions 生成。
- 请求适配器通过生成的 Zod Schema 校验响应，并由 `parseBoundary` 将失败转换为 `ApiParseError`；不得恢复等价的手写 wire Schema。
- OpenAPI 生成器暂未表达的约束只能保留最小后置不变量；当前只有文章 `tagIds` 的 `uniqueItems` 检查。运行时配置、URL query、localStorage、用户导入数据和第三方接口仍按各自边界校验。
- 领域类型只表达真实语义差异，例如分页索引转换、编辑器 `content` 与 `markdown` 的语义差异、多个接口组合的 ViewModel 或 UI 状态。字段和含义完全一致时直接使用生成类型。

### Adapter 与 Service

- Adapter 按需使用，不按资源数量强制创建。只在请求参数、响应结构或业务语义确实需要转换时编写小型 adapter。
- 仅复制同名字段的 adapter 不得保留；不要为了统一分层建立空 adapter、repository 或 service。
- 请求层负责调用、参数转换、必要的响应转换和错误边界；分页追加、去重、重试、loading 和局部错误由 Store、composable 或页面状态机负责。
- 跨接口编排、共享运行机制或能力门禁放在 `src/services/`；页面通过注入的 API Services 或服务使用能力，不在组件内另建 HTTP 客户端。

### 请求、错误与分页

- wire 字段、路径和参数以 OpenAPI 为准；前端内部字段只有在存在明确领域差异时才另命名。
- 版本控制请求使用契约规定的版本头；错误在 HTTP 边界统一为稳定错误类型，保留状态码、`detail` 及可用的 Problem Details 字段。页面不得依赖 Axios 原始错误结构。
- 分页请求一次只负责一页并返回契约分页信息。调用方负责下一页、追加、去重、失败页重试和 `hasNextPage` 判断。
- 文章列表的排序值必须来自 OpenAPI 允许范围；未指定时使用契约约定的默认值。不要在不同页面重复拼接排序参数。

### Mock、路由和测试

- Mock 和真实实现共享同一调用契约；页面不直接读取 mock 数据，也不解析原始 wire 响应。
- fixture 优先使用生成类型或统一 factory，避免维护第二套接口事实。
- 路由 query 只接受契约定义的键和合法值。未知键、重复键和非法值进入稳定 invalid 状态；invalid 状态显示提示、不调用依赖该查询的 API，也不自动清理或改写 URL。
- 测试可观察行为：路径、方法、参数、请求体、响应映射、错误行为、分页、重试和页面状态。不要测试生成文件的内部结构、字段复制实现或 adapter 是否存在。

## 常见修改路径

### 修改页面或交互

1. 在 `views/` 或 `components/` 修改可观察行为。
2. 复用 `stores/`、`services/` 和注入的 API Services，不在组件内另建请求客户端。
3. 仅为稳定的用户行为、边界或失败结果补充相应层级测试。
4. 运行类型检查、对应测试和实际页面验证。

### 修改后端接口适配

1. 先确认 `backend/api/openapi.yaml` 的契约已更新；接口字段和路径不在前端文档中另立事实。
2. 重新生成类型和 Zod Schema，并检查生成产物是否覆盖当前响应、错误和约束需求。
3. 生成类型与调用方一致时直接复用；存在真实语义差异时，在请求边界增加最小 adapter 或 service。
4. 对生成器未表达的 OpenAPI 约束增加最小后置不变量，不维护第二套完整 wire Schema。
5. 通过 `createApiServices` 暴露需要的能力；页面不读取原始响应，也不在页面层兼容错误结构。
6. 验证成功响应、主要错误、不兼容响应以及请求路径和参数。

### 引入或升级 Hey API

当前 Hey API 生成配置以 [`frontend/openapi-ts.config.ts`](../../../frontend/openapi-ts.config.ts) 为准，启用 TypeScript 与 Zod definitions，不生成 operation client。修改 API 契约时，先更新 [`backend/api/openapi.yaml`](../../../backend/api/openapi.yaml)，在 `frontend/` 执行 `pnpm generate:api`，同步必要的领域适配器、mock、fixture 和调用方，再按本文执行验证；生成目录禁止手工修改。既往实施计划是临时存档，不作为正式开发规则。

### 修改路由

- 公共、作者和管理路由分别修改 `src/router/modules/` 下对应模块。
- 公共 catch-all 必须继续排在管理域和作者域之后。
- 新路由应提供 `meta.title`；旧地址兼容集中在路由层处理。
- 涉及滚动行为时同时检查普通导航、前进后退和刷新恢复。

### 修改主题或视觉基础

主题、语义 Token、布局域和交互约束以 [设计系统](design.md) 为权威来源。组件应消费语义变量，不在局部引入第二套颜色或尺寸约定。

## 验证命令

| 命令 | 覆盖范围 | 外部依赖 |
| --- | --- | --- |
| `pnpm typecheck` | Vue 与 TypeScript 静态检查 | 无后端 |
| `pnpm test:unit` | `tests/unit/**/*.test.ts`，Node 环境 | 无后端 |
| `pnpm test:component` | `tests/component/**/*.test.ts`，jsdom 环境 | 默认使用测试替身或注入依赖 |
| `pnpm build` | 类型检查并生成 `dist/` 生产产物 | 无后端 |
| `pnpm check` | 重新生成并检查 API 类型与 Zod Schema 无漂移，然后执行类型检查、单元测试、组件测试和生产构建 | 无后端、无浏览器 |
| `pnpm test:e2e` | Playwright 开发验收桌面视口，自动启动 development Vite server，排除 `tests/e2e/t13` | 本地 Microsoft Edge |
| `pnpm test:e2e:production` | `tests/e2e/t13` 对预构建产物执行生产验收 | 先执行 `pnpm build`；本地 Microsoft Edge |
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

### 作者地址跳回登录页

确认浏览器中存在未过期会话，且登录响应符合 OpenAPI `LoginResponse`。若受保护请求返回 `401`，前端会清除会话，用户需要重新登录。

### E2E 无法启动浏览器

未设置或设置 `CI` 时，Playwright 配置均使用本机 Microsoft Edge channel；确认 Edge 可用。不要把配置改成与项目桌面支持范围无关的移动端项目。

## 相关文档

- [服务入口](README.md)
- [服务架构](architecture.md)
- [部署与运行手册](operations.md)
