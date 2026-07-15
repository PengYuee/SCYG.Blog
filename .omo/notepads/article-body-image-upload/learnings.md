## 2026-07-13 Todo 9

- `md-editor-v3` 6.5.3 在 Edge 中以 CodeMirror contenteditable textbox 呈现，链接文本视觉折叠为 `...`；浏览器验收应通过预览图片 `src` 与最终文章请求 body 验证完整远程 URL。
- 文件 input 的 `setInputFiles` 只等待 DOM 事件分发，不等待异步 `onUploadImg`；保存或离页前必须等待预览图片进入 DOM。
- 离页清理必须通过 Vue Router 的 SPA 导航验证；`page.goto` 会触发文档级导航并可能中止卸载阶段的异步 DELETE。
- 图片生命周期必须保存服务端 `id`，URL 只负责 Markdown 展示；DELETE 永远不能由 URL basename 推导。
- `Promise.allSettled` 允许多图上传部分失败后保留成功项的 pending 记录，同时禁止编辑器插入不完整 URL 集合。

## 2026-07-13 Todo 8

- 清理候选查询的 `FOR UPDATE SKIP LOCKED` 只在查询事务内有效；每条执行前必须再次 `FindForUpdate` 并复核状态/期限，orphaned 还要在同一图片行锁内计数引用。
- 文件删除与 DB 删除无法原子提交，安全顺序是“文件删除成功或不存在 -> DeleteMetadata”；DB 失败保留行，下一轮依赖文件删除幂等收敛。
- worker Stop 超时不能继续关闭 DB；App 保持可重试关闭状态，下一次 Shutdown 用新 context 等待 worker 后再关闭数据库。
- 根模块架构规则禁止两个以上方法的接口；清理存储按最终删除、temp 枚举、temp 删除拆成单方法窄端口。
- Start 失败清理与正常 Shutdown 必须共享“worker 退出是关闭 DB 的前置条件”不变量；Stop 失败时应记录 startFailed 但保留 stopped=false，禁止重启并允许继续 Shutdown。
- 清理在删除 DB 行前已经删除文件，因此直接先暂停清理会让重新引用在文件预检阶段提前失败；确定性竞争需要先在 Article insert barrier 暂停引用，再让清理持有图片行锁，最后释放引用 barrier 观察真实锁等待。
- 部分失败批次的成功 URL 必须单独标记为 `retainForCancel`；这样文章保存成功时不立即 DELETE，但后续离页仍能按 id 取消未写入 Markdown 的资源。
- `ImageLifecycle.cancel()` 返回清理是否全部成功，页面据此显示中文 TTL 兜底提示；本地 pending 无论 DELETE 成败都立即收敛。
- Edge 图片验收不能只断言 `<img>` 挂载；受控媒体 GET 必须返回匹配扩展名的真实 JPEG/PNG 字节，并断言 `complete`、`naturalWidth` 与 console/page/request 错误集合。
