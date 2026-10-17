import { flushPromises, mount, type VueWrapper } from "@vue/test-utils"
import { createMemoryHistory, createRouter, RouterView, type Router } from "vue-router"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import HomeView from "@/views/public/HomeView.vue"
import { apiServicesKey, createApiServices, type ApiServices } from "@/request/api-services"
import { http } from "@/request/http"
import { createArticleFeed, type ArticleFeedApi } from "@/stores/article-feed"
import { createTaxonomy, type TaxonomyApi } from "@/stores/taxonomy"
import type { ArticleDetail } from "@/types/article"
import type { PageResult } from "@/types/api"
import type { ArticleType, Tag } from "@/types/taxonomy"
import { scrollRestorationKey } from "@/services/scroll-restoration"
import HeroSearch from "@/components/public/HeroSearch.vue"
import { ensureAnonymousSession, invalidateAnonymousSession } from "@/services/anonymous-ai-session"
import type { AgentRun } from "@/request/generated"

/** 首页首次终态通知观察器。 */
const markReady = vi.fn()
const scrollRestoration = { install: vi.fn(), arm: vi.fn(() => false), cancel: vi.fn(), markReady, wait: vi.fn(async () => false) }

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
const wrappers: VueWrapper[] = []

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
const mountHome = async (articleApi: ArticleFeedApi, taxonomyApi: TaxonomyApi): Promise<{ readonly wrapper: VueWrapper; readonly router: Router; readonly services: ApiServices }> => {
  const router = await createTestRouter()
  const services = createApiServices(http, "http://localhost:5000/api")
  const wrapper = mount(HomeView, {
    attachTo: document.body,
    props: { articleFeed: createArticleFeed(articleApi, 20), taxonomy: createTaxonomy(taxonomyApi), observerFactory },
    global: { plugins: [router], provide: { [scrollRestorationKey]: scrollRestoration, [apiServicesKey]: services } },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return { wrapper, router, services }
}

/** 构建一页已加载文章结果。 */
const page = (items: readonly ArticleDetail[]): PageResult<ArticleDetail> => ({ items, pageIndex: 0, pageSize: 20, totalItems: items.length, totalPages: items.length === 0 ? 0 : 1 })

beforeEach(() => {
  vi.stubGlobal("matchMedia", vi.fn(() => ({ matches: false })))
})
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  invalidateAnonymousSession()
  sessionStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
  markReady.mockClear()
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
    const wrapper = mount(RouterView, { global: { plugins: [router], provide: { [apiServicesKey]: apiServices, [scrollRestorationKey]: scrollRestoration } } })
    wrappers.push(wrapper)
    await flushPromises()

    // Then: 默认类型化状态机呈现两个作用域失败，而不是空白页面或缺失属性异常。
    expect(wrapper.get("[data-testid='taxonomy-error']").text()).toContain("离线验收")
    expect(wrapper.get("[data-testid='feed-error']").text()).toContain("离线验收")
    expect(wrapper.text()).toContain("妄揽明月")
    expect(markReady).toHaveBeenCalledOnce()
  })
  it("composes real profile, recommendations and one complete feed with ordered category filters", async () => {
    // Given: 两个有序分类与八篇真实领域文章。
    const items = [article(1, 1, 10), article(2, 2, 90), article(3, 1, 70), article(4, 1, 60), article(5, 1, 50), article(6, 1, 40), article(7, 1, 30), article(8, 1, 20)]

    // When: 首页并行加载字典和文章流。
    const { wrapper } = await mountHome({ list: async () => page(items) }, { listArticleTypes: async () => categories, listTags: async () => tags })

    // Then: Hero 首先可见，访问量排序推荐和完整文章流已挂载在隐藏内容区。
    expect(wrapper.get(".public-hero").isVisible()).toBe(true)
    expect(wrapper.get(".public-lower-page").isVisible()).toBe(false)
    expect(wrapper.text()).toContain("妄揽明月")
    expect(wrapper.get("[data-testid='home-announcement']").text()).toContain("已加载 8 篇")
    const section = wrapper.get("[data-testid='home-announcement']").element.closest("section")
    const heading = section?.querySelector("h2")
    expect(heading?.compareDocumentPosition(wrapper.get("[data-testid='home-announcement']").element) ?? 0).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
    expect(wrapper.get("[data-testid='recommended-articles']").text()).toMatch(/文章 2[\s\S]*文章 3[\s\S]*文章 4/)
    expect(wrapper.findAll("[data-testid='article-grid']")).toHaveLength(1)
    expect(wrapper.get("[data-testid='article-grid']").findAll("article")).toHaveLength(8)
    expect(wrapper.findAll('[aria-label="文章分类"] [data-filter-id]').map((button) => button.text())).toEqual(["后端", "前端"])
  })

  it("routes hero search through an isolated memory router", async () => {
    // Given: 已加载首页和独立内存路由。
    const list = vi.fn(async () => page([article(1, 1)]))
    const { wrapper, router } = await mountHome({ list }, { listArticleTypes: async () => categories, listTags: async () => tags })
    const scrollIntoView = vi.fn()
    wrapper.get("#home-results").element.scrollIntoView = scrollIntoView
    const scroll = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined)

    // When: 用户提交带空白的 Hero 搜索词。
    await wrapper.get("#hero-search-input").setValue("  Vue 状态机  ")
    await wrapper.get("#hero-search-input").trigger("submit")
    await flushPromises()

    // Then: 查询保留在首页并通过注入文章 API 加载。
    expect(router.currentRoute.value.path).toBe("/")
    expect(router.currentRoute.value.query["q"]).toBe("Vue 状态机")
    expect(scrollIntoView).not.toHaveBeenCalled()
    expect(scroll).toHaveBeenCalledWith(expect.objectContaining({ top: 0 }))
    expect(wrapper.get('[data-layout="public"]').attributes("data-scene")).toBe("content")
    expect(wrapper.get(".public-hero").isVisible()).toBe(false)
    expect(wrapper.get(".public-lower-page").isVisible()).toBe(true)
    expect(list).toHaveBeenLastCalledWith({ q: "Vue 状态机", pageModel: { pageIndex: 0, pageSize: 20 } })
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
      global: { plugins: [router], provide: { [scrollRestorationKey]: scrollRestoration, [apiServicesKey]: createApiServices(http, "http://localhost:5000/api") } },
    })
    wrappers.push(wrapper)
    await wrapper.vm.$nextTick()

    // Then: 字典和文章流分别提供不清空页面的加载反馈。
    expect(wrapper.get("[data-testid='taxonomy-loading']").attributes("role")).toBe("status")
    expect(wrapper.get("[data-testid='feed-loading']").attributes("role")).toBe("status")
    expect(wrapper.text()).toContain("妄揽明月")
    expect(markReady).not.toHaveBeenCalled()
  })
  it("shares both full search drafts across scenes without creating AI runs or resetting loaded pagination", async () => {
    const firstPage = Array.from({ length: 20 }, (_, index) => article(index + 1, 1))
    const list = vi.fn<ArticleFeedApi["list"]>(async ({ pageModel }) => ({
      items: pageModel.pageIndex === 0 ? firstPage : [article(21, 2)],
      pageIndex: pageModel.pageIndex, pageSize: 20, totalItems: 21, totalPages: 2,
    }))
    const { wrapper, router, services } = await mountHome({ list }, { listArticleTypes: async () => categories, listTags: async () => tags })
    const create = vi.spyOn(services.ai, "create")
    const anonymousSession = vi.spyOn(services.ai, "session")
    vi.spyOn(window, "scrollTo").mockImplementation(() => undefined)
    const searches = wrapper.findAllComponents(HeroSearch)
    expect(searches).toHaveLength(2)
    expect(searches.every((search) => search.props("compact") === false)).toBe(true)
    expect(wrapper.find("header .unified-search").exists()).toBe(false)
    expect(wrapper.find(".unified-search--compact").exists()).toBe(false)
    const hero = searches.find((search) => search.props("inputId") === "hero-search-input")!
    const content = searches.find((search) => search.props("inputId") === "content-search-input")!
    expect(wrapper.get(".home-content-search").element).toBe(content.element)
    await hero.get('input[type="search"]').setValue("未提交关键词")
    expect(content.get('input[type="search"]').element).toHaveProperty("value", "未提交关键词")
    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")
    await wrapper.get("[data-testid='load-more']").trigger("click")
    await flushPromises()
    expect(wrapper.get("[data-testid='article-grid']").findAll("article")).toHaveLength(21)
    const grid = wrapper.get("[data-testid='article-grid']").element
    await content.get('input[type="search"]').setValue("内容区关键词")
    expect(hero.get('input[type="search"]').element).toHaveProperty("value", "内容区关键词")
    await content.get('[aria-label="搜索方式"] button:nth-child(2)').trigger("click")
    await flushPromises()
    expect(hero.props("mode")).toBe("ai")
    expect(content.find(".compact-question").exists()).toBe(false)
    await content.get("textarea").setValue("独立问题\n第二行")
    expect(hero.get("textarea").element).toHaveProperty("value", "独立问题\n第二行")
    await content.get(".search-options-trigger").trigger("click")
    await content.get('input[type="number"]').setValue("3")
    await hero.get(".search-options-trigger").trigger("click")
    expect(hero.get('input[type="number"]').element).toHaveProperty("value", "3")
    const home = wrapper.findAll("header button").find((button) => button.text() === "主页")!
    await home.trigger("click")
    expect(wrapper.get(".public-hero").isVisible()).toBe(true)
    expect(wrapper.get(".public-lower-page").isVisible()).toBe(false)
    expect(wrapper.get("[data-testid='article-grid']").element).toBe(grid)
    await hero.get("textarea").setValue("Hero 修改的问题")
    expect(content.get("textarea").element).toHaveProperty("value", "Hero 修改的问题")
    await hero.get('[aria-label="搜索方式"] button:nth-child(1)').trigger("click")
    await flushPromises()
    expect(router.currentRoute.value.fullPath).toBe("/")
    expect(hero.get('input[type="search"]').element).toHaveProperty("value", "内容区关键词")
    expect(content.get('input[type="search"]').element).toHaveProperty("value", "内容区关键词")
    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")
    expect(wrapper.get(".public-lower-page").isVisible()).toBe(true)
    expect(wrapper.get("[data-testid='article-grid']").findAll("article")).toHaveLength(21)
    expect(list).toHaveBeenCalledTimes(2)
    expect(create).not.toHaveBeenCalled()
    expect(anonymousSession).not.toHaveBeenCalled()
  })

  it("keeps the active task mounted when returning to Hero and entering content again", async () => {
    const { wrapper, router } = await mountHome({ list: async () => page([article(1, 1)]) }, { listArticleTypes: async () => categories, listTags: async () => tags })
    vi.spyOn(window, "scrollTo").mockImplementation(() => undefined)
    await router.push("/?mode=ai&runId=scene-run")
    await vi.dynamicImportSettled()
    await flushPromises()
    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")
    const task = wrapper.getComponent({ name: "PublicSearchResult" })
    const feed = wrapper.get("[data-testid='article-grid']").element
    const home = wrapper.findAll("header button").find((button) => button.text() === "主页")!
    await home.trigger("click")
    expect(wrapper.get(".public-hero").isVisible()).toBe(true)
    expect(wrapper.get(".public-lower-page").isVisible()).toBe(false)
    expect(wrapper.getComponent({ name: "PublicSearchResult" }).element).toBe(task.element)
    expect(router.currentRoute.value.query["runId"]).toBe("scene-run")
    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")
    expect(wrapper.getComponent({ name: "PublicSearchResult" }).element).toBe(task.element)
    expect(wrapper.get("[data-testid='article-grid']").element).toBe(feed)
    expect(wrapper.get(".public-lower-page").isVisible()).toBe(true)
  })

  it("routes category filters into the same article feed and resets its page", async () => {
    const list = vi.fn<ArticleFeedApi["list"]>(async ({ articleTypeId }) => page([article(articleTypeId ?? 1, articleTypeId ?? 1)]))
    const { wrapper, router } = await mountHome({ list }, { listArticleTypes: async () => categories, listTags: async () => tags })
    await wrapper.get('[aria-label="文章分类"] [data-filter-id="2"]').trigger("click")
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ articleTypeId: "2" })
    expect(list).toHaveBeenLastCalledWith({ q: "", articleTypeId: 2, pageModel: { pageIndex: 0, pageSize: 20 } })
    expect(wrapper.findAll("[data-testid='article-grid']")).toHaveLength(1)
    expect(wrapper.get("[data-testid='article-grid']").findAll("article")).toHaveLength(1)
    expect(wrapper.get('[aria-label="文章分类"] [data-filter-id="2"]').attributes("aria-pressed")).toBe("true")
  })

  it("combines a sidebar tag with the applied keyword and category", async () => {
    // Given: 已应用关键词与分类，搜索框另有尚未提交的草稿。
    const list = vi.fn<ArticleFeedApi["list"]>(async () => page([article(1, 2)]))
    const { wrapper, router } = await mountHome({ list }, { listArticleTypes: async () => categories, listTags: async () => tags })
    await router.push("/?q=Vue&articleTypeId=2")
    await flushPromises()
    await wrapper.get("#content-search-input").setValue("未提交")

    // When: 点击右栏标签。
    await wrapper.get('[aria-label="文章标签"] [data-filter-id="1"]').trigger("click")
    await flushPromises()

    // Then: 只叠加标签，保留已应用关键词、分类及输入草稿。
    expect(router.currentRoute.value.query).toEqual({ q: "Vue", articleTypeId: "2", tagId: "1" })
    expect(list).toHaveBeenLastCalledWith({ q: "Vue", articleTypeId: 2, tagId: 1, pageModel: { pageIndex: 0, pageSize: 20 } })
    expect(wrapper.get("#content-search-input").element).toHaveProperty("value", "未提交")
  })

  it("keeps selected taxonomy filters when submitting a keyword", async () => {
    // Given: 分类与标签已经应用。
    const list = vi.fn<ArticleFeedApi["list"]>(async () => page([article(1, 2)]))
    const { wrapper, router } = await mountHome({ list }, { listArticleTypes: async () => categories, listTags: async () => tags })
    await router.push("/?articleTypeId=2&tagId=1")
    await flushPromises()

    // When: 在右侧搜索卡提交新关键词。
    await wrapper.get("#content-search-input").setValue("  Go  ")
    await wrapper.get("form.home-content-search").trigger("submit")
    await flushPromises()

    // Then: 同一次请求包含三项筛选，不丢失已有条件。
    expect(router.currentRoute.value.query).toEqual({ q: "Go", articleTypeId: "2", tagId: "1" })
    expect(list).toHaveBeenLastCalledWith({ q: "Go", articleTypeId: 2, tagId: 1, pageModel: { pageIndex: 0, pageSize: 20 } })
  })

  it("clears only the selected category and restores focus after navigation", async () => {
    // Given: 三项筛选同时生效，取消按钮可获得焦点。
    const list = vi.fn<ArticleFeedApi["list"]>(async () => page([article(1, 2)]))
    const { wrapper, router } = await mountHome({ list }, { listArticleTypes: async () => categories, listTags: async () => tags })
    await router.push("/?q=Go&articleTypeId=2&tagId=1")
    await flushPromises()
    const cancel = wrapper.get('#category-filter-options .filter-clear')
    cancel.element.focus()

    // When: 取消分类。
    await cancel.trigger("click")
    await flushPromises()

    // Then: 标签和关键词继续生效，焦点不随取消按钮一起丢失。
    expect(router.currentRoute.value.query).toEqual({ q: "Go", tagId: "1" })
    expect(list).toHaveBeenLastCalledWith({ q: "Go", tagId: 1, pageModel: { pageIndex: 0, pageSize: 20 } })
    expect(document.activeElement).toBe(wrapper.get('#category-filter-options [data-filter-id="2"]').element)
  })

  it("restores ordinary filter cards and selections after leaving AI mode", async () => {
    // Given: 普通模式的筛选已生效。
    const { wrapper, router, services } = await mountHome({ list: async () => page([article(1, 2)]) }, { listArticleTypes: async () => categories, listTags: async () => tags })
    await router.push("/?q=Go&articleTypeId=2&tagId=1")
    await flushPromises()
    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")
    const create = vi.spyOn(services.ai, "create")

    // When: 在右侧搜索卡切换到 AI，再切回普通模式。
    await wrapper.get('.home-content-search [aria-label="搜索方式"] button:nth-child(2)').trigger("click")
    await flushPromises()
    expect(wrapper.get(".home-taxonomy-cards").isVisible()).toBe(false)
    expect(wrapper.get("#content-search-input").element.tagName).toBe("TEXTAREA")
    await wrapper.get('.home-content-search [aria-label="搜索方式"] button:nth-child(1)').trigger("click")
    await flushPromises()

    // Then: 原筛选全部恢复，切换本身不创建 AI 任务。
    expect(wrapper.get(".home-taxonomy-cards").isVisible()).toBe(true)
    expect(router.currentRoute.value.query).toEqual({ q: "Go", articleTypeId: "2", tagId: "1" })
    expect(wrapper.get('#category-filter-options [data-filter-id="2"]').attributes("aria-pressed")).toBe("true")
    expect(wrapper.get('#tag-filter-options [data-filter-id="1"]').attributes("aria-pressed")).toBe("true")
    expect(create).not.toHaveBeenCalled()
  })

  it("keeps restart accessible when an anonymous identity expires on an existing run", async () => {
    // Given: 已显示的搜索有明确 Run，随后匿名身份失效。
    const identityServices = createApiServices(http, "http://localhost:5000/api")
    vi.spyOn(identityServices.ai, "session").mockResolvedValue({ accessToken: "recovery-identity", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 })
    await ensureAnonymousSession(identityServices.ai, true)
    const { wrapper, router, services } = await mountHome({ list: async () => page([]) }, { listArticleTypes: async () => categories, listTags: async () => tags })
    const run: AgentRun = { runId: "recovery-run", capability: "search", status: "succeeded", recipeId: "search-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: "/api/v1/runs/recovery-run/events", result: { conclusion: "", articles: [] }, failure: null, pendingInteraction: null }
    vi.spyOn(services.ai, "get").mockResolvedValue(run)
    const create = vi.spyOn(services.ai, "create")
    await router.push("/?mode=ai&runId=recovery-run")
    await flushPromises()
    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")
    expect(services.ai.get).toHaveBeenCalledWith("recovery-run", "anonymous", expect.any(AbortSignal))
    invalidateAnonymousSession()
    await flushPromises()

    // When: 用户明确开始新搜索。
    const restart = wrapper.findAll(".search-result-action").find(button => button.text() === "开始新的搜索")
    expect(restart?.isVisible()).toBe(true)
    await restart?.trigger("click")
    await flushPromises()

    // Then: 解除失效状态并清除旧 Run 地址，但不自动创建任务。
    expect(router.currentRoute.value.query["runId"]).toBeUndefined()
    expect(wrapper.get("#content-search-input").attributes("disabled")).toBeUndefined()
    expect(create).not.toHaveBeenCalled()
  })

  it("keeps expired unknown-creation recovery accessible while restoring an older run", async () => {
    // Given: 冻结时间，旧 Run 地址与已过期的未知创建同时被恢复。
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date("2026-10-17T00:00:00Z"))
    vi.stubGlobal("ResizeObserver", class { observe(): void {} unobserve(): void {} disconnect(): void {} })
    const identityServices = createApiServices(http, "http://localhost:5000/api")
    vi.spyOn(identityServices.ai, "session").mockResolvedValue({ accessToken: "recovery-identity", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 })
    await ensureAnonymousSession(identityServices.ai, true)
    sessionStorage.setItem("scyg.ai.anonymous.recovery-identity.workspace", JSON.stringify({
      form: { query: "尚未确认的新搜索", maxResults: 5 }, lastRun: "recovery-run",
      pending: { key: "12345678-1234-4234-8234-123456789012", input: { query: "尚未确认的新搜索", max_results: 5 }, capability: "search", createdAt: Date.now() - 86400001 },
    }))
    const { wrapper, router, services } = await mountHome({ list: async () => page([]) }, { listArticleTypes: async () => categories, listTags: async () => tags })
    const run: AgentRun = { runId: "recovery-run", capability: "search", status: "succeeded", recipeId: "search-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: "/api/v1/runs/recovery-run/events", result: { conclusion: "", articles: [] }, failure: null, pendingInteraction: null }
    vi.spyOn(services.ai, "get").mockResolvedValue(run)
    const create = vi.spyOn(services.ai, "create")
    await router.push("/?mode=ai&runId=recovery-run")
    await flushPromises()
    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")

    // When: 用户打开处理入口并确认仅放弃本地未知操作。
    await wrapper.get('[data-testid="abandon-public-search"]').trigger("click")
    await flushPromises()
    const confirmation = [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find(button => button.textContent?.trim() === "确认放弃并开始新操作")
    expect(confirmation).toBeDefined()
    confirmation?.click()
    await flushPromises()

    // Then: 原 Run 可再次显示，恢复动作不创建新任务。
    expect(router.currentRoute.value.query["runId"]).toBe("recovery-run")
    expect(wrapper.get("#content-search-input").attributes("disabled")).toBeUndefined()
    expect(create).not.toHaveBeenCalled()
  })
  it("clears the active task when history removes its URL identity", async () => {
    // Given: 一个任务地址由同一首页实例呈现。
    const { wrapper, router } = await mountHome({ list: async () => page([]) }, { listArticleTypes: async () => categories, listTags: async () => tags })
    await router.push("/?mode=ai")
    await router.push("/?mode=ai&runId=history-run")
    await vi.dynamicImportSettled()
    await flushPromises()
    expect(wrapper.findComponent({ name: "PublicSearchResult" }).exists()).toBe(true)
    // When: 浏览器后退到没有任务标识的 AI 搜索。
    const navigated = Promise.withResolvers<void>()
    const remove = router.afterEach(() => { remove(); navigated.resolve() })
    router.back()
    await navigated.promise
    await flushPromises()
    // Then: 旧任务组件移除，后续模式切换也不会把旧标识写回地址。
    expect(wrapper.findComponent({ name: "PublicSearchResult" }).exists()).toBe(false)
    await router.push("/")
    await wrapper.get('[aria-label="搜索方式"] button:nth-child(2)').trigger("click")
    await flushPromises()
    expect(router.currentRoute.value.query["runId"]).toBeUndefined()
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
    expect(wrapper.get("#hero-search-input").attributes("type")).toBe("search")
    expect(wrapper.text()).toContain("TypeScript")
    expect(wrapper.text()).toContain("妄揽明月")
  })
})
