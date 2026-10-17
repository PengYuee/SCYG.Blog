import { flushPromises, mount } from "@vue/test-utils"
import { createMemoryHistory, createRouter, type Router } from "vue-router"
import { describe, expect, it, vi } from "vitest"
import { defineComponent, ref } from "vue"
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
  it("emits normal form submission without a router and keeps independent controlled drafts", async () => {
    const wrapper = mount(defineComponent({
      components: { HeroSearch },
      setup() {
        return { mode: ref<"normal" | "ai">("normal"), keyword: ref(""), question: ref(""), maxResults: ref(5), submit: vi.fn() }
      },
      template: '<HeroSearch v-model:mode="mode" v-model:keyword="keyword" v-model:question="question" v-model:max-results="maxResults" @submit="submit" />',
    }))
    const search = wrapper.getComponent(HeroSearch)
    await wrapper.get('input[type="search"]').setValue("  Vue 3  ")
    await wrapper.get("form").trigger("submit")
    expect(search.emitted("submit")).toHaveLength(1)
    expect(search.props("keyword")).toBe("  Vue 3  ")
    await wrapper.get('[aria-label="搜索方式"] button:nth-child(2)').trigger("click")
    await wrapper.get("textarea").setValue("第一行\n第二行")
    await wrapper.get("textarea").trigger("keydown", { key: "Enter" })
    expect(search.emitted("submit")).toHaveLength(1)
    await wrapper.get(".search-options-trigger").trigger("click")
    await wrapper.get('input[type="number"]').setValue("3")
    expect(search.props("maxResults")).toBe(3)
    await wrapper.get('[aria-label="搜索方式"] button:nth-child(1)').trigger("click")
    expect(wrapper.get('input[type="search"]').element).toHaveProperty("value", "  Vue 3  ")
    await wrapper.get('[aria-label="搜索方式"] button:nth-child(2)').trigger("click")
    expect(wrapper.get("textarea").element).toHaveProperty("value", "第一行\n第二行")
    expect(search.emitted("submit")).toHaveLength(1)
    wrapper.unmount()
  })

  it.each(["ctrlKey", "metaKey"])("submits AI multiline input with %s plus Enter only", async (modifier) => {
    const wrapper = mount(HeroSearch, { props: { mode: "ai", question: "问题\n补充" } })
    await wrapper.get("textarea").trigger("keydown", { key: "Enter", [modifier]: true, isComposing: true })
    expect(wrapper.emitted("submit")).toBeUndefined()
    await wrapper.get("textarea").trigger("keydown", { key: "Enter" })
    expect(wrapper.emitted("submit")).toBeUndefined()
    await wrapper.get("textarea").trigger("keydown", { key: "Enter", [modifier]: true })
    expect(wrapper.emitted("submit")).toHaveLength(1)
    wrapper.unmount()
  })

  it("does not submit unavailable or busy AI searches", async () => {
    const wrapper = mount(HeroSearch, { props: { mode: "ai", canSubmit: false } })
    await wrapper.get("form").trigger("submit")
    expect(wrapper.emitted("submit")).toBeUndefined()
    await wrapper.setProps({ canSubmit: true, busy: true })
    await wrapper.get("textarea").trigger("keydown", { key: "Enter", ctrlKey: true })
    expect(wrapper.emitted("submit")).toBeUndefined()
    wrapper.unmount()
  })
  it("restores compact-question focus after keyboard submission", async () => {
    // Given: 紧凑 AI 问题编辑器获得焦点。
    const wrapper = mount(HeroSearch, { attachTo: document.body, props: { compact: true, mode: "ai", question: "question" } })
    await wrapper.get(".compact-question").trigger("click")
    await flushPromises()
    // When: 用户用快捷键提交。
    await wrapper.get("textarea").trigger("keydown", { key: "Enter", ctrlKey: true })
    await flushPromises()
    // Then: 输入区折叠且焦点仍留在对应搜索控件。
    expect(wrapper.emitted("submit")).toHaveLength(1)
    expect(document.activeElement).toBe(wrapper.get(".compact-question").element)
    wrapper.unmount()
  })
  it.each([false, true])("closes search options before the editor with Escape (compact=%s)", async (compact) => {
    // Given: 参考文章数输入框在搜索选项中获得焦点。
    const wrapper = mount(HeroSearch, { attachTo: document.body, props: { compact, mode: "ai", question: "question" } })
    if (compact) await wrapper.get(".compact-question").trigger("click")
    await wrapper.get(".search-options-trigger").trigger("click")
    wrapper.get('input[type="number"]').element.focus()
    // When: 用户在数字输入中按 Escape。
    await wrapper.get('input[type="number"]').trigger("keydown", { key: "Escape" })
    await flushPromises()
    // Then: 只关闭选项，焦点回到原触发器，编辑草稿仍在。
    expect(wrapper.find('input[type="number"]').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get(".search-options-trigger").element)
    expect(wrapper.get("textarea").element).toHaveProperty("value", "question")
    wrapper.unmount()
  })
  it("records article navigation before scrolling the destination homepage", async () => {
    // Given: 详情路由的导航守卫尚未放行。
    const router = await createTestRouter("/articles/1")
    const gate = Promise.withResolvers<void>()
    const entered = Promise.withResolvers<void>()
    router.beforeEach(() => { entered.resolve(); return gate.promise })
    const scroll = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined)
    const wrapper = mount(BlogHeader, { props: { boundary: null }, global: { plugins: [router] } })
    // When: 点击博客名称，但目标路由还在导航中。
    await wrapper.get(".blog-shell > a").trigger("click")
    await entered.promise
    expect(scroll).not.toHaveBeenCalled()
    gate.resolve()
    await flushPromises()
    // Then: 只有导航完成后才滚动，不覆盖被离开页面的历史位置。
    expect(router.currentRoute.value.path).toBe("/")
    expect(scroll).toHaveBeenCalledOnce()
    wrapper.unmount()
    scroll.mockRestore()
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
    callback?.([{ isIntersecting: true, boundingClientRect: { bottom: 900 } } as IntersectionObserverEntry], {} as IntersectionObserver)
    await wrapper.vm.$nextTick()
    expect(wrapper.get("header").attributes("data-state")).toBe("transparent")
    callback?.([{ isIntersecting: false, boundingClientRect: { bottom: 0 } } as IntersectionObserverEntry], {} as IntersectionObserver)
    await wrapper.vm.$nextTick()

    // Then: 头部进入已滚动状态，卸载释放观察器。
    expect(wrapper.get("header").attributes("data-state")).toBe("scrolled")
    wrapper.unmount()
    expect(observe).toHaveBeenCalledWith(boundary)
    expect(disconnect).toHaveBeenCalledOnce()
  })

  it("exposes public landmarks, scroll cue, fallback and desktop article grammar", async () => {
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

    // Then: 布局、滚动提示、无障碍回退和三列卡片原语均可组合。
    expect(layout.attributes("data-layout")).toBe("public")
    expect(layout.get('a.fixed[href="#blog-content"]').text()).toBe("跳到主要内容")
    const scrollCue = layout.get('[data-testid="hero-scroll-cue"]')
    expect(scrollCue.element.tagName).toBe("BUTTON")
    expect(scrollCue.attributes("type")).toBe("button")
    expect(scrollCue.attributes("href")).toBeUndefined()
    expect(scrollCue.attributes("aria-label")).toBe("进入内容区")
    expect(scrollCue.get("svg").attributes("aria-hidden")).toBe("true")
    expect(layout.find(".wave-divider").exists()).toBe(false)
    expect(layout.get("main").attributes("id")).toBe("blog-content")
    expect(layout.get('[data-testid="hero-fallback"]').attributes("role")).toBe("img")
    expect(section.get('[data-testid="article-grid"]').classes()).toContain("blog-card-grid")
    expect(section.findAll("article")).toHaveLength(3)
    layout.unmount()
    section.unmount()
  })

  it("switches mutually exclusive mounted scenes through the model, CTA and home action", async () => {
    const router = await createTestRouter()
    const scroll = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined)
    const wrapper = mount(defineComponent({
      components: { BlogLayout },
      setup: () => ({
        contentVisible: ref(false),
        observerFactory: () => ({ observe: vi.fn(), disconnect: vi.fn() }),
      }),
      template: '<BlogLayout v-model:content-visible="contentVisible" :observer-factory="observerFactory"><input data-testid="persistent-draft" /></BlogLayout>',
    }), { attachTo: document.body, global: { plugins: [router] } })
    const layout = wrapper.getComponent(BlogLayout)
    const hero = wrapper.get(".public-hero")
    const content = wrapper.get(".public-lower-page")
    const draft = wrapper.get('[data-testid="persistent-draft"]')
    expect(layout.attributes("data-scene")).toBe("hero")
    expect(hero.isVisible()).toBe(true)
    expect(content.isVisible()).toBe(false)

    await wrapper.get('[data-testid="hero-scroll-cue"]').trigger("click")
    await flushPromises()
    expect(layout.props("contentVisible")).toBe(true)
    expect(layout.attributes("data-scene")).toBe("content")
    expect(hero.isVisible()).toBe(false)
    expect(content.isVisible()).toBe(true)
    await draft.setValue("保留的草稿")
    const home = wrapper.findAll("header button").find((button) => button.text() === "主页")
    expect(home).toBeDefined()
    await home!.trigger("click")
    await flushPromises()
    expect(layout.props("contentVisible")).toBe(false)
    expect(layout.attributes("data-scene")).toBe("hero")
    expect(hero.isVisible()).toBe(true)
    expect(content.isVisible()).toBe(false)
    expect(wrapper.get('[data-testid="persistent-draft"]').element).toBe(draft.element)
    wrapper.vm.contentVisible = true
    await flushPromises()
    expect(hero.isVisible()).toBe(false)
    expect(content.isVisible()).toBe(true)
    expect(draft.element).toHaveProperty("value", "保留的草稿")
    wrapper.unmount()
    scroll.mockRestore()
  })

  it("enters content only for downward hero wheel outside interactive regions and never returns on scroll", async () => {
    const router = await createTestRouter()
    const scroll = vi.spyOn(window, "scrollTo").mockImplementation(() => undefined)
    const layout = mount(BlogLayout, {
      attachTo: document.body,
      props: { observerFactory: () => ({ observe: vi.fn(), disconnect: vi.fn() }) },
      slots: {
        "hero-search": '<div><form data-wheel="form"><span>表单区域</span></form><input data-wheel="input" /><textarea data-wheel="textarea" /><select data-wheel="select"><option>选项</option></select><button data-wheel="button">按钮</button><a data-wheel="link" href="/">链接</a><div role="dialog" data-wheel="dialog"><span>对话框</span></div><div contenteditable="true" data-wheel="editable">编辑区</div></div>',
        default: "<p>内容</p>",
      },
      global: { plugins: [router] },
    })
    expect(layout.props("contentVisible")).toBe(false)
    await layout.get(".public-hero").trigger("wheel", { deltaY: -100 })
    expect(layout.attributes("data-scene")).toBe("hero")
    for (const control of layout.findAll("[data-wheel]")) {
      await control.trigger("wheel", { deltaY: 100 })
      expect(layout.attributes("data-scene")).toBe("hero")
      expect(layout.get(".public-lower-page").isVisible()).toBe(false)
    }
    await layout.get(".public-hero").trigger("wheel", { deltaY: 100 })
    expect(layout.attributes("data-scene")).toBe("content")
    expect(layout.get(".public-hero").isVisible()).toBe(false)
    expect(layout.get(".public-lower-page").isVisible()).toBe(true)
    await layout.get(".public-lower-page").trigger("wheel", { deltaY: -100 })
    window.dispatchEvent(new Event("scroll"))
    await layout.vm.$nextTick()
    expect(layout.attributes("data-scene")).toBe("content")
    expect(layout.get(".public-lower-page").isVisible()).toBe(true)
    layout.unmount()
    scroll.mockRestore()
  })
})
