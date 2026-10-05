import { describe, expect, it, vi } from "vitest"
import { createAuthApi } from "@/request/api/auth"
import { createArticleApi } from "@/request/api/article"
import { createArticleTypeApi } from "@/request/api/article-type"
import { createPublicTaxonomyApi } from "@/request/api/public-taxonomy"
import { createTagApi } from "@/request/api/tag"
import type { HttpTransport } from "@/request/transport"

const articleType = { id: 2, name: "Tech", image: "/images/tech.png" }
const article = { id: 7, title: "T", slug: "typed", digest: "D", content: "M", articleTypeId: 2, articleType, tagIds: [3], status: 2, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }
const page = { number: 1, size: 20, totalItems: 1, totalPages: 1 }
const managedType = { id: 4, name: "Tech", image: "/images/tech.png", menu: 3, version: 7, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }
const managedTag = { id: 5, name: "Vue", version: 3, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }

const publicType = { id: 2, name: "Tech", image: "/images/tech.png", articleCount: 4 }
const publicTag = { id: 5, name: "Vue", articleCount: 3 }


function client(): HttpTransport { return { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() } }

describe("current v1 API contract mappings", () => {
  it("posts trimmed credentials and parses a Bearer session", async () => {
    const transport = client()
    const response = { accessToken: "signed-token", tokenType: "Bearer", expiresAt: "2026-09-23T01:00:00Z" }
    vi.mocked(transport.post).mockResolvedValue({ data: response })

    await expect(createAuthApi(transport).login({ username: " author ", password: "secret" })).resolves.toEqual(response)
    expect(transport.post).toHaveBeenCalledWith("/api/v1/auth/login", { username: "author", password: "secret" })
  })
  it("reads public taxonomy from non-management routes and maps counts", async () => {
    const transport = client()
    vi.mocked(transport.get)
      .mockResolvedValueOnce({ data: { items: [publicType], page: { number: 1, size: 100, totalItems: 1, totalPages: 1 } } })
      .mockResolvedValueOnce({ data: { items: [publicTag], page: { number: 1, size: 100, totalItems: 1, totalPages: 1 } } })
    const api = createPublicTaxonomyApi(transport, "https://api.test")
    await expect(api.listArticleTypes(" tech ")).resolves.toEqual([{ id: 2, name: "Tech", imageUrl: "https://api.test/images/tech.png", menu: 0, articleCount: 4 }])
    await expect(api.listTags(" vue ")).resolves.toEqual([{ id: 5, name: "Vue", articleCount: 3 }])
    expect(transport.get).toHaveBeenNthCalledWith(1, "/api/v1/article-types", { params: { pageSize: 100, q: "tech", page: 1 } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/api/v1/tags", { params: { pageSize: 100, q: "vue", page: 1 } })
  })
  it("maps article list search filters and zero-based pagination to v1", async () => {
    const transport = client()
    vi.mocked(transport.get).mockResolvedValue({ data: { items: [article], page } })
    const result = await createArticleApi(transport).list({ q: "  typed  ", tagId: 3, articleTypeId: 2, pageModel: { pageIndex: 1, pageSize: 20 } })
    expect(transport.get).toHaveBeenCalledWith("/api/v1/articles", { params: { page: 2, pageSize: 20, sort: "-createdAt", articleTypeId: 2, tagId: 3, q: "typed" } })
    expect(result.items[0]).toMatchObject({ id: 7, markdown: "M", articleTypeId: 2, articleType: { id: 2, name: "Tech", imageUrl: "/images/tech.png" } })
  })

  it("maps article detail from the current v1 route", async () => {
    const transport = client()
    vi.mocked(transport.get).mockResolvedValue({ data: article })
    const result = await createArticleApi(transport).detail(7)
    expect(transport.get).toHaveBeenCalledWith("/api/v1/articles/7")
    expect(result.markdown).toBe("M")
  })
  it("maps management article list and detail to generated read routes", async () => {
    const transport = client()
    vi.mocked(transport.get)
      .mockResolvedValueOnce({ data: { items: [article], page } })
      .mockResolvedValueOnce({ data: article })
    const api = createArticleApi(transport)
    const list = await api.manageList({ q: "  typed  ", tagId: 3, articleTypeId: 2, pageModel: { pageIndex: 1, pageSize: 20 } })
    const detail = await api.manageDetail(7)
    expect(transport.get).toHaveBeenNthCalledWith(1, "/api/v1/manage/articles", { params: { page: 2, pageSize: 20, sort: "-createdAt", articleTypeId: 2, tagId: 3, q: "typed" } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/api/v1/manage/articles/7")
    expect(list.items[0].markdown).toBe("M")
    expect(detail.id).toBe(7)
  })

  it("reads every managed article-type page and preserves image mapping", async () => {
    const transport = client()
    vi.mocked(transport.get)
      .mockResolvedValueOnce({ data: { items: [{ ...managedType, id: 2 }], page: { ...page, size: 100, totalItems: 2, totalPages: 2 } } })
      .mockResolvedValueOnce({ data: { items: [{ ...managedType, id: 3, name: "Life", image: null, menu: 2 }], page: { ...page, number: 2, size: 100, totalItems: 2, totalPages: 2 } } })
    const result = await createArticleTypeApi(transport, "https://api.test").list(" tech ")
    expect(transport.get).toHaveBeenNthCalledWith(1, "/api/v1/manage/article-types", { params: { pageSize: 100, q: "tech", page: 1 } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/api/v1/manage/article-types", { params: { pageSize: 100, q: "tech", page: 2 } })
    expect(result).toEqual([{ id: 2, name: "Tech", imageUrl: "https://api.test/images/tech.png", menu: 3, version: 7 }, { id: 3, name: "Life", imageUrl: null, menu: 2, version: 7 }])
  })

  it("creates and deletes managed article types with version tags", async () => {
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: managedType })
    const result = await createArticleTypeApi(transport, "https://api.test").create({ name: "Tech", image: "/images/tech.png", menu: 3 })
    expect(transport.post).toHaveBeenCalledWith("/api/v1/manage/article-types", { name: "Tech", image: "/images/tech.png", menu: 3 })
    expect(result).toMatchObject({ id: 4, menu: 3, version: 7 })
    await createArticleTypeApi(transport, "https://api.test").delete({ id: 4, version: 7 })
    expect(transport.delete).toHaveBeenCalledWith("/api/v1/manage/article-types/4", { headers: { "If-Match": "\"7\"" } })
  })

  it("reads every managed tag page and preserves version", async () => {
    const transport = client()
    vi.mocked(transport.get)
      .mockResolvedValueOnce({ data: { items: [{ ...managedTag, id: 3 }], page: { ...page, size: 100, totalItems: 2, totalPages: 2 } } })
      .mockResolvedValueOnce({ data: { items: [{ ...managedTag, id: 4, name: "TypeScript" }], page: { ...page, number: 2, size: 100, totalItems: 2, totalPages: 2 } } })
    const result = await createTagApi(transport).list(" vue ")
    expect(transport.get).toHaveBeenNthCalledWith(1, "/api/v1/manage/tags", { params: { pageSize: 100, q: "vue", page: 1 } })
    expect(transport.get).toHaveBeenNthCalledWith(2, "/api/v1/manage/tags", { params: { pageSize: 100, q: "vue", page: 2 } })
    expect(result).toEqual([{ id: 3, name: "Vue", version: 3 }, { id: 4, name: "TypeScript", version: 3 }])
  })

  it("creates and deletes managed tags", async () => {
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: managedTag })
    const result = await createTagApi(transport).create("Vue")
    expect(transport.post).toHaveBeenCalledWith("/api/v1/manage/tags", { name: "Vue" })
    expect(result).toEqual({ id: 5, name: "Vue", version: 3 })
    await createTagApi(transport).delete({ id: 5, version: 3 })
    expect(transport.delete).toHaveBeenCalledWith("/api/v1/manage/tags/5", { headers: { "If-Match": "\"3\"" } })
  })

  it("rejects a created tag resource without a version", async () => {
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: { id: 5, name: "Vue", createdAt: "2026-07-11T00:00:00Z", updatedAt: null } })
    await expect(createTagApi(transport).create("Vue")).rejects.toThrow()
  })

  it("maps management article creation and patch to camelCase body", async () => {
    const transport = client()
    vi.mocked(transport.post).mockResolvedValue({ data: article })
    vi.mocked(transport.patch).mockResolvedValue({ data: { ...article, version: 2, content: "updated" } })
    const api = createArticleApi(transport)
    const created = await api.create({ title: "T", slug: "typed", digest: "D", markdown: "M", articleTypeId: 2, tagIds: [3], status: 1 })
    const updated = await api.update({ id: 7, version: 1, changes: { title: "T2", slug: "typed", digest: "D2", markdown: "updated", articleTypeId: 2, tagIds: [3] } })
    expect(transport.post).toHaveBeenCalledWith("/api/v1/manage/articles", { title: "T", slug: "typed", digest: "D", content: "M", articleTypeId: 2, tagIds: [3], status: 1 })
    expect(transport.patch).toHaveBeenCalledWith("/api/v1/manage/articles/7", { title: "T2", slug: "typed", digest: "D2", content: "updated", articleTypeId: 2, tagIds: [3] }, { headers: { "If-Match": "\"1\"" } })
    expect(created).toMatchObject({ id: 7, markdown: "M" })
    expect(updated).toMatchObject({ markdown: "updated", version: 2 })
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
})
