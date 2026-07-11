import { readFile } from "node:fs/promises"
import { flushPromises, mount } from "@vue/test-utils"
import { createMemoryHistory, createRouter } from "vue-router"
import { describe, expect, it, vi } from "vitest"

/** 可提升的生产适配器假实现与请求记录。 */
const { get } = vi.hoisted(() => {
  /** 测试适配器有意维护的文章页游标。 */
  let articlePage = 0
  /** 创建旧 API 文章响应。 */
  const apiArticle = (id: number) => ({ id, title: `Vue 文章 ${id}`, slug: `vue-${id}`, digest: `Vue 摘要 ${id}`, content: "正文", article_type_id: 2, tag_ids: [9], status: 2, support: 1, comment: 1, visited: id, version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null })
  return {
    get: vi.fn(async (url: string) => {
      if (url.includes("GetArticleList")) {
        const pageIndex = articlePage
        articlePage += 1
        const items = pageIndex === 0 ? Array.from({ length: 9 }, (_, index) => apiArticle(index + 1)) : [apiArticle(10)]
        return { data: { items, page: { number: pageIndex + 1, size: 9, total_items: 10, total_pages: 2 } } }
      }
      if (url.includes("ArticleType")) return { data: { items: [{ id: 2, name: "前端", image: null, meun: 1 }], page: { number: 1, size: 20, total_items: 1, total_pages: 1 } } }
      return { data: { items: [{ id: 9, name: "Vue" }], page: { number: 1, size: 20, total_items: 1, total_pages: 1 } } }
    }),
  }
})

vi.mock("@/request/http", () => ({ http: { get } }))
import ArticleListView from "@/views/public/ArticleListView.vue"

describe("T9 article list behavior", () => {
  it("syncs query filters and requests the next page only after load more", async () => {
    // Given: 带全部三种筛选的直接深链。
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/articles", component: ArticleListView }, { path: "/articles/:id", component: { template: "<p>detail</p>" } }] })
    await router.push("/articles?q=Vue&categoryId=2&tagId=9")
    const wrapper = mount(ArticleListView, { global: { plugins: [router], stubs: { BlogLayout: { template: "<main><slot /></main>" } } } })
    await flushPromises()

    // When: 首屏完成后尚未发生自动翻页。
    const articleCalls = get.mock.calls.filter(([url]) => url.includes("GetArticleList"))
    // Then: URL 筛选映射到 T4 feed，且只请求第零页。
    expect(articleCalls).toHaveLength(1)
    expect(articleCalls[0]?.[1]).toMatchObject({ params: { articleTypeId: 2, tagId: 9, pageModel: { pageIndex: 0, pageSize: 9 } } })
    expect(wrapper.text()).toContain("Vue 文章 1")
    expect(wrapper.findAll("article")).toHaveLength(9)

    // When: 用户显式点击加载更多。
    await wrapper.get('[data-testid="load-more"]').trigger("click")
    await flushPromises()
    // Then: 第二页只由该动作触发并追加第十篇。
    expect(get.mock.calls.filter(([url]) => url.includes("GetArticleList"))).toHaveLength(2)
    expect(wrapper.findAll("article")).toHaveLength(10)
  })

  it("contains no window infinite-scroll registration", async () => {
    // Given: 文章列表生产源码。
    const source = await readFile("src/views/public/ArticleListView.vue", "utf8")
    // When / Then: 分页只保留按钮路径，不监听 window scroll。
    expect(source).not.toMatch(/addEventListener\s*\(\s*["']scroll/)
    expect(source).toContain("加载更多")
  })
})
