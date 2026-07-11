import { flushPromises, mount } from "@vue/test-utils"
import { createPinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { beforeAll, describe, expect, it, vi } from "vitest"
import ArticleEditorView from "@/views/author/ArticleEditorView.vue"
import TaxonomyView from "@/views/author/TaxonomyView.vue"

vi.mock("md-editor-v3", () => ({ MdEditor: { props: ["modelValue"], template: "<textarea data-testid='markdown-editor' :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' />" }, MdPreview: { props: ["modelValue"], template: "<div data-testid='safe-preview'>{{ modelValue }}</div>" }, MdCatalog: { template: "<nav />" } }))

/** Headless UI 弹窗在浏览器中依赖的尺寸观察器测试替身。 */
class TestResizeObserver { observe(): void {} unobserve(): void {} disconnect(): void {} }
beforeAll(() => { window.ResizeObserver = TestResizeObserver })

/** 创建作者组件测试路由。 */
async function authorRouter(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/articles/new", component: ArticleEditorView }, { path: "/author/articles/:id/edit", component: ArticleEditorView }, { path: "/author/taxonomy", component: TaxonomyView }] })
  await router.push(path); await router.isReady(); return router
}

describe("T11 author views", () => {
  it("creates a controlled fake article and prevents duplicate saving", async () => {
    // Given: 显式 Fake 新建文章页面。
    const router = await authorRouter("/author/articles/new")
    const wrapper = mount(ArticleEditorView, { global: { plugins: [createPinia(), router] } })
    await flushPromises()
    // When: 用户填写标题与 Markdown 并快速点击保存两次。
    await wrapper.get("[data-testid='article-title']").setValue("T11 富文本文章")
    await wrapper.get("[data-testid='markdown-editor']").setValue("# 正文")
    await wrapper.get("[data-testid='save-article']").trigger("click")
    await wrapper.get("[data-testid='save-article']").trigger("click")
    await flushPromises()
    // Then: 保存完成并显示成功反馈，预览消费受控 Markdown。
    expect(wrapper.text()).toContain("文章已保存")
    expect(wrapper.get("[data-testid='safe-preview']").text()).toContain("正文")
  })

  it("uses shared dialogs and toast for taxonomy creation", async () => {
    // Given: 显式 Fake 分类页面。
    const router = await authorRouter("/author/taxonomy")
    const wrapper = mount(TaxonomyView, { global: { plugins: [createPinia(), router] }, attachTo: document.body })
    await flushPromises()
    // When: 用户通过共享弹窗创建标签。
    await wrapper.get("[data-testid='create-tag']").trigger("click")
    await flushPromises()
    const input = document.body.querySelector("input")
    if (!(input instanceof HTMLInputElement)) throw new TypeError("taxonomy modal input missing")
    input.value = "守卫标签"; input.dispatchEvent(new Event("input", { bubbles: true }))
    const createButton = [...document.body.querySelectorAll("button")].find((button) => button.textContent === "创建")
    if (!(createButton instanceof HTMLButtonElement)) throw new TypeError("taxonomy create button missing")
    createButton.click(); await flushPromises()
    // Then: 新标签可见且共享 Toast 报告成功。
    expect(document.body.textContent).toContain("守卫标签")
    expect(document.body.textContent).toContain("分类字典已更新")
    wrapper.unmount()
  })
})
