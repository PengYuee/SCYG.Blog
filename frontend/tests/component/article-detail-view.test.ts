import { flushPromises, mount, type VueWrapper } from "@vue/test-utils"
import { defineComponent, h } from "vue"
import { createMemoryHistory, createRouter, type Router } from "vue-router"
import { describe, expect, it, vi } from "vitest"
import ArticleDetailView from "@/views/public/ArticleDetailView.vue"
import { apiServicesKey, createApiServices, type ApiServices } from "@/request/api-services"
import { http, HttpRequestError } from "@/request/http"
import { sanitizeMarkdown } from "@/security/sanitize-markdown"
import type { Taxonomy, TaxonomyState } from "@/stores/taxonomy"
import type { ArticleDetail } from "@/types/article"

/** 创建文章 101 的完整领域 fixture。 */
const articleFixture = (markdown = "## 安全目录\n\n```ts\nconst title = 'safe'\n```"): ArticleDetail => ({
  id: 101,
  title: "Vue 安全渲染实践",
  slug: "vue-secure-rendering",
  digest: "在共享边界内安全呈现 Markdown。",
  markdown,
  articleTypeId: 7,
  tagIds: [3, 5],
  status: 1,
  support: 12,
  comment: 4,
  visited: 208,
  version: 1,
  createdAt: "2026-07-11T08:00:00Z",
  updatedAt: "2026-07-11T09:30:00Z",
})

/** 创建测试可控的 taxonomy 状态机。 */
const taxonomyFixture = (nextState: TaxonomyState): Taxonomy => {
  let state: TaxonomyState = { kind: "idle" }
  return {
    get state() { return state },
    async load() { state = nextState },
    async retry() { state = nextState },
  }
}

/** 创建详情页内存路由。 */
const createDetailRouter = async (id = "101"): Promise<Router> => {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: { template: "<div />" } },
      { path: "/articles", component: { template: "<div />" } },
      { path: "/articles/:id", component: { template: "<div />" } },
    ],
  })
  await router.push(`/articles/${id}`)
  await router.isReady()
  return router
}

/** 预览桩保留 T7 的清理回调并呈现可观察产物。 */
const MdPreviewStub = defineComponent({
  name: "MdPreview",
  props: {
    id: { type: String, required: true },
    modelValue: { type: String, required: true },
    sanitize: { type: Function, required: true },
  },
  setup(props) {
    return () => h("article", {
      "data-preview-id": props.id,
      innerHTML: sanitizeMarkdown(`<h2 id="safe-catalog">安全目录</h2><pre><code>const title = 'safe'</code></pre><a href="javascript:alert(1)" onclick="alert(1)">bad</a>${props.modelValue}`),
    })
  },
})

/** 目录桩暴露共享预览标识。 */
const MdCatalogStub = defineComponent({
  name: "MdCatalog",
  props: { editorId: { type: String, required: true } },
  setup: (props) => () => h("nav", { "data-catalog-for": props.editorId }, "安全目录"),
})

/** 挂载带真实内存路由和 T7 子组件 seam 的详情页。 */
const mountDetail = async (
  loader: { readonly detail: (id: number) => Promise<ArticleDetail> },
  taxonomy?: Taxonomy,
  id = "101",
  apiServices?: ApiServices,
): Promise<VueWrapper> => {
  const router = await createDetailRouter(id)
  return mount(ArticleDetailView, {
    props: { articleLoader: loader, ...(taxonomy === undefined ? {} : { taxonomy }) },
    global: { plugins: [router], ...(apiServices === undefined ? {} : { provide: { [apiServicesKey]: apiServices } }), stubs: { MdPreview: MdPreviewStub, MdCatalog: MdCatalogStub } },
  })
}

