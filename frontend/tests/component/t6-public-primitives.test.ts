import { flushPromises, mount } from "@vue/test-utils"
import { createMemoryHistory, createRouter, type Router } from "vue-router"
import { describe, expect, it, vi } from "vitest"
import ArticleSection from "@/components/article/ArticleSection.vue"
import BlogHeader from "@/components/public/BlogHeader.vue"
import HeroProfile from "@/components/public/HeroProfile.vue"
import HeroSearch from "@/components/public/HeroSearch.vue"
import BlogLayout from "@/layouts/BlogLayout.vue"
import type { ArticleSummary } from "@/types/article"

/** 创建仅供组件行为测试使用的内存路由。 */
const createTestRouter = async (path = "/"): Promise<Router> => {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { template: "<div />" } },
      { path: "/articles", component: { template: "<div />" } },
      { path: "/articles/:id", component: { template: "<div />" } },
    ],
  })
  await router.push(path)
  await router.isReady()
  return router
}

/** 创建一条完整只读文章摘要 fixture。 */
const article = (id: number): ArticleSummary => ({
  id,
  title: `文章 ${id}`,
  slug: `article-${id}`,
  digest: `第 ${id} 篇文章摘要`,
  markdown: "正文",
  articleTypeId: 1,
  tagIds: [1],
  status: 1,
  support: 2,
  comment: 3,
  visited: 4,
  version: 1,
  createdAt: "2026-07-11T00:00:00Z",
  updatedAt: null,
})

