import type { AxiosInstance } from "axios"
import { describe, expect, it, vi } from "vitest"
import { createArticleApi } from "@/request/api/article"
import { createArticleTypeApi } from "@/request/api/article-type"
import { createTagApi } from "@/request/api/tag"

const article = {
  id: 7, title: "T", slug: "typed", digest: "D", content: "M", article_type_id: 2, tag_ids: [3], status: 2,
  support: 0, comment: 0, visited: 0, version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null,
}

function client(): AxiosInstance {
  return { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() } as never
}

describe("legacy API contract mappings", () => {
  it("maps every article request exactly", async () => {
    // Given: an isolated transport and valid response fixtures.
    const transport = client()
    vi.mocked(transport.get).mockResolvedValueOnce({ data: { items: [], page: { number: 1, size: 20, total_items: 0, total_pages: 0 } } }).mockResolvedValueOnce({ data: article })
    vi.mocked(transport.post).mockResolvedValueOnce({ data: { success: true } }).mockResolvedValueOnce({ data: { url: "/uploads/x.png" } })
    vi.mocked(transport.put).mockResolvedValue({ data: { success: true } })
    vi.mocked(transport.delete).mockResolvedValue({ data: { success: true } })
    const api = createArticleApi(transport, "https://api.test")
    // When: every matrix operation is invoked.
    await api.list({ tagId: 3, articleTypeId: 2, pageModel: { pageIndex: 1, pageSize: 20 } })
    await api.detail(7)
    await api.create({ title: "T", markdown: "M", digest: "D", tagIds: [3], articleTypeId: 2 })
    await api.update({ id: 7, title: "T", markdown: "M", digest: "D", tagIds: [3], articleTypeId: 2 })
    const image = new File(["x"], "x.png", { type: "image/png" })
    await api.uploadImage(image)
    await api.deleteImage("x.png")
    // Then: names and body mappings match the legacy contract exactly.
    expect(transport.get).toHaveBeenCalledWith("/Article/GetArticleList", { params: { tagId: 3, articleTypeId: 2, pageModel: { pageIndex: 1, pageSize: 20 } } })
    expect(transport.get).toHaveBeenCalledWith("/Article/GetArticle", { params: { id: 7 } })
    expect(transport.post).toHaveBeenCalledWith("/Article/CreateArticle", { title: "T", body: "M", digest: "D", tagIds: [3], articleTypeId: 2 })
    expect(transport.put).toHaveBeenCalledWith("/Article/UpdateArticle", { id: 7, title: "T", body: "M", digest: "D", tagIds: [3], articleTypeId: 2 })
    const upload = vi.mocked(transport.post).mock.calls[1]
    expect(upload?.[0]).toBe("/Article/UpLoadArticleImage")
    expect(upload?.[1]).toBeInstanceOf(FormData)
    expect(upload?.[1] instanceof FormData ? upload[1].get("image") : null).toBe(image)
    expect(upload?.[2]).toBeUndefined()
    expect(transport.delete).toHaveBeenCalledWith("/Article/DeleteArticleImage", { params: { imageName: "x.png" } })
  })

  it("maps taxonomy dictionary and mutation requests", async () => {
    // Given: successful dictionary and mutation transport results.
    const transport = client()
    vi.mocked(transport.get).mockResolvedValue({ data: { items: [], page: { number: 1, size: 20, total_items: 0, total_pages: 0 } } })
    vi.mocked(transport.post).mockResolvedValue({ data: { success: true } })
    vi.mocked(transport.delete).mockResolvedValue({ data: { success: true } })
    // When: category and tag matrix methods run.
    const types = createArticleTypeApi(transport, "https://api.test")
    await types.list("tech"); await types.create({ name: "Tech", image: null, menu: 1 }); await types.delete(2)
    const tags = createTagApi(transport)
    await tags.list("vue"); await tags.create("Vue"); await tags.delete(3)
    // Then: legacy paths and payload styles remain exact.
    expect(transport.get).toHaveBeenNthCalledWith(1, "/ArticleType/GetArticleTypeDic", { params: { filter: "tech" } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/Tag/GetTagDic", { params: { filter: "vue" } })
    const typeCreate = vi.mocked(transport.post).mock.calls[0]
    expect(typeCreate?.[0]).toBe("/ArticleType/CreateArticleType")
    expect(typeCreate?.[1]).toBeInstanceOf(FormData)
    expect(transport.post).toHaveBeenNthCalledWith(2, "/Tag/CreateTag", { name: "Vue" })
    expect(transport.delete).toHaveBeenCalledWith("/ArticleType/DeleteArticleType", { params: { id: 2 } })
    expect(transport.delete).toHaveBeenCalledWith("/Tag/DeleteTag", { params: { id: 3 } })
  })
})
