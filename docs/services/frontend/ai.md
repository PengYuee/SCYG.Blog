# AI 前端接入与实现映射

本文供前端维护者定位组件、请求、状态和部署接点。作者四能力、写作交互及共同 Run 行为由 [AI 任务与作者辅助](../../modules/ai-assistance.md)维护；匿名身份、公开范围、额度及公共失败差异由[公共 AI 搜索](../../modules/public-ai-search.md)维护。本页不再维护另一份完整业务流程。

## 页面和服务装配

| 接点 | 前端职责与实现入口 |
| --- | --- |
| 作者入口 | [作者路由](../../../frontend/src/router/modules/author.ts)装配工作台和任务页，沿作者布局与认证守卫进入；共享登录语义见[登录与会话](../../modules/authentication.md)。 |
| 公共入口 | [首页](../../../frontend/src/views/public/HomeView.vue)提供普通／AI 模式切换；[公共创建控制器](../../../frontend/src/services/public-ai-search.ts)维护匿名提交与 Run 主题，[首页结果组件](../../../frontend/src/components/ai/PublicSearchResult.vue)复用 Run 协调器和结果解析，以轻量状态及普通文章卡片展示。布局约定见[设计系统](design.md#homepage-search)。旧公共搜索及任务地址由[公共路由](../../../frontend/src/router/modules/public.ts)转到首页，不复用作者 token。 |
| 工作台 | [AiWorkspace](../../../frontend/src/components/ai/AiWorkspace.vue)负责输入、客户端校验、创建中的本地状态和导航；[AiArticlePicker](../../../frontend/src/components/ai/AiArticlePicker.vue)提供管理端参考选择 UI。 |
| 任务页 | [AiRunView](../../../frontend/src/components/ai/AiRunView.vue)按路由参数持有一个生命周期实例，负责挂载、参数切换与卸载；业务交互由 [AiInteraction](../../../frontend/src/components/ai/AiInteraction.vue)呈现。 |
| 作者结果边界 | [AiResult](../../../frontend/src/components/ai/AiResult.vue)配合 [ai-business](../../../frontend/src/services/ai-business.ts)解析作者界面支持的业务结果，Markdown 统一走现有安全渲染器；公共搜索复用解析模型，不展开生成文本。 |
| Run 协调 | [ai-run](../../../frontend/src/services/ai-run.ts)拥有快照单飞、generation、AbortController、通知合并及恢复协调；组件不各自实现第二套监听。 |
| 主体状态 | [作者临时状态](../../../frontend/src/services/ai-session-state.ts)与[匿名会话](../../../frontend/src/services/anonymous-ai-session.ts)封装存储与失效通知；页面生命周期订阅通知并清理自身实例。 |
| API 端口 | 页面经注入的 ApiServices.ai 使用 [AI adapter](../../../frontend/src/request/api/ai.ts)，不在组件内创建 HTTP 客户端。 |

这些对象承载业务模块规定的行为；修改控件、存储或生命周期时，应先确定影响哪个模块规则，再选择对应验证入口。

## 请求、SSE 与错误边界

普通 HTTP 复用共享 Axios，明确选择 author、anonymous 或 none 认证策略；token provider 与失效处理集中到[认证策略](../../../frontend/src/request/auth-policy.ts)，页面不自行拼接 Bearer。普通请求与事件流采用同一认证域和公开错误模型，不能让匿名失败清除作者会话。

[SSE](../../../frontend/src/request/sse.ts)使用 fetch、ReadableStream 和增量 UTF-8 解码。请求从运行时 serverUrl 与服务端 streamUrl 组合，保留部署基础路径；仅接受配置的 Blog 来源与合法事件路径，不把 Cookie、URL token 或原生 EventSource 当作替代实现。游标和完整帧规则由 [Agent 流式合同](../agent/streaming.md)维护。

普通请求及 SSE 建立前错误统一为 HttpRequestError，保留公开 Problem 和校验后的 retryAfterSeconds。转换集中到[错误边界](../../../frontend/src/request/http-error.ts)，页面不依赖 Axios 原始 cause，也不从错误文字推算等待时间。错误后应采取的业务动作只维护在对应模块。

## 类型、生成与内容安全

HTTP types 与 Zod definitions 从唯一 [OpenAPI](../../../backend/api/openapi.yaml)生成；禁止手改生成物或手写等价的完整 wire Schema。ai-business 只描述 UI 对 Agent 任意 JSON 结果/交互的实际消费结构，不替代 Run HTTP 包络。

Markdown 使用现有 MarkdownRenderer 和 DOMPurify 边界，模型输出不是可信 HTML。适配器、Schema、领域模型及按需抽象的规则见[开发指南](development.md#api-契约与代码生成)，不在本页复制生成配置。

## 修改与验证

- UI 或生命周期变化：核对业务模块对应场景，并运行相关类型、单元/组件检查与真实页面验证。
- SSE 解析、认证域或错误归一化变化：验证实际网络分块/游标、来源限制与认证失效边界，不能只检查方法接线。
- 输入、结果或交互结构变化：同步权威合同/Agent 业务结构、生成物及消费者；先核对兼容关系，不在页面猜字段。
- 发布环境：确认 config.json、SPA fallback、跨域与网关允许的头、基础路径及持续事件连接，操作见[运行手册](operations.md)。

Microsoft Edge、代码生成和检查命令由[开发指南](development.md#验证命令)维护。确定性 fixture 不能代替真实 Blog→Agent→Provider 结果；既有业务回执的版本与覆盖范围应独立核对。
