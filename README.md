# SCYG.Blog

SCYG Blog 是包含浏览器应用、博客 API 和独立 Agent 服务的智能博客项目。

## 当前能力

- 博客：公共文章列表、详情、分类与标签发现。
- 作者：真实登录、浏览器会话、文章编辑、分类标签管理和图片操作。
- AI：作者工作台提供站内搜索、写作草稿、润色与独立问答，访客可使用公共 AI 搜索；浏览器通过 Blog HTTP/SSE 调用 Agent，支持补充要求、选择参考与草稿确认。使用边界见 [AI 工作台与公共搜索](docs/services/frontend/ai.md)。
- 使用范围：前端面向桌面 Microsoft Edge，布局下限 1024px；管理后台暂未开放。

## 快速开始

按[项目开发指南](docs/development.md)准备 Blog PostgreSQL，迁移并启动后端，再配置并启动前端。博客主路径不要求先运行 Agent；使用 AI 时还需启用 Blog 的 Agent 集成，并准备 Agent PostgreSQL、Redis 和可用 Provider，初始化与启动见 [Agent 入口](docs/services/agent/README.md)。

启动后先验证公共阅读，再使用本地活动账号登录作者页面；保存测试内容会修改数据库。配置检查与实际业务验收不能互相替代。

## 仓库与文档

| 目录 | 职责 | 正式文档 |
| --- | --- | --- |
| frontend/ | Vue 浏览器应用 | [前端](docs/services/frontend/README.md) |
| backend/ | Go 认证、内容和图片 API | [后端](docs/services/backend/README.md) |
| agent/ | Python Agent 执行服务 | [Agent](docs/services/agent/README.md) |
| contracts/ | protobuf/gRPC 源合同 | [协议目录](contracts/proto/) |
| docs/modules/ | 跨服务业务流程、规则、状态与失败语义 | [业务模块](docs/modules/README.md) |
| docs/ | 正式项目说明 | [文档总导航](docs/README.md) |

整体技术关系见[项目架构](docs/architecture.md)，完整业务流程从[业务模块](docs/modules/README.md)进入，开发检查按[改动范围](docs/development.md)选择。任务计划、草稿、日志和原始报告属于本地工作材料，不作为现行能力依据；归属规则见[根 AGENTS](AGENTS.md)。

## 许可证

[MIT](LICENSE)