describe("T6 public desktop primitives", () => {
  it("routes a trimmed search and scrolls below the hero only from home", async () => {
    // Given: 首页 Hero 搜索和原生内容边界。
    const router = await createTestRouter()
    const scrollIntoView = vi.fn()
    const contentTarget = document.createElement("main")
    contentTarget.scrollIntoView = scrollIntoView
    const wrapper = mount(HeroSearch, { props: { contentTarget }, global: { plugins: [router] } })

    // When: 用户提交带首尾空白的关键词。
    await wrapper.get('input[type="search"]').setValue("  Vue 3  ")
    await wrapper.get("form").trigger("submit")
    await flushPromises()

    // Then: 路由查询稳定且仅首页内容边界被原生滚动。
    expect(router.currentRoute.value.fullPath).toBe("/articles?q=Vue+3")
    expect(scrollIntoView).toHaveBeenCalledOnce()
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "start" })
  })

  it("does not scroll when search starts outside the homepage", async () => {
    // Given: 文章列表页中的搜索和可观察的内容边界。
    const router = await createTestRouter("/articles")
    const scrollIntoView = vi.fn()
    const contentTarget = document.createElement("main")
    contentTarget.scrollIntoView = scrollIntoView
    const wrapper = mount(HeroSearch, { props: { contentTarget }, global: { plugins: [router] } })

    // When: 用户在非首页提交有效关键词。
    await wrapper.get('input[type="search"]').setValue("TypeScript")
    await wrapper.get("form").trigger("submit")
    await flushPromises()

    // Then: 查询被导航，但不会触发首页专属滚动。
    expect(router.currentRoute.value.fullPath).toBe("/articles?q=TypeScript")
    expect(scrollIntoView).not.toHaveBeenCalled()
  })
  it("does nothing when the search query is empty", async () => {
    // Given: 搜索框只有空白字符。
    const router = await createTestRouter()
    const wrapper = mount(HeroSearch, { global: { plugins: [router] } })
    await wrapper.get('input[type="search"]').setValue("   ")

    // When: 用户提交搜索。
    await wrapper.get("form").trigger("submit")
    await flushPromises()

    // Then: 地址保持首页。
    expect(router.currentRoute.value.fullPath).toBe("/")
  })

  it("keeps a local quote stable until explicit refresh and protects external links", async () => {
    // Given: 个人门户完成挂载。
    const wrapper = mount(HeroProfile)
    const originalQuote = wrapper.get('[data-testid="hero-quote"]').text()

    // When: Vue 重新渲染但用户未刷新名言。
    await wrapper.setProps({})

    // Then: 名言稳定，且全部真实站外链接安全打开。
    expect(wrapper.get('[data-testid="hero-quote"]').text()).toBe(originalQuote)
    const links = wrapper.findAll('a[target="_blank"]')
    expect(links).toHaveLength(4)
    expect(links.every((link) => link.attributes("rel") === "noopener noreferrer")).toBe(true)
    expect(links.map((link) => link.attributes("href"))).toEqual([
      "http://wpa.qq.com/msgrd?v=3&uin=798513422&site=qq&menu=yes",
      "https://gitee.com/wlmy1996/personal_website",
      "https://www.cnblogs.com/qwfy-y/",
      "https://github.com/PengYuee",
    ])

    // When: 用户显式点击换一句。
    await wrapper.get('[data-testid="quote-refresh"]').trigger("click")

    // Then: 有多个本地候选时不会立即重复。
    expect(wrapper.get('[data-testid="hero-quote"]').text()).not.toBe(originalQuote)
  })

  it("maps observer results to transparent and scrolled header states and cleans up", async () => {
    // Given: 可注入的原生 IntersectionObserver 工厂。
    const observe = vi.fn()
    const disconnect = vi.fn()
    let callback: IntersectionObserverCallback | undefined
    const observerFactory = vi.fn((nextCallback: IntersectionObserverCallback) => {
      callback = nextCallback
      return { observe, disconnect } as Pick<IntersectionObserver, "observe" | "disconnect">
    })
    const boundary = document.createElement("div")
    const router = await createTestRouter()
    const wrapper = mount(BlogHeader, { props: { boundary, observerFactory }, global: { plugins: [router] } })

    // When: Hero 边界进入和离开视口。
    callback?.([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver)
    await wrapper.vm.$nextTick()
    expect(wrapper.get("header").attributes("data-state")).toBe("transparent")
    callback?.([{ isIntersecting: false } as IntersectionObserverEntry], {} as IntersectionObserver)
    await wrapper.vm.$nextTick()

    // Then: 头部进入已滚动状态，卸载释放观察器。
    expect(wrapper.get("header").attributes("data-state")).toBe("scrolled")
    wrapper.unmount()
    expect(observe).toHaveBeenCalledWith(boundary)
    expect(disconnect).toHaveBeenCalledOnce()
  })

  it("exposes public landmarks, fallback and desktop article grammar", async () => {
    // Given: 公共布局与三篇文章。
    const router = await createTestRouter()
    const layoutObserverFactory = (): Pick<IntersectionObserver, "observe" | "disconnect"> => ({ observe: vi.fn(), disconnect: vi.fn() })
    const layout = mount(BlogLayout, { props: { observerFactory: layoutObserverFactory }, slots: { default: "<p>内容边界</p>" }, global: { plugins: [router] } })
    const section = mount(ArticleSection, {
      props: { title: "最新文章", articles: [article(1), article(2), article(3)], categories: [{ id: 1, name: "前端", imageUrl: null, menu: 1 }] },
      global: { plugins: [router] },
    })

    // When: Hero 项目图片加载失败。
    await layout.get('[data-testid="hero-image"]').trigger("error")

    // Then: 布局、无障碍回退和三列卡片原语均可组合。
    expect(layout.attributes("data-layout")).toBe("public")
    expect(layout.get('a[href="#blog-content"]').text()).toBe("跳到主要内容")
    expect(layout.get("main").attributes("id")).toBe("blog-content")
    expect(layout.get('[data-testid="hero-fallback"]').attributes("role")).toBe("img")
    expect(section.get('[data-testid="article-grid"]').classes()).toContain("blog-card-grid")
    expect(section.findAll("article")).toHaveLength(3)
  })
})