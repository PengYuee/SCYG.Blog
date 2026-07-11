import { flushPromises, mount } from "@vue/test-utils"
import { createPinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { beforeAll, describe, expect, it, vi } from "vitest"
import ArticleEditorView from "@/views/author/ArticleEditorView.vue"
import TaxonomyView from "@/views/author/TaxonomyView.vue"
import RichMarkdownEditor from "@/components/editor/RichMarkdownEditor.vue"
import { createFakeAuthorRuntime } from "@/services/author-runtime"
import { ImageUploadError, type ImageLifecycle } from "@/services/image-lifecycle"

vi.mock("md-editor-v3", () => ({ MdEditor: { props: ["modelValue", "onUploadImg"], template: "<div><textarea data-testid='markdown-editor' :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /><button data-testid='upload-image' @click='onUploadImg([new File([\"x\"], \"failed.png\")], (urls) => $emit(\"update:modelValue\", modelValue + urls[0]))'>上传</button></div>" }, MdPreview: { props: ["modelValue"], template: "<div data-testid='safe-preview'>{{ modelValue }}</div>" }, MdCatalog: { template: "<nav />" } }))

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

  it("unlocks saving and shows retry feedback when repository save rejects", async () => {
    // Given: 初始化成功但保存拒绝的注入运行时。
    const router = await authorRouter("/author/articles/new")
    const base = createFakeAuthorRuntime()
    const create = vi.fn(base.articles.create).mockRejectedValueOnce(new TypeError("save rejected"))
    const runtime = { ...base, articles: { ...base.articles, create } }
    const wrapper = mount(ArticleEditorView, { props: { runtime }, global: { plugins: [createPinia(), router] } })
    await flushPromises()
    // When: 用户保存草稿。
    await wrapper.get("[data-testid='save-article']").trigger("click"); await flushPromises()
    // Then: 保存锁释放，且共享 Toast 给出可重试反馈。
    expect(wrapper.get("[data-testid='save-article']").attributes("disabled")).toBeUndefined()
    expect(wrapper.text()).toContain("保存失败")
    expect(wrapper.text()).toContain("草稿仍在，请重试保存")
    await wrapper.get("[data-testid='save-article']").trigger("click"); await flushPromises()
    expect(create).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain("文章已保存")
  })

  it("converges editor initialization failure to a retryable state", async () => {
    // Given: 详情首次失败、重试成功的编辑运行时。
    const router = await authorRouter("/author/articles/42/edit")
    const base = createFakeAuthorRuntime()
    const detail = vi.fn(base.articles.detail).mockRejectedValueOnce(new TypeError("detail rejected"))
    const runtime = { ...base, articles: { ...base.articles, detail } }
    const wrapper = mount(ArticleEditorView, { props: { runtime }, global: { plugins: [createPinia(), router] } })
    await flushPromises()
    // When: 用户在可见错误态点击重试。
    expect(wrapper.text()).toContain("写作台加载失败")
    await wrapper.get("[data-testid='retry-editor-init']").trigger("click"); await flushPromises()
    // Then: 页面恢复为已载入编辑态。
    expect(wrapper.get("[data-testid='article-title']").element).toBeInstanceOf(HTMLInputElement)
  })

  it("converges taxonomy initialization failure to a retryable state", async () => {
    // Given: 标签首次失败、重试成功的分类运行时。
    const router = await authorRouter("/author/taxonomy")
    const base = createFakeAuthorRuntime()
    const listTags = vi.fn(base.taxonomy.listTags).mockRejectedValueOnce(new TypeError("tags rejected"))
    const runtime = { ...base, taxonomy: { ...base.taxonomy, listTags } }
    const wrapper = mount(TaxonomyView, { props: { runtime }, global: { plugins: [createPinia(), router] } })
    await flushPromises()
    // When: 用户点击重新载入。
    expect(wrapper.text()).toContain("分类加载失败")
    await wrapper.get("[data-testid='retry-taxonomy-init']").trigger("click"); await flushPromises()
    // Then: 分类操作重新可见。
    expect(wrapper.get("[data-testid='create-tag']").text()).toContain("新建标签")
  })

  it("does not emit a markdown update when image upload rejects", async () => {
    // Given: 保持原文且上传失败的受控编辑器。
    const images: ImageLifecycle = { preview: () => "blob:failed", async upload() { throw new ImageUploadError("upload rejected") }, commit() {}, async cancel() {} }
    const wrapper = mount(RichMarkdownEditor, { props: { modelValue: "原始正文", images } })
    // When: md-editor 发起图片上传。
    await wrapper.get("[data-testid='upload-image']").trigger("click"); await flushPromises()
    // Then: 失败事件可见，但受控 Markdown 从未更新。
    expect(wrapper.emitted("uploadFailure")).toHaveLength(1)
    expect(wrapper.emitted("update:modelValue")).toBeUndefined()
  })
})
