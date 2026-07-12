import { flushPromises, mount, type VueWrapper } from "@vue/test-utils"
import { createMemoryHistory, createRouter, RouterView, type Router } from "vue-router"
import { afterEach, describe, expect, it, vi } from "vitest"
import HomeView from "@/views/public/HomeView.vue"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import { http } from "@/request/http"
import { createArticleFeed, type ArticleFeedApi } from "@/stores/article-feed"
import { createTaxonomy, type TaxonomyApi } from "@/stores/taxonomy"
import type { ArticleDetail } from "@/types/article"
import type { PageResult } from "@/types/api"
import type { ArticleType, Tag } from "@/types/taxonomy"

/** 创建完整文章 fixture，保留真实领域统计字段。 */
const article = (id: number, articleTypeId: number, visited = id): ArticleDetail => ({
  id,
  title: `文章 ${id}`,
  slug: `article-${id}`,
  digest: `第 ${id} 篇文章摘要`,
  markdown: "正文",
  articleTypeId,
  tagIds: [1],
  status: 1,
  support: id,
  comment: id + 1,
  visited,
  version: 1,
  createdAt: `2026-07-${String(id).padStart(2, "0")}T00:00:00Z`,
  updatedAt: null,
})

/** 首页测试使用的有序分类。 */
const categories: readonly ArticleType[] = [
  { id: 2, name: "后端", imageUrl: null, menu: 1 },
  { id: 1, name: "前端", imageUrl: null, menu: 2 },
]
/** 首页测试使用的标签。 */
const tags: readonly Tag[] = [{ id: 1, name: "TypeScript" }]
/** 测试用无副作用边界观察器。 */
const observerFactory = (): Pick<IntersectionObserver, "observe" | "disconnect"> => ({ observe: vi.fn(), disconnect: vi.fn() })

/** 创建仅含 T8 所需目的地的内存路由。 */
const createTestRouter = async (): Promise<Router> => {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { template: "<div />" } },
      { path: "/articles", component: { template: "<div />" } },
      { path: "/articles/:id", component: { template: "<div />" } },
    ],
  })
  await router.push("/")
  await router.isReady()
  return router
}

/** 挂载注入真实 T4 状态机的首页。 */
const mountHome = async (articleApi: ArticleFeedApi, taxonomyApi: TaxonomyApi): Promise<{ readonly wrapper: VueWrapper; readonly router: Router }> => {
  const router = await createTestRouter()
  const wrapper = mount(HomeView, {
    props: { articleFeed: createArticleFeed(articleApi, 20), taxonomy: createTaxonomy(taxonomyApi), observerFactory },
    global: { plugins: [router] },
  })
  await flushPromises()
  return { wrapper, router }
}

