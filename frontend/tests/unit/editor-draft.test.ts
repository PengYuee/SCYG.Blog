import { setActivePinia, createPinia } from "pinia"
import { beforeEach, describe, expect, it } from "vitest"
import { digestMarkdown, purgeDataImages, useEditorDraftStore } from "@/stores/editor-draft"

describe("editor draft", () => {
  beforeEach(() => setActivePinia(createPinia()))
  it("generates a safe digest from markdown lines", () => {
    // Given: 带标题、链接和图片的 Markdown。
    const markdown = "# 标题\n\n[正文](https://example.test) ![图](data:image/png;base64,bad)"
    // When / Then: 摘要仅保留文本，不持久化 data URL。
    expect(digestMarkdown(markdown)).toBe("标题 正文")
  })
  it("prevents duplicate submit while saving", () => {
    // Given: 空闲草稿仓库。
    const store = useEditorDraftStore()
    // When / Then: 首次提交成功占用，第二次被拒绝。
    expect(store.beginSave()).toBe(true)
    expect(store.beginSave()).toBe(false)
  })
  it("purges data images at the persistence boundary", () => {
    // Given: 正文同时包含 Base64 图片与普通远程图片。
    const markdown = "before ![local](data:image/png;base64,AAAA) after ![remote](https://cdn.test/a.png)"
    // When / Then: 仅不允许持久化的 Base64 图片被移除。
    expect(purgeDataImages(markdown)).toBe("before  after ![remote](https://cdn.test/a.png)")
  })
  it("maps the T3 article markdown model back to a write request", () => {
    // Given: T3 已解析的文章详情。
    const store = useEditorDraftStore()
    store.load({ id: 42, title: "映射", slug: "mapping", digest: "旧摘要", markdown: "# 正文", articleTypeId: 3, tagIds: [4], status: 1, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-07-12T00:00:00Z", updatedAt: null })
    // When: 编辑器生成 T3 ArticleWrite。
    const write = store.toWrite()
    // Then: Markdown、标题与字典标识保持领域字段，不改写为 HTML。
    expect(write).toEqual({ title: "映射", markdown: "# 正文", digest: "正文", articleTypeId: 3, tagIds: [4] })
  })
})
