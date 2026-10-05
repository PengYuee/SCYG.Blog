# 项目架构

SCYG.Blog 包含 Vue 浏览器应用、Go Blog API 与 Python Agent。浏览器已通过 HTTP API 使用博客内容和作者功能；Agent 有独立实现，但 Blog 到 Agent 的产品链路和前端 AI 页面尚未接通。

## 服务与合同

| 对象 | 职责与权威入口 |
| --- | --- |
| frontend | 公共阅读、真实登录、浏览器会话、作者编辑和 taxonomy 管理；[服务文档](services/frontend/README.md) |
| backend | 用户认证、内容业务、图片和 PostgreSQL 持久化；REST 由 [OpenAPI](../backend/api/openapi.yaml)定义；[现行架构](services/backend/architecture/current-state-architecture.zh-CN.md) |
| agent | gRPC、HTTP/SSE、Worker、Recipe/Runtime 执行与恢复；[服务架构](services/agent/architecture.md) |
| contracts | [protobuf/gRPC 源合同](../contracts/proto/)；合同存在不代表调用端已实现 |

当前链路是“浏览器 → Blog HTTP API → Blog 数据库与图片文件”。Agent 独立连接自己的 PostgreSQL、Redis 和模型 Provider。现有 Agent HTTP/SSE 不等于浏览器已有 AI 入口。

## 数据与身份归属

Blog 拥有用户、文章、分类、标签、图片元数据及文件生命周期。Agent 不读取 Blog 数据库，不持有 Blog DSN；跨服务调用经过合同边界。

Agent truth 保存 Run、状态、lease、持久事件与命令；LangGraph checkpoint 保存执行恢复状态，二者在同一应用数据库使用同一账号和 DSN。Redis 保存当前流式路径的瞬时事件，不替代 PostgreSQL 业务真相。游标区别见 [Agent 流式合同](services/agent/streaming.md)。

浏览器使用 Blog 短期 Bearer JWT；本地退出和过期清理不代表有服务端 refresh/logout API。Agent Run 鉴权与博客登录是不同边界，不直接混用令牌。

## 实现与验收边界

管理后台只保留不可用路由边界；前端支持桌面 Microsoft Edge，布局下限 1024px。Agent 当前工作区同时装配 Recipe AgentRunner 与 SIMPLE/DEEP Router，不能将重构目标写成旧运行时已完全移除。

运行与检查见[开发指南](development.md)。本次整理依据源码、配置和合同，不宣称真实数据库、Provider、容器或浏览器端到端验收已通过。正式需求和设计须标注目标状态，不能用实施计划覆盖现行说明。
