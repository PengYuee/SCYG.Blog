import { setActivePinia, createPinia } from "pinia"
import { beforeEach, describe, expect, it } from "vitest"
import { digestMarkdown, generateArticleSlug, purgeDataImages, useEditorDraftStore } from "@/stores/editor-draft"

describe("editor draft", () => {
  beforeEach(() => setActivePinia(createPinia()))
  it("generates a safe digest from markdown lines", () => {
    // Given: 带标题、链接和图片的 Markdown。
    const markdown = "# 标题\n\n[正文](https://example.test) ![图](data:image/png;base64,bad)"
    // When / Then: 摘要仅保留文本，不持久化 data URL。
    expect(digestMarkdown(markdown)).toBe("标题 正文")
  })
  it("generates a valid article link name from an English or Chinese title", () => {
    expect(generateArticleSlug("Vue 3 项目实战")).toBe("vue-3-xiang-mu-shi-zhan")
    expect(generateArticleSlug("  ")).toBe("")
  })

  it("keeps automatic names in sync until the author overrides them", () => {
    const store = useEditorDraftStore()
    store.updateTitle("First Guide")
    expect(store.draft.slug).toBe("first-guide")
    store.updateSlug("custom-guide")
    store.updateTitle("Second Guide")
    expect(store.draft.slug).toBe("custom-guide")
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
  it("maps the T3 article markdown model to create and patch requests", () => {
    // Given: T3 已解析的文章详情。
    const store = useEditorDraftStore()
    store.load({ id: 42, title: "映射", slug: "mapping", digest: "旧摘要", markdown: "# 正文", articleTypeId: 3, tagIds: [4], status: 1, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-07-12T00:00:00Z", updatedAt: null })

    // When: 编辑器生成创建与局部更新模型。
    const create = store.toCreate()
    const patch = store.toPatch()

    // Then: wire 之前保持领域字段，创建固定为草稿且局部更新不带版本控制字段。
    expect(create).toEqual({ title: "映射", slug: "mapping", markdown: "# 正文", digest: "正文", articleTypeId: 3, tagIds: [4], status: 1 })
    expect(patch).toEqual({ title: "映射", slug: "mapping", markdown: "# 正文", digest: "正文", articleTypeId: 3, tagIds: [4] })
    expect(store.version).toBe(1)
  })

  it("converges the local draft to the server resource after save", () => {
    const store = useEditorDraftStore()
    store.update({ title: "本地", slug: "local", markdown: "本地正文", articleTypeId: 1, tagIds: [] })
    store.beginSave()

    store.finishSave({ id: 9, title: "服务端", slug: "server", digest: "摘要", markdown: "服务端正文", articleTypeId: 2, tagIds: [3], status: 1, support: 0, comment: 0, visited: 0, version: 4, createdAt: "2026-07-12T00:00:00Z", updatedAt: null })

    expect(store.draft).toMatchObject({ title: "服务端", slug: "server", markdown: "服务端正文", articleTypeId: 2, tagIds: [3] })
    expect(store.version).toBe(4)
    expect(store.dirty).toBe(false)
    expect(store.saving).toBe(false)
  })
})
