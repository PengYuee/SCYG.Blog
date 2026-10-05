import { flushPromises, mount, type VueWrapper } from "@vue/test-utils"
import { createPinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { beforeAll, describe, expect, it, vi } from "vitest"
import ArticleEditorView from "@/views/author/ArticleEditorView.vue"
import TaxonomyView from "@/views/author/TaxonomyView.vue"
import RichMarkdownEditor from "@/components/editor/RichMarkdownEditor.vue"
import { createFakeAuthorRuntime, type AuthorRuntime } from "@/services/author-runtime"
import type { AuthorArticleType } from "@/services/author-contracts"
import { HttpRequestError } from "@/request/http-error"
import { ImageUploadError, type ImageLifecycle } from "@/services/image-lifecycle"

vi.mock("md-editor-v3", () => ({ MdEditor: { props: ["modelValue", "onUploadImg"], data: () => ({ uploadFile: new globalThis.File(["x"], "failed.png"), secondFile: new globalThis.File(["y"], "second.png") }), template: "<div><textarea data-testid='markdown-editor' :value='modelValue' @input='$emit(\"update:modelValue\", $event.target.value)' /><button data-testid='upload-image' @click='onUploadImg([uploadFile], (urls) => $emit(\"update:modelValue\", modelValue + urls[0]))'>上传</button><button data-testid='upload-two-images' @click='onUploadImg([uploadFile, secondFile], (urls) => $emit(\"update:modelValue\", modelValue + urls[0] + urls[1]))'>上传两张</button></div>" }, MdPreview: { props: ["modelValue"], template: "<div data-testid='safe-preview'>{{ modelValue }}</div>" }, MdCatalog: { template: "<nav />" } }))

/** Headless UI 弹窗在浏览器中依赖的尺寸观察器测试替身。 */
class TestResizeObserver { observe(): void {} unobserve(): void {} disconnect(): void {} }
beforeAll(() => { window.ResizeObserver = TestResizeObserver })

/** 创建作者组件测试路由。 */
async function authorRouter(path: string) {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/articles/new", component: ArticleEditorView }, { path: "/author/articles/:id/edit", component: ArticleEditorView }, { path: "/author/taxonomy", component: TaxonomyView }] })
  await router.push(path); await router.isReady(); return router
}

/** 挂载可注入运行时的文章编辑器。 */
async function mountEditor(runtime: AuthorRuntime = createFakeAuthorRuntime()): Promise<VueWrapper> {
  const router = await authorRouter("/author/articles/new")
  return mount(ArticleEditorView, { props: { runtime }, global: { plugins: [createPinia(), router] }, attachTo: document.body })
}

/** 打开文章分类下拉框。 */
async function openArticleTypes(wrapper: VueWrapper): Promise<void> {
  await wrapper.get("[role='combobox']").trigger("click"); await flushPromises()
}

/** 按显示名称激活分类下拉选项。 */
async function activateArticleType(wrapper: VueWrapper, name: string): Promise<void> {
  await openArticleTypes(wrapper)
  const option = wrapper.findAll("[role='option']").find((item) => item.text() === name)
  if (option === undefined) throw new TypeError(`缺少分类选项：${name}`)
  await option.trigger("click"); await flushPromises()
}

/** 获取弹窗内指定测试标识的按钮。 */
function modalButton(testId: string): HTMLButtonElement {
  const button = document.body.querySelector(`[data-testid='${testId}']`)
  if (!(button instanceof HTMLButtonElement)) throw new TypeError(`缺少弹窗按钮：${testId}`)
  return button
}
describe("T11 author views", () => {
  it("creates a controlled fake article and prevents duplicate saving", async () => {
    // Given: 显式 Fake 新建文章页面。
    const router = await authorRouter("/author/articles/new")
    const wrapper = mount(ArticleEditorView, { props: { runtime: createFakeAuthorRuntime() }, global: { plugins: [createPinia(), router] } })
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

  it("opens create modal and cancel clears only the selected category", async () => {
    // Given: 已选择现有分类的文章草稿。
    const wrapper = await mountEditor(); await flushPromises(); await activateArticleType(wrapper, "工程笔记")
    // When: 用户选择新增分类后取消弹窗。
    await activateArticleType(wrapper, "新增分类")
    expect(document.body.textContent).toContain("新增分类")
    modalButton("cancel-create-article-type").click(); await flushPromises()
    // Then: 当前选择被清空，但原分类选项仍保留。
    expect(wrapper.get("[role='combobox']").text()).toContain("请选择分类")
    await openArticleTypes(wrapper); expect(wrapper.text()).toContain("工程笔记"); wrapper.unmount()
  })

  it("refreshes article types and selects the created resource", async () => {
    // Given: 创建返回的新分类与随后刷新的分类列表。
    const base = createFakeAuthorRuntime(); const created: AuthorArticleType = { id: 8, name: "架构", imageUrl: null, menu: 2, version: 1 }
    const listArticleTypes = vi.fn(base.taxonomy.listArticleTypes).mockResolvedValueOnce([{ id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }]).mockResolvedValueOnce([{ id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }, created])
    const createArticleType = vi.fn(base.taxonomy.createArticleType).mockResolvedValue(created)
    const runtime = { ...base, taxonomy: { ...base.taxonomy, listArticleTypes, createArticleType } }; const wrapper = await mountEditor(runtime); await flushPromises()
    // When: 用户提交首尾带空格的新分类名称。
    await activateArticleType(wrapper, "新增分类"); const input = document.body.querySelector("[data-testid='article-type-name']")
    if (!(input instanceof HTMLInputElement)) throw new TypeError("缺少分类名称输入框")
    input.value = "  架构  "; input.dispatchEvent(new Event("input", { bubbles: true })); await flushPromises(); modalButton("submit-create-article-type").click(); await flushPromises()
    // Then: 请求使用裁剪名称，刷新完成并自动选择服务端返回标识。
    expect(createArticleType).toHaveBeenCalledWith({ name: "架构", image: null, menu: 2 }); expect(listArticleTypes).toHaveBeenCalledTimes(2)
    expect(wrapper.get("[role='combobox']").text()).toContain("架构"); wrapper.unmount()
  })

  it("merges and selects the created resource when successful refresh is stale", async () => {
    // Given: 创建返回新分类，但成功刷新的列表仍只有旧分类。
    const oldArticleType: AuthorArticleType = { id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }
    const created: AuthorArticleType = { id: 9, name: "最终一致分类", imageUrl: null, menu: 2, version: 1 }
    const base = createFakeAuthorRuntime(); const listArticleTypes = vi.fn(base.taxonomy.listArticleTypes).mockResolvedValueOnce([oldArticleType]).mockResolvedValueOnce([oldArticleType])
    const createArticleType = vi.fn(base.taxonomy.createArticleType).mockResolvedValue(created)
    const runtime = { ...base, taxonomy: { ...base.taxonomy, listArticleTypes, createArticleType } }; const wrapper = await mountEditor(runtime); await flushPromises()
    // When: 用户创建分类并收到陈旧但成功的刷新结果。
    await activateArticleType(wrapper, "新增分类"); const input = document.body.querySelector("[data-testid='article-type-name']")
    if (!(input instanceof HTMLInputElement)) throw new TypeError("缺少分类名称输入框")
    input.value = created.name; input.dispatchEvent(new Event("input", { bubbles: true })); await flushPromises(); modalButton("submit-create-article-type").click(); await flushPromises()
    // Then: 返回资源被无重复地补入选项，并按返回标识选中展示。
    expect(wrapper.get("[role='combobox']").text()).toContain(created.name); await openArticleTypes(wrapper)
    const createdOptions = wrapper.findAll("[role='option']").filter((option) => option.text() === created.name)
    expect(createdOptions).toHaveLength(1); expect(createdOptions[0]?.attributes("aria-selected")).toBe("true"); wrapper.unmount()
  })
  it("does not select from delete icon and clears a deleted selected category", async () => {
    // Given: 当前选择指向唯一分类。
    const base = createFakeAuthorRuntime(); const listArticleTypes = vi.fn(base.taxonomy.listArticleTypes).mockResolvedValueOnce([{ id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }]).mockResolvedValueOnce([])
    const runtime = { ...base, taxonomy: { ...base.taxonomy, listArticleTypes } }; const wrapper = await mountEditor(runtime); await flushPromises(); await activateArticleType(wrapper, "工程笔记"); await openArticleTypes(wrapper)
    // When: 点击删除图标后确认删除。
    await wrapper.get("button[aria-label='删除分类“工程笔记”']").trigger("click"); await flushPromises()
    expect(wrapper.get("[role='combobox']").text()).toContain("工程笔记"); expect(document.body.textContent).toContain("工程笔记")
    modalButton("confirm-delete-article-type").click(); await flushPromises()
    // Then: 图标点击本身未改选，成功删除才清空所选分类。
    expect(wrapper.get("[role='combobox']").text()).toContain("请选择分类"); wrapper.unmount()
  })

  it("removes a deleted category even when successful refresh is stale", async () => {
    // Given: 删除成功后的刷新仍返回已删除分类。
    const target: AuthorArticleType = { id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }; const base = createFakeAuthorRuntime()
    const listArticleTypes = vi.fn(base.taxonomy.listArticleTypes).mockResolvedValueOnce([target]).mockResolvedValueOnce([target]); const runtime = { ...base, taxonomy: { ...base.taxonomy, listArticleTypes } }; const wrapper = await mountEditor(runtime); await flushPromises(); await openArticleTypes(wrapper)
    // When: 用户确认删除该分类。
    await wrapper.get("button[aria-label='删除分类“工程笔记”']").trigger("click"); await flushPromises(); modalButton("confirm-delete-article-type").click(); await flushPromises(); await openArticleTypes(wrapper)
    // Then: 陈旧刷新不能把已确认删除的分类重新加入选项。
    expect(wrapper.findAll("[role='option']").some((option) => option.text() === target.name)).toBe(false); wrapper.unmount()
  })

  it("refreshes a conflicted delete target and requires fresh confirmation", async () => {
    // Given: 首次删除使用旧版本并收到冲突，刷新返回新版本。
    const stale: AuthorArticleType = { id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }; const refreshed = { ...stale, version: 2 }
    const base = createFakeAuthorRuntime(); const listArticleTypes = vi.fn(base.taxonomy.listArticleTypes).mockResolvedValueOnce([stale]).mockResolvedValueOnce([refreshed]).mockResolvedValueOnce([])
    const deleteArticleType = vi.fn(base.taxonomy.deleteArticleType).mockRejectedValueOnce(new HttpRequestError("版本冲突", 409, "CONFLICT", null)).mockResolvedValueOnce(undefined)
    const runtime = { ...base, taxonomy: { ...base.taxonomy, listArticleTypes, deleteArticleType } }; const wrapper = await mountEditor(runtime); await flushPromises(); await openArticleTypes(wrapper)
    // When: 用户确认旧版本删除，再按提示重新确认。
    await wrapper.get("button[aria-label='删除分类“工程笔记”']").trigger("click"); await flushPromises(); modalButton("confirm-delete-article-type").click(); await flushPromises()
    expect(document.body.textContent).toContain("请再次确认"); expect(deleteArticleType).toHaveBeenLastCalledWith({ id: 1, version: 1 })
    modalButton("confirm-delete-article-type").click(); await flushPromises()
    // Then: 第二次删除严格使用刷新后的版本。
    expect(deleteArticleType).toHaveBeenLastCalledWith({ id: 1, version: 2 }); wrapper.unmount()
  })

  it("locks the category dropdown until create reconciliation completes", async () => {
    // Given: 创建成功后的分类刷新仍处于挂起状态。
    const oldType: AuthorArticleType = { id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }; const created = { ...oldType, id: 2, name: "延迟分类", menu: 2 }
    let resolveRefresh: ((value: readonly AuthorArticleType[]) => void) | undefined; const deferredRefresh = new Promise<readonly AuthorArticleType[]>((resolve) => { resolveRefresh = resolve })
    const base = createFakeAuthorRuntime(); const listArticleTypes = vi.fn(base.taxonomy.listArticleTypes).mockResolvedValueOnce([oldType]).mockImplementationOnce(() => deferredRefresh)
    const createArticleType = vi.fn(base.taxonomy.createArticleType).mockResolvedValue(created); const runtime = { ...base, taxonomy: { ...base.taxonomy, listArticleTypes, createArticleType } }; const wrapper = await mountEditor(runtime); await flushPromises()
    // When: 创建写入完成但刷新尚未收敛。
    await activateArticleType(wrapper, "新增分类"); const input = document.body.querySelector("[data-testid='article-type-name']"); if (!(input instanceof HTMLInputElement)) throw new TypeError("缺少分类名称输入框")
    input.value = created.name; input.dispatchEvent(new Event("input", { bubbles: true })); await flushPromises(); modalButton("submit-create-article-type").click(); await flushPromises()
    // Then: 下拉在整个协调阶段禁用，刷新完成后才恢复并选中新分类。
    expect(wrapper.get("[role='combobox']").attributes("disabled")).toBeDefined(); if (resolveRefresh === undefined) throw new TypeError("缺少刷新完成器")
    resolveRefresh([oldType]); await flushPromises(); expect(wrapper.get("[role='combobox']").attributes("disabled")).toBeUndefined(); expect(wrapper.get("[role='combobox']").text()).toContain(created.name); wrapper.unmount()
  })
  it("preserves selection when deleting a different category", async () => {
    // Given: 选择工程笔记，同时列表中还有待删除的后端分类。
    const first: readonly AuthorArticleType[] = [{ id: 1, name: "工程笔记", imageUrl: null, menu: 1, version: 1 }, { id: 2, name: "后端", imageUrl: null, menu: 2, version: 3 }]
    const base = createFakeAuthorRuntime(); const listArticleTypes = vi.fn(base.taxonomy.listArticleTypes).mockResolvedValueOnce(first).mockResolvedValueOnce([first[0]])
    const runtime = { ...base, taxonomy: { ...base.taxonomy, listArticleTypes } }; const wrapper = await mountEditor(runtime); await flushPromises(); await activateArticleType(wrapper, "工程笔记"); await openArticleTypes(wrapper)
    // When: 用户删除未选中的后端分类。
    await wrapper.get("button[aria-label='删除分类“后端”']").trigger("click"); await flushPromises(); modalButton("confirm-delete-article-type").click(); await flushPromises()
    // Then: 当前工程笔记选择保持不变。
    expect(wrapper.get("[role='combobox']").text()).toContain("工程笔记"); wrapper.unmount()
  })

  it("keeps create modal input and selection retryable when mutation rejects", async () => {
    // Given: 已选分类且创建仓储第一次拒绝、第二次成功。
    const base = createFakeAuthorRuntime(); const created: AuthorArticleType = { id: 2, name: "可重试分类", imageUrl: null, menu: 2, version: 1 }
    const createArticleType = vi.fn(base.taxonomy.createArticleType).mockRejectedValueOnce(new HttpRequestError("请求失败", 503, "UPSTREAM", null)).mockResolvedValueOnce(created)
    const runtime = { ...base, taxonomy: { ...base.taxonomy, createArticleType } }; const wrapper = await mountEditor(runtime); await flushPromises(); await activateArticleType(wrapper, "工程笔记"); await activateArticleType(wrapper, "新增分类")
    const input = document.body.querySelector("[data-testid='article-type-name']"); if (!(input instanceof HTMLInputElement)) throw new TypeError("缺少分类名称输入框")
    input.value = "可重试分类"; input.dispatchEvent(new Event("input", { bubbles: true })); await flushPromises(); modalButton("submit-create-article-type").click(); await flushPromises()
    // When/Then: 失败保留名称、弹窗、选项与选择，并允许原地重试。
    expect(document.body.textContent).toContain("创建分类失败"); expect(input.value).toBe("可重试分类"); expect(wrapper.get("[role='combobox']").text()).toContain("工程笔记")
    modalButton("submit-create-article-type").click(); await flushPromises(); expect(createArticleType).toHaveBeenCalledTimes(2); wrapper.unmount()
  })

  it("keeps delete target and selection retryable when mutation rejects", async () => {
    // Given: 删除仓储第一次拒绝、第二次成功。
    const base = createFakeAuthorRuntime(); const deleteArticleType = vi.fn(base.taxonomy.deleteArticleType).mockRejectedValueOnce(new HttpRequestError("请求失败", 503, "UPSTREAM", null)).mockResolvedValueOnce(undefined)
    const runtime = { ...base, taxonomy: { ...base.taxonomy, deleteArticleType } }; const wrapper = await mountEditor(runtime); await flushPromises(); await activateArticleType(wrapper, "工程笔记"); await openArticleTypes(wrapper)
    await wrapper.get("button[aria-label='删除分类“工程笔记”']").trigger("click"); await flushPromises(); modalButton("confirm-delete-article-type").click(); await flushPromises()
    // When/Then: 失败保留目标、选择和确认框，第二次确认可直接重试。
    expect(document.body.textContent).toContain("删除分类失败"); expect(document.body.textContent).toContain("工程笔记"); expect(wrapper.get("[role='combobox']").text()).toContain("工程笔记")
    modalButton("confirm-delete-article-type").click(); await flushPromises(); expect(deleteArticleType).toHaveBeenCalledTimes(2); wrapper.unmount()
  })
  it("uses shared dialogs and toast for taxonomy creation", async () => {
    // Given: 显式 Fake 分类页面。
    const router = await authorRouter("/author/taxonomy")
    const wrapper = mount(TaxonomyView, { props: { runtime: createFakeAuthorRuntime() }, global: { plugins: [createPinia(), router] }, attachTo: document.body })
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

  it("cancels an uploaded image by id when article saving rejects", async () => {
    // Given: 图片上传成功、文章保存失败的真实生命周期形状。
    const router = await authorRouter("/author/articles/new")
    const base = createFakeAuthorRuntime()
    const create = vi.fn(base.articles.create).mockRejectedValueOnce(new TypeError("save rejected"))
    const deleteImage = vi.fn(base.articles.deleteImage)
    const runtime = { ...base, articles: { ...base.articles, create, deleteImage } }
    const wrapper = mount(ArticleEditorView, { props: { runtime }, global: { plugins: [createPinia(), router] } })
    await flushPromises()
    // When: 编辑器先插入远程 URL，再尝试保存文章。
    await wrapper.get("[data-testid='upload-image']").trigger("click"); await flushPromises()
    await wrapper.get("[data-testid='save-article']").trigger("click"); await flushPromises()
    // Then: 失败保存保留正文反馈，并严格取消上传返回的 id。
    expect(deleteImage).toHaveBeenCalledWith("fake-image-1")
    expect(wrapper.text()).toContain("保存失败")
  })

  it("commits uploaded images locally after save without issuing delete on unmount", async () => {
    // Given: 图片与文章均保存成功的编辑会话。
    const router = await authorRouter("/author/articles/new")
    const base = createFakeAuthorRuntime()
    const deleteImage = vi.fn(base.articles.deleteImage)
    const runtime = { ...base, articles: { ...base.articles, deleteImage } }
    const wrapper = mount(ArticleEditorView, { props: { runtime }, global: { plugins: [createPinia(), router] } })
    await flushPromises()
    // When: 上传图片、保存文章并离开页面。
    await wrapper.get("[data-testid='upload-image']").trigger("click"); await flushPromises()
    await wrapper.get("[data-testid='save-article']").trigger("click"); await flushPromises()
    wrapper.unmount(); await flushPromises()
    // Then: commit 只清除本地 pending，卸载不再删除已提交图片。
    expect(deleteImage).not.toHaveBeenCalled()
  })

  it("cancels a pending image by id when the editor unmounts", async () => {
    // Given: 已上传但尚未保存文章的编辑会话。
    const router = await authorRouter("/author/articles/new")
    const base = createFakeAuthorRuntime()
    const deleteImage = vi.fn(base.articles.deleteImage)
    const runtime = { ...base, articles: { ...base.articles, deleteImage } }
    const wrapper = mount(ArticleEditorView, { props: { runtime }, global: { plugins: [createPinia(), router] } })
    await flushPromises()
    await wrapper.get("[data-testid='upload-image']").trigger("click"); await flushPromises()
    // When: 用户未保存就离开编辑器。
    wrapper.unmount(); await flushPromises()
    // Then: 生命周期严格使用上传返回的 id 取消待提交图片。
    expect(deleteImage).toHaveBeenCalledWith("fake-image-1")
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
    const images: ImageLifecycle = { preview: () => "blob:failed", async upload() { throw new ImageUploadError("upload rejected") }, retainForCancel() {}, commit() {}, async cancel() { return true } }
    const wrapper = mount(RichMarkdownEditor, { props: { modelValue: "原始正文", images } })
    // When: md-editor 发起图片上传。
    await wrapper.get("[data-testid='upload-image']").trigger("click"); await flushPromises()
    // Then: 失败事件可见，但受控 Markdown 从未更新。
    expect(wrapper.emitted("uploadFailure")).toHaveLength(1)
    expect(wrapper.emitted("update:modelValue")).toBeUndefined()
  })

  it("retains successful URLs for cancellation when a sibling upload fails", async () => {
    // Given: 同批第一张成功、第二张失败的受控编辑器。
    const retainForCancel = vi.fn()
    let uploadCount = 0
    const images: ImageLifecycle = {
      preview: () => "blob:partial",
      async upload() { uploadCount += 1; if (uploadCount === 2) throw new ImageUploadError("second rejected"); return "https://api.test/first.png" },
      retainForCancel,
      commit() {},
      async cancel() { return true },
    }
    const wrapper = mount(RichMarkdownEditor, { props: { modelValue: "原始正文", images } })
    // When: md-editor 发起两张图片上传。
    await wrapper.get("[data-testid='upload-two-images']").trigger("click"); await flushPromises()
    // Then: 正文不更新，成功 URL 被标记为提交后仍可取消。
    expect(retainForCancel).toHaveBeenCalledWith(["https://api.test/first.png"])
    expect(wrapper.emitted("update:modelValue")).toBeUndefined()
  })
})
