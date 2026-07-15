## 2026-07-13 Todo 9

- 工作树开始时已有大量 backend 与 Todo 8 frontend 并行修改，其中 `api-adapters.test.ts`、`article.ts`、`api-services.ts`、`ArticleEditorView.vue` 等目标文件已包含未提交变更；提交必须按 hunk 精确暂存，不能整文件纳入。
- 完整 unit 当前有 3 个并行基线失败，完整 component 有 15 个并行公共 UI 失败；Todo 9 targeted unit/component 均重复通过。
- `typecheck` 与 `build` 被并行文件 `src/theme/theme.ts:49` 的 TS4111 阻断；Todo 9 文件没有被 vue-tsc 报错。
- TypeScript/Vue LSP 未安装且用户此前已拒绝安装，因此逐文件 `lsp_diagnostics` 只能记录工具不可用。

## 2026-07-13 Todo 8

- `go test ./...` 仍被并行基线 `internal/contracttest/documentation_test.go` 与 `internal/reviewtest/review_test.go` 的 `ARCH_MUTABLE_GLOBAL` 拒绝；Todo 8 自身的 `ARCH_UNIVERSAL_API` 已通过窄接口拆分消除。
- Todo 8 必改的 `app.go`、`cleanup.go`、`bootstrap_test.go` 与未跟踪 `app_startup_test.go` 含前序启动日志差异；实现已安全叠加，提交时必须检查 staged patch，不能整文件误归属。
- 首版竞态 barrier 只暂停 cleanup，导致文件先被删除、引用在文件预检阶段返回，无法到达行锁；已改为 reference-insert 与 cleanup-delete 双 barrier，并连续 10 次通过。
- 独立复核最初发现“部分失败后保存成功会清空未引用 pending”的缺口；已通过 `retainForCancel`、单元测试和组件测试修复。
- Edge 复核发现 catch-all 曾让上传后媒体 GET 返回 404；已在 task-9 spec 中增加真实 JPEG/PNG 媒体 fixture 和零浏览器错误断言。
