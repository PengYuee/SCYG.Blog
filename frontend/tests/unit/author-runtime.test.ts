import { afterEach, describe, expect, it, vi } from "vitest"
import { createApiServices } from "@/request/api-services"
import type { HttpTransport } from "@/request/transport"
import { AuthorRuntimeUnavailableError, createAuthorRuntime, createFakeAuthorRuntime } from "@/services/author-runtime"

/** 创建不会执行网络请求的完整 transport。 */
function transport(): HttpTransport {
  return { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
}

/** 每个运行时场景后恢复 Vite 环境与模块缓存。 */
afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})

describe("author runtime environment boundary", () => {
  it("refuses to impersonate an author outside explicit development mode", () => {
    // Given: 当前 Vitest 非 development 环境与完整真实 API 容器。
    const services = createApiServices(transport(), "https://api.test")
    // When / Then: 运行时在接触任何 API 前以中文类型化错误拒绝固定身份。
    expect(() => createAuthorRuntime(services)).toThrow(AuthorRuntimeUnavailableError)
    expect(() => createAuthorRuntime(services)).toThrow("当前环境未启用可信作者运行时")
  })

  it("keeps the fake taxonomy compatible with versioned article types", async () => {
    // Given: 显式 Fake 作者运行时。
    const runtime = createFakeAuthorRuntime()

    // When: 创建分类后使用返回版本删除同一资源。
    const created = await runtime.taxonomy.createArticleType({ name: "测试分类", image: null, menu: 2 })
    await runtime.taxonomy.deleteArticleType({ id: created.id, version: created.version })

    // Then: Fake 返回版本化资源，并按新窄目标完成删除。
    expect(created).toEqual({ id: 2, name: "测试分类", imageUrl: null, menu: 2, version: 1 })
    expect(await runtime.taxonomy.listArticleTypes()).toEqual([{ id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }])
  })

  it("keeps the fake taxonomy compatible with versioned tags", async () => {
    // Given: 显式 Fake 作者运行时。
    const runtime = createFakeAuthorRuntime()

    // When: 创建标签后使用返回版本删除同一资源。
    const created = await runtime.taxonomy.createTag("测试标签")
    await runtime.taxonomy.deleteTag({ id: created.id, version: created.version })

    // Then: Fake 返回完整版本化标签，并按版本目标完成删除。
    expect(created).toEqual({ id: 3, name: "测试标签", version: 1 })
    expect(await runtime.taxonomy.listTags()).toEqual([{ id: 1, name: "Vue", version: 1 }, { id: 2, name: "TypeScript", version: 1 }])
  })

  it("allocates a unique tag id after deleting a non-last tag", async () => {
    // Given: Fake 中仍保留 id=2 标签，但先删除了 id=1 的非末尾标签。
    const runtime = createFakeAuthorRuntime()
    await runtime.taxonomy.deleteTag({ id: 1, version: 1 })

    // When: 删除后再创建一个新标签。
    const created = await runtime.taxonomy.createTag("新标签")

    // Then: 新标识严格递增，且原有 id=2 标签仍完整保留。
    expect(created).toEqual({ id: 3, name: "新标签", version: 1 })
    expect(await runtime.taxonomy.listTags()).toEqual([
      { id: 2, name: "TypeScript", version: 1 },
      { id: 3, name: "新标签", version: 1 },
    ])
  })

  it("wires real article-type mutations in development", async () => {
    // Given: 显式开发作者环境与可观察的真实 API transport。
    vi.stubEnv("MODE", "development")
    vi.stubEnv("VITE_FAKE_AUTHOR", "true")
    vi.resetModules()
    const { createAuthorRuntime: createDevelopmentAuthorRuntime } = await import("@/services/author-runtime")
    const client = transport()
    vi.mocked(client.get).mockResolvedValue({ data: { items: [{ id: 9, name: "架构", image: null, meun: 2, version: 4, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page: { number: 1, size: 100, total_items: 1, total_pages: 1 } } })
    vi.mocked(client.post).mockResolvedValue({ data: { id: 9, name: "架构", image: null, meun: 2, version: 4, created_at: "2026-07-11T00:00:00Z", updated_at: null } })
    vi.mocked(client.delete).mockResolvedValue({ data: undefined })
    const runtime = createDevelopmentAuthorRuntime(createApiServices(client, "https://api.test"))

    // When: 分类走真实创建与删除。
    const listed = await runtime.taxonomy.listArticleTypes()
    const created = await runtime.taxonomy.createArticleType({ name: "架构", image: null, menu: 2 })
    await runtime.taxonomy.deleteArticleType({ id: created.id, version: created.version })

    // Then: 分类命中真实适配器。
    expect(listed).toEqual([{ id: 9, name: "架构", imageUrl: null, menu: 2, version: 4 }])
    expect(client.get).toHaveBeenCalledWith("/api/v1/article-types", { params: { page_size: 100, page: 1 } })
    expect(created).toEqual({ id: 9, name: "架构", imageUrl: null, menu: 2, version: 4 })
    expect(client.post).toHaveBeenCalledTimes(1)
    expect(client.delete).toHaveBeenCalledWith("/api/v1/article-types/9", { headers: { "If-Match": "\"4\"" } })
  })

  it("wires real tag list, create, and delete in development", async () => {
    // Given: 显式开发作者环境与可观察的真实标签 API transport。
    vi.stubEnv("MODE", "development")
    vi.stubEnv("VITE_FAKE_AUTHOR", "true")
    vi.resetModules()
    const { createAuthorRuntime: createDevelopmentAuthorRuntime } = await import("@/services/author-runtime")
    const client = transport()
    const tagResponse = { id: 7, name: "Vue", version: 3, created_at: "2026-07-11T00:00:00Z", updated_at: null }
    vi.mocked(client.get).mockResolvedValue({ data: { items: [tagResponse], page: { number: 1, size: 100, total_items: 1, total_pages: 1 } } })
    vi.mocked(client.post).mockResolvedValue({ data: tagResponse })
    vi.mocked(client.delete).mockResolvedValue({ data: undefined })
    const runtime = createDevelopmentAuthorRuntime(createApiServices(client, "https://api.test"))

    // When: 标签依次执行真实读取、创建和版本化删除。
    const listed = await runtime.taxonomy.listTags()
    const created = await runtime.taxonomy.createTag("Vue")
    await runtime.taxonomy.deleteTag({ id: created.id, version: created.version })

    // Then: 三项操作均直接命中真实标签适配器。
    expect(listed).toEqual([{ id: 7, name: "Vue", version: 3 }])
    expect(created).toEqual({ id: 7, name: "Vue", version: 3 })
    expect(client.get).toHaveBeenCalledWith("/api/v1/tags", { params: { page_size: 100, page: 1 } })
    expect(client.post).toHaveBeenCalledWith("/api/v1/tags", { name: "Vue" })
    expect(client.delete).toHaveBeenCalledWith("/api/v1/tags/7", { headers: { "If-Match": "\"3\"" } })
  })
})
