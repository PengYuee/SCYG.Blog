import { describe, expect, it, vi } from "vitest"
import { createArticleApi } from "@/request/api/article"
import { createArticleTypeApi } from "@/request/api/article-type"
import { createTagApi } from "@/request/api/tag"
import type { HttpTransport } from "@/request/transport"

const article = {
  id: 7, title: "T", slug: "typed", digest: "D", content: "M", article_type_id: 2, tag_ids: [3], status: 2,
  support: 0, comment: 0, visited: 0, version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null,
}
const page = { number: 1, size: 20, total_items: 1, total_pages: 1 }

/** 创建隔离的类型化传输层替身。 */
function client(): HttpTransport {
  return { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
}

describe("current v1 API contract mappings", () => {
  it("maps article list search filters and zero-based pagination to v1", async () => {
    // Given: 当前 v1 文章分页响应。
    const transport = client()
    vi.mocked(transport.get).mockResolvedValue({ data: { items: [article], page } })
    const api = createArticleApi(transport)

    // When: 带搜索词与分类筛选读取第二页。
    const result = await api.list({ q: "  typed  ", tagId: 3, articleTypeId: 2, pageModel: { pageIndex: 1, pageSize: 20 } })

    // Then: 查询词被清理，页码映射为从一开始，响应映射回从零开始。
    expect(transport.get).toHaveBeenCalledWith("/api/v1/articles", { params: { page: 2, page_size: 20, article_type_id: 2, tag_id: 3, q: "typed" } })
    expect(result).toEqual({ items: [{ id: 7, title: "T", slug: "typed", digest: "D", markdown: "M", articleTypeId: 2, tagIds: [3], status: 2, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }], pageIndex: 0, pageSize: 20, totalItems: 1, totalPages: 1 })
  })

  it("maps article detail from the current v1 route", async () => {
    // Given: 当前 v1 文章详情响应。
    const transport = client()
    vi.mocked(transport.get).mockResolvedValue({ data: article })

    // When: 按编号读取文章详情。
    const result = await createArticleApi(transport).detail(7)

    // Then: 使用当前详情路由并保留 Markdown 内容。
    expect(transport.get).toHaveBeenCalledWith("/api/v1/articles/7")
    expect(result.markdown).toBe("M")
  })

  it("reads every article-type page and preserves server URL image mapping", async () => {
    // Given: 两页当前 v1 分类响应。
    const transport = client()
    vi.mocked(transport.get)
      .mockResolvedValueOnce({ data: { items: [{ id: 2, name: "Tech", image: "/images/tech.png", meun: 1, version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page: { ...page, size: 100, total_items: 2, total_pages: 2 } } })
      .mockResolvedValueOnce({ data: { items: [{ id: 3, name: "Life", image: null, meun: 2, version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page: { ...page, number: 2, size: 100, total_items: 2, total_pages: 2 } } })

    // When: 使用清理后的关键词读取完整分类。
    const result = await createArticleTypeApi(transport, "https://api.test").list(" tech ")

    // Then: 请求从一开始逐页读取，图片仍按服务地址映射。
    expect(transport.get).toHaveBeenNthCalledWith(1, "/api/v1/article-types", { params: { page_size: 100, q: "tech", page: 1 } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/api/v1/article-types", { params: { page_size: 100, q: "tech", page: 2 } })
    expect(result).toEqual([{ id: 2, name: "Tech", imageUrl: "https://api.test/images/tech.png", menu: 1, version: 1 }, { id: 3, name: "Life", imageUrl: null, menu: 2, version: 1 }])
  })

  it("creates an article type from the unwrapped versioned resource", async () => {
    // Given: 当前 v1 创建接口返回未包裹的分类资源。
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: { id: 4, name: "Tech", image: "/images/tech.png", meun: 3, version: 7, created_at: "2026-07-11T00:00:00Z", updated_at: null } })

    // When: 作者使用领域字段创建分类。
    const result = await createArticleTypeApi(transport, "https://api.test").create({ name: "Tech", image: "/images/tech.png", menu: 3 })

    // Then: 请求精确映射后端 meun 字段，并返回保留版本的作者分类。
    expect(transport.post).toHaveBeenCalledWith("/api/v1/article-types", { name: "Tech", image: "/images/tech.png", meun: 3 })
    expect(result).toEqual({ id: 4, name: "Tech", imageUrl: "https://api.test/images/tech.png", menu: 3, version: 7 })
  })

  it("deletes an article type with its strong version entity tag", async () => {
    // Given: 当前 v1 删除接口返回无响应体的成功结果。
    const transport = client()
    vi.mocked(transport.delete).mockResolvedValue({ data: undefined })

    // When: 作者提交分类标识及当前版本。
    await createArticleTypeApi(transport, "https://api.test").delete({ id: 4, version: 7 })

    // Then: 版本以带双引号的强 If-Match 实体标签发送。
    expect(transport.delete).toHaveBeenCalledWith("/api/v1/article-types/4", { headers: { "If-Match": "\"7\"" } })
  })

  it("reads every tag page from the current v1 route", async () => {
    // Given: 两页当前 v1 标签响应。
    const transport = client()
    vi.mocked(transport.get)
      .mockResolvedValueOnce({ data: { items: [{ id: 3, name: "Vue", version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page: { ...page, size: 100, total_items: 2, total_pages: 2 } } })
      .mockResolvedValueOnce({ data: { items: [{ id: 4, name: "TypeScript", version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page: { ...page, number: 2, size: 100, total_items: 2, total_pages: 2 } } })

    // When: 使用清理后的关键词读取完整标签。
    const result = await createTagApi(transport).list(" vue ")

    // Then: 请求从一开始逐页读取并保留作者并发写入所需版本。
    expect(transport.get).toHaveBeenNthCalledWith(1, "/api/v1/tags", { params: { page_size: 100, q: "vue", page: 1 } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/api/v1/tags", { params: { page_size: 100, q: "vue", page: 2 } })
    expect(result).toEqual([{ id: 3, name: "Vue", version: 1 }, { id: 4, name: "TypeScript", version: 1 }])
  })

  it("creates a tag from the unwrapped versioned resource", async () => {
    // Given: 当前 v1 创建接口返回未包裹的标签资源。
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: { id: 5, name: "Vue", version: 3, created_at: "2026-07-11T00:00:00Z", updated_at: null } })

    // When: 作者按名称创建标签。
    const result = await createTagApi(transport).create("Vue")

    // Then: 请求体只包含名称，并返回保留版本的作者标签。
    expect(transport.post).toHaveBeenCalledWith("/api/v1/tags", { name: "Vue" })
    expect(result).toEqual({ id: 5, name: "Vue", version: 3 })
  })

  it("rejects a created tag resource without a version", async () => {
    // Given: 创建接口返回缺少并发版本的非法资源。
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: { id: 5, name: "Vue", created_at: "2026-07-11T00:00:00Z", updated_at: null } })

    // When: 作者适配器解析创建响应。
    const result = createTagApi(transport).create("Vue")

    // Then: 信任边界拒绝缺少版本的资源。
    await expect(result).rejects.toThrow()
  })

  it("deletes a tag with its strong version entity tag", async () => {
    // Given: 当前 v1 删除接口返回无响应体的成功结果。
    const transport = client()
    vi.mocked(transport.delete).mockResolvedValue({ data: undefined })

    // When: 作者提交标签标识及当前版本。
    await createTagApi(transport).delete({ id: 5, version: 3 })

    // Then: 版本以带双引号的强 If-Match 实体标签发送。
    expect(transport.delete).toHaveBeenCalledWith("/api/v1/tags/5", { headers: { "If-Match": "\"3\"" } })
  })
  it("maps management article creation to the complete snake_case body", async () => {
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: article })

    const result = await createArticleApi(transport).create({ title: "T", slug: "typed", digest: "D", markdown: "M", articleTypeId: 2, tagIds: [3], status: 1 })

    expect(transport.post).toHaveBeenCalledWith("/api/v1/manage/articles", { title: "T", slug: "typed", digest: "D", content: "M", article_type_id: 2, tag_ids: [3], status: 1 })
    expect(result).toMatchObject({ id: 7, slug: "typed", markdown: "M", version: 1 })
  })

  it("reads and patches management articles with the resource version", async () => {
    const transport = client()
    vi.mocked(transport.get).mockResolvedValue({ data: article })
    vi.mocked(transport.patch).mockResolvedValue({ data: { ...article, version: 2, content: "updated" } })
    const api = createArticleApi(transport)

    await api.manageDetail(7)
    const result = await api.update({ id: 7, version: 1, changes: { title: "T2", slug: "typed", digest: "D2", markdown: "updated", articleTypeId: 2, tagIds: [3] } })

    expect(transport.get).toHaveBeenCalledWith("/api/v1/manage/articles/7")
    expect(transport.patch).toHaveBeenCalledWith("/api/v1/manage/articles/7", { title: "T2", slug: "typed", digest: "D2", content: "updated", article_type_id: 2, tag_ids: [3] }, { headers: { "If-Match": "\"1\"" } })
    expect(result).toMatchObject({ markdown: "updated", version: 2 })
  })

  it("maps article lifecycle actions to versioned management endpoints", async () => {
    const transport = client()
    vi.mocked(transport.delete).mockResolvedValue({ data: undefined })
    vi.mocked(transport.post).mockResolvedValue({ data: article })
    const api = createArticleApi(transport)

    await api.deleteManage({ id: 7, version: 3 })
    await api.publish({ id: 7, version: 3 })
    await api.archive({ id: 7, version: 4 })

    expect(transport.delete).toHaveBeenCalledWith("/api/v1/manage/articles/7", { headers: { "If-Match": "\"3\"" } })
    expect(transport.post).toHaveBeenNthCalledWith(1, "/api/v1/manage/articles/7/publish", undefined, { headers: { "If-Match": "\"3\"" } })
    expect(transport.post).toHaveBeenNthCalledWith(2, "/api/v1/manage/articles/7/archive", undefined, { headers: { "If-Match": "\"4\"" } })
  })

  it("gets and patches taxonomy resources through their v1 routes", async () => {
    const transport = client()
    const type = { id: 4, name: "Tech", image: null, meun: 3, version: 7, created_at: "2026-07-11T00:00:00Z", updated_at: null }
    const tag = { id: 5, name: "Vue", version: 3, created_at: "2026-07-11T00:00:00Z", updated_at: null }
    vi.mocked(transport.get).mockResolvedValueOnce({ data: type }).mockResolvedValueOnce({ data: tag })
    vi.mocked(transport.patch).mockResolvedValueOnce({ data: type }).mockResolvedValueOnce({ data: tag })

    const typeApi = createArticleTypeApi(transport, "https://api.test")
    const tagApi = createTagApi(transport)
    await typeApi.detail(4)
    await typeApi.update({ id: 4, version: 7, changes: { name: "Backend", menu: 4 } })
    await tagApi.detail(5)
    await tagApi.update({ id: 5, version: 3, changes: { name: "Golang" } })

    expect(transport.get).toHaveBeenNthCalledWith(1, "/api/v1/article-types/4")
    expect(transport.patch).toHaveBeenNthCalledWith(1, "/api/v1/article-types/4", { name: "Backend", meun: 4 }, { headers: { "If-Match": "\"7\"" } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/api/v1/tags/5")
    expect(transport.patch).toHaveBeenNthCalledWith(2, "/api/v1/tags/5", { name: "Golang" }, { headers: { "If-Match": "\"3\"" } })
  })
})
