# 前端开发规则

修改 `frontend/` 内的代码、测试、配置或文档前，必须先阅读 [`../docs/services/frontend/development.md`](../docs/services/frontend/development.md)。该文档维护前端开发路径、API 契约、代码生成、按需抽象、测试和验证规范。

## 规则

- `backend/api/openapi.yaml` 是 HTTP API 契约的唯一来源；不要在前端维护与其冲突的接口事实。
- OpenAPI 当前使用 camelCase；不要为接口字段增加重复的 snake_case/camelCase 转换层。
- 生成代码禁止手工修改。先修改 OpenAPI 和生成配置，再重新生成客户端或类型。
- 生成类型与页面或 Store 模型一致时直接复用；只有存在真实语义差异时才增加最小 adapter 或 service。
- 不要为了统一分层创建空 adapter、repository 或 service；分页、重试、去重和 UI 状态由调用方负责。
- 运行时 schema 按风险使用，优先从 OpenAPI 自动生成；不得长期手写一套等价的完整接口 schema。
- HTTP 错误在请求边界归一化；页面不得依赖 Axios 原始错误结构或兼容旧接口别名。
- Mock、fixture 和真实实现共享调用契约；测试验证可观察行为，不测试生成文件内部结构或字段复制实现。
- 修改页面、作者界面或交互后，按开发指南执行对应类型检查、测试和真实页面验证。

## 文档归属

- 前端开发流程、API 生成和抽象规则：[`../docs/services/frontend/development.md`](../docs/services/frontend/development.md)
- 前端架构、启动链路和模块职责：[`../docs/services/frontend/architecture.md`](../docs/services/frontend/architecture.md)
- 部署、运行时配置和发布验证：[`../docs/services/frontend/operations.md`](../docs/services/frontend/operations.md)
- 主题、视觉 Token、布局和交互：[`../docs/services/frontend/design.md`](../docs/services/frontend/design.md)