describe("T10 article detail", () => {
  it("renders loading before the article request settles", async () => {
    // Given: 永不提前完成的文章请求和缺席 taxonomy。
    const loader = { detail: vi.fn(() => new Promise<ArticleDetail>(() => undefined)) }
    const wrapper = await mountDetail(loader, taxonomyFixture({ kind: "empty", articleTypes: [], tags: [] }))

    // When: 挂载生命周期开始请求。
    await wrapper.vm.$nextTick()

    // Then: 页面显式呈现 loading 状态。
    expect(wrapper.attributes("data-state")).toBe("loading")
    expect(wrapper.get('[role="status"]').text()).toContain("正在加载")
  })

  it("renders article 101 metadata, taxonomy, code, and desktop catalog", async () => {
    // Given: 完整文章与已加载的分类标签字典。
    const taxonomy = taxonomyFixture({
      kind: "ready",
      articleTypes: [{ id: 7, name: "前端", imageUrl: "https://api.example.test/images/frontend.jpg", menu: 1 }],
      tags: [{ id: 3, name: "Vue" }, { id: 5, name: "安全" }],
    })
    const loader = { detail: vi.fn(async () => articleFixture()) }

    // When: 详情与 taxonomy 完成加载。
    const wrapper = await mountDetail(loader, taxonomy)
    await flushPromises()

    // Then: 规范化 title、元数据、分类、标签、代码和目录全部可见。
    expect(wrapper.attributes("data-state")).toBe("ready")
    expect(wrapper.get("h1").text()).toBe("Vue 安全渲染实践")
    expect(wrapper.text()).toContain("2026年7月11日")
    expect(wrapper.text()).toContain("前端")
    expect(wrapper.text()).toContain("Vue")
    expect(wrapper.text()).toContain("安全")
    expect(wrapper.get('[data-testid="category-image"]').attributes("src")).toBe("https://api.example.test/images/frontend.jpg")
    await wrapper.get('[data-testid="category-image"]').trigger("error")
    expect(wrapper.get('[data-testid="category-image"]').attributes("src")).toBe("/images/hero-starry.jpg")
    expect(wrapper.get("code").text()).toContain("const title")
    const markdownLayout = wrapper.get('[data-testid="markdown-layout"]')
    expect(markdownLayout.classes()).toContain("lg:grid-cols-[minmax(0,var(--layout-reading-measure))_16rem]")
    expect(wrapper.get('[aria-label="文章目录"]').classes()).toContain("lg:sticky")
    expect(loader.detail).toHaveBeenCalledWith(101)
  })

  it("normalizes a relative category image against injected runtime config", async () => {
    // Given: 生产 taxonomy 接口返回相对图片，运行时配置指向独立后端。
    vi.spyOn(http, "get").mockImplementation(async (url: string) => {
      if (url.includes("ArticleType")) return { data: { items: [{ id: 7, name: "前端", image: "/media/frontend.jpg", meun: 1, version: 1, created_at: "2026-07-11T00:00:00Z", updated_at: null }], page: { number: 1, size: 20, total_items: 1, total_pages: 1 } } }
      return { data: { items: [], page: { number: 1, size: 20, total_items: 0, total_pages: 0 } } }
    })

    // When: 详情页使用生产 taxonomy 适配器加载分类。
    const apiServices = createApiServices(http, "http://localhost:5000/api")
    const wrapper = await mountDetail({ detail: async () => articleFixture() }, undefined, "101", apiServices)
    await flushPromises()

    // Then: 相对图片与 API 请求共享运行时后端地址。
    expect(wrapper.get('[data-testid="category-image"]').attributes("src")).toBe("http://localhost:5000/media/frontend.jpg")
  })
  it("keeps ready content safe while taxonomy is absent", async () => {
    // Given: taxonomy 永远停留在加载竞态，文章请求可独立完成。
    const taxonomy: Taxonomy = {
      state: { kind: "loading" },
      async load() { return new Promise<void>(() => undefined) },
      async retry() { return undefined },
    }

    // When: 文章先于 taxonomy 返回。
    const wrapper = await mountDetail({ detail: async () => articleFixture() }, taxonomy)
    await flushPromises()

    // Then: 页面不会崩溃，并使用稳定分类与图片回退。
    expect(wrapper.attributes("data-state")).toBe("ready")
    expect(wrapper.text()).toContain("未分类")
    expect(wrapper.get('[data-testid="category-image"]').attributes("src")).toBe("/images/hero-starry.jpg")
  })

  it("renders recoverable not-found and request error states", async () => {
    // Given: 一个后端 404 与一个可重试的失败请求。
    const emptyTaxonomy = taxonomyFixture({ kind: "empty", articleTypes: [], tags: [] })
    const notFound = await mountDetail({ detail: async () => Promise.reject(new HttpRequestError("missing", 404, "NOT_FOUND", null)) }, emptyTaxonomy)
    await flushPromises()
    const loader = { detail: vi.fn().mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce(articleFixture()) }
    const failed = await mountDetail(loader, emptyTaxonomy)
    await flushPromises()

    // When: 用户重试失败请求。
    expect(notFound.attributes("data-state")).toBe("not-found")
    expect(failed.attributes("data-state")).toBe("error")
    await failed.get("button").trigger("click")
    await flushPromises()

    // Then: 错误状态在当前详情范围内恢复。
    expect(failed.attributes("data-state")).toBe("ready")
    expect(loader.detail).toHaveBeenCalledTimes(2)
  })

  it("removes scripts, events, and unsafe URIs from malicious Markdown output", async () => {
    // Given: 包含脚本、事件属性与 javascript URI 的未受信任 Markdown。
    const markdown = '# 标题\n<script>alert(1)</script><img src=x onerror="alert(1)">[危险](javascript:alert(1))'

    // When: 内容仅经过共享 MarkdownRenderer 呈现。
    const wrapper = await mountDetail({ detail: async () => articleFixture(markdown) }, taxonomyFixture({ kind: "empty", articleTypes: [], tags: [] }))
    await flushPromises()
    const rendered = wrapper.html().toLowerCase()

    // Then: 最终 DOM 不含任何可执行脚本、事件或危险协议。
    expect(rendered).not.toContain("<script")
    expect(rendered).not.toContain("onerror")
    expect(rendered).not.toContain("onclick")
    expect(rendered).not.toContain('href="javascript:')
  })
})
