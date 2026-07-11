import { readFile } from "node:fs/promises"
import { describe, expect, it } from "vitest"

describe("T11 author contract", () => {
  it("routes explicitly enabled fake authoring to the rich editor", async () => {
    // Given: T9 的作者路由模块源码。
    const source = await readFile("src/router/modules/author.ts", "utf8")
    // When / Then: T11 视图仅由显式 Fake 边界选择。
    expect(source).toContain("ArticleEditorView.vue")
    expect(source).toContain("fakeAuthorEnabled")
  })

  it("keeps production authoring on the truthful unavailable view", async () => {
    // Given: T9 的作者路由模块源码。
    const source = await readFile("src/router/modules/author.ts", "utf8")
    // When / Then: 生产分支仍保留不可用视图，且不直接构造网络适配器。
    expect(source).toContain("PublicNotFoundView.vue")
    expect(source).not.toMatch(/createArticleApi|createArticleTypeApi|createTagApi/)
  })
})