/** 构建一页已加载文章结果。 */
const page = (items: readonly ArticleDetail[]): PageResult<ArticleDetail> => ({ items, pageIndex: 0, pageSize: 20, totalItems: items.length, totalPages: items.length === 0 ? 0 : 1 })

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})
describe("T8 desktop homepage discovery", () => {
  it("mounts from a direct route without injected props and keeps failures composed", async () => {
    // Given: 生产路由直接挂载 HomeView，且网络边界返回失败。
    vi.stubGlobal("IntersectionObserver", class {
      /** 测试观察器不产生交叉事件。 */ observe(): void {}
      /** 测试观察器释放时无外部资源。 */ disconnect(): void {}
    })
    vi.spyOn(http, "get").mockRejectedValue(new Error("离线验收"))
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/", component: HomeView },
        { path: "/articles", component: { template: "<div />" } },
      ],
    })
    await router.push("/")
    await router.isReady()

    // When: RouterView 按 T9 的直接组件方式渲染首页。
    const apiServices = createApiServices(http, "http://localhost:5000/api")
    const wrapper = mount(RouterView, { global: { plugins: [router], provide: { [apiServicesKey]: apiServices } } })
    await flushPromises()

    // Then: 默认类型化状态机呈现两个作用域失败，而不是空白页面或缺失属性异常。
    expect(wrapper.get("[data-testid='taxonomy-error']").text()).toContain("离线验收")
    expect(wrapper.get("[data-testid='feed-error']").text()).toContain("离线验收")
    expect(wrapper.text()).toContain("妄揽明月")
  })
  it("composes real profile, recommendations and stable category groups capped at six", async () => {
    // Given: 两个有序分类与八篇真实领域文章。
    const items = [article(1, 1, 10), article(2, 2, 90), article(3, 1, 70), article(4, 1, 60), article(5, 1, 50), article(6, 1, 40), article(7, 1, 30), article(8, 1, 20)]

    // When: 首页并行加载字典和文章流。
    const { wrapper } = await mountHome({ list: async () => page(items) }, { listArticleTypes: async () => categories, listTags: async () => tags })

    // Then: 真实个人资料、访问量排序推荐、稳定分类顺序和六篇上限均可见。
    expect(wrapper.text()).toContain("妄揽明月")
    expect(wrapper.get("[data-testid='home-announcement']").text()).toContain("已加载 8 篇")
    expect(wrapper.get("[data-testid='recommended-articles']").text()).toMatch(/文章 2[\s\S]*文章 3[\s\S]*文章 4/)
    const groups = wrapper.findAll("[data-testid='category-section']")
    expect(groups.map((group) => group.get("h2").text())).toEqual(["后端", "前端"])
    expect(groups[1]?.findAll("article")).toHaveLength(6)
    expect(wrapper.get("a[href='/articles?articleTypeId=2']").exists()).toBe(true)
  })

  it("routes hero search through an isolated memory router", async () => {
    // Given: 已加载首页和独立内存路由。
    const { wrapper, router } = await mountHome({ list: async () => page([article(1, 1)]) }, { listArticleTypes: async () => categories, listTags: async () => tags })
    const scrollIntoView = vi.fn()
    wrapper.get("main").element.scrollIntoView = scrollIntoView

    // When: 用户提交带空白的 Hero 搜索词。
    await wrapper.get("#hero-search-input").setValue("  Vue 状态机  ")
    await wrapper.get("form[role='search']").trigger("submit")
    await flushPromises()

    // Then: 查询确定性进入文章列表且滚动至 Hero 下方。
    expect(router.currentRoute.value.path).toBe("/articles")
    expect(router.currentRoute.value.query["q"]).toBe("Vue 状态机")
    expect(scrollIntoView).toHaveBeenCalledOnce()
  })

  it("shows scoped loading feedback while both requests are pending", async () => {
    // Given: 两个请求均保持在途。
    const pendingArticles = new Promise<PageResult<ArticleDetail>>(() => undefined)
    const pendingCategories = new Promise<readonly ArticleType[]>(() => undefined)
    const router = await createTestRouter()

    // When: 首页挂载并启动加载。
    const wrapper = mount(HomeView, {
      props: {
        articleFeed: createArticleFeed({ list: async () => pendingArticles }, 20),
        taxonomy: createTaxonomy({ listArticleTypes: async () => pendingCategories, listTags: async () => tags }),
        observerFactory,
      },
      global: { plugins: [router] },
    })
    await wrapper.vm.$nextTick()

    // Then: 字典和文章流分别提供不清空页面的加载反馈。
    expect(wrapper.get("[data-testid='taxonomy-loading']").attributes("role")).toBe("status")
    expect(wrapper.get("[data-testid='feed-loading']").attributes("role")).toBe("status")
    expect(wrapper.text()).toContain("妄揽明月")
  })
  it("recovers dictionary and feed failures independently", async () => {
    // Given: 字典和文章流首次请求分别失败。
    let feedCalls = 0
    let taxonomyCalls = 0
    const { wrapper } = await mountHome(
      { list: async () => { feedCalls += 1; if (feedCalls === 1) throw new Error("文章流暂不可用"); return page([article(1, 1)]) } },
      {
        listArticleTypes: async () => { taxonomyCalls += 1; if (taxonomyCalls === 1) throw new Error("字典暂不可用"); return categories },
        listTags: async () => tags,
      },
    )

    // When: 用户只重试字典，再单独重试文章流。
    expect(wrapper.get("[data-testid='taxonomy-error']").text()).toContain("字典暂不可用")
    expect(wrapper.get("[data-testid='feed-error']").text()).toContain("文章流暂不可用")
    await wrapper.get("[data-testid='taxonomy-retry']").trigger("click")
    await flushPromises()

    // Then: 字典恢复不隐式重试文章流，随后文章流可独立恢复且页面从未空白。
    expect(wrapper.find("[data-testid='taxonomy-error']").exists()).toBe(false)
    expect(wrapper.get("[data-testid='feed-error']").exists()).toBe(true)
    expect(feedCalls).toBe(1)
    await wrapper.get("[data-testid='feed-retry']").trigger("click")
    await flushPromises()
    expect(wrapper.find("[data-testid='feed-error']").exists()).toBe(false)
    expect(wrapper.text()).toContain("文章 1")
  })

  it("keeps the composed sidebar when the article feed is empty", async () => {
    // Given: 字典可用但文章流真实为空。
    // When: 首页完成加载。
    const { wrapper } = await mountHome({ list: async () => page([]) }, { listArticleTypes: async () => categories, listTags: async () => tags })

    // Then: 组合式空状态、个人资料、搜索和标签仍同时存在。
    expect(wrapper.get("[data-testid='feed-empty']").text()).toContain("暂无已发布文章")
    expect(wrapper.text()).toContain("文章搜索")
    expect(wrapper.text()).toContain("TypeScript")
    expect(wrapper.text()).toContain("妄揽明月")
  })
})
