# ADR-010：Scalar 自托管浏览器资产版本

**状态：** 已接受

## 决策

绑定架构曾计划采用 `@scalar/api-reference` `1.49.3`。该版本在同步资产时不能解析为可验证的发布物，因此以 `1.62.5` 取代该绑定版本。此 ADR 取代 ADR-002 中的 Scalar 版本约束，不改变其自托管要求。

当前资产的来源、哈希与许可证记录在 [`manifest.json`](../../internal/transport/rest/apidocs/assets/manifest.json)：上游 npm tarball、`release-2026-07-08-8cbf82d` 发布、`standalone.js` SHA-256 `b5edb255af0e112c4530c41da2350fca28a72f4388694880ba50acb442fda88f`，许可证为 MIT。随资产保留的 [`LICENSE.scalar`](../../internal/transport/rest/apidocs/assets/LICENSE.scalar) 是许可证副本。

## 后补治理例外

**取代：** supersedes ADR-002 中已无法解析的 Scalar `1.49.3` 绑定记录。

**后补理由：** 绑定版本变更已发生后才补充本 ADR，用于将可验证发布物的纠偏决策显式化。

**哈希：** SHA-256 `b5edb255af0e112c4530c41da2350fca28a72f4388694880ba50acb442fda88f`，对应 `standalone.js`。

**许可证：** MIT，许可证副本见 [`LICENSE.scalar`](../../internal/transport/rest/apidocs/assets/LICENSE.scalar)。

**回退：** 仅选择可验证发布物，并同步来源、哈希与许可证记录。

**非改写理由：** 共享历史包含用户提交，禁止通过 rewrite 或 rebase 补造 ADR 祖先关系；后补 ADR 是可审计的显式纠偏，保留原始提交历史。

## 兼容性与回退

`1.62.5` 继续以嵌入的 `standalone.js` 渲染本地 `/openapi.yaml`，不要求运行时 CDN、网络抓取或新增 Go 依赖。若该版本出现浏览器兼容性问题，回退必须选择可验证的 Scalar 发布物；同步其许可证和 manifest 中的版本、来源、完整性字段，并以新的 ADR 记录原因。不得修改已验证的 `scalar.js` 字节来伪造回退。
