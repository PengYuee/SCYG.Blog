import { flushPromises, mount, type VueWrapper } from "@vue/test-utils"
import { createPinia } from "pinia"
import { createMemoryHistory, createRouter } from "vue-router"
import { beforeAll, describe, expect, it, vi } from "vitest"
import ArticleSettings from "@/components/editor/ArticleSettings.vue"
import { HttpRequestError } from "@/request/http-error"
import { createFakeAuthorRuntime, type AuthorRuntime } from "@/services/author-runtime"
import type { AuthorTag } from "@/services/author-contracts"
import ArticleEditorView from "@/views/author/ArticleEditorView.vue"

/** Headless UI 弹窗在组件测试中使用的尺寸观察器替身。 */
class TestResizeObserver { observe(): void {} unobserve(): void {} disconnect(): void {} }
beforeAll(() => { window.ResizeObserver = TestResizeObserver })

/** 可由测试显式结束的异步操作。 */
type Deferred<Value> = {
  /** 挂起中的承诺。 */ readonly promise: Promise<Value>
  /** 结束挂起操作。 */ readonly resolve: (value: Value) => void
}

/** 创建一个由测试控制完成时机的承诺。 */
function deferred<Value>(): Deferred<Value> {
  let complete: ((value: Value) => void) | undefined
  const promise = new Promise<Value>((resolve) => { complete = resolve })
  if (complete === undefined) throw new TypeError("缺少异步完成器")
  return { promise, resolve: complete }
}

/** 挂载可注入运行时的新建文章编辑器。 */
async function mountEditor(runtime: AuthorRuntime): Promise<VueWrapper> {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/articles/new", component: ArticleEditorView }] })
  await router.push("/author/articles/new")
  await router.isReady()
  return mount(ArticleEditorView, { props: { runtime }, global: { plugins: [createPinia(), router] }, attachTo: document.body })
}

/** 获取新增标签弹窗内的名称输入框。 */
function tagNameInput(): HTMLInputElement {
  const input = document.body.querySelector("[data-testid='tag-name']")
  if (!(input instanceof HTMLInputElement)) throw new TypeError("缺少标签名称输入框")
  return input
}

/** 获取新增标签弹窗内的指定按钮。 */
function modalButton(testId: string): HTMLButtonElement {
  const button = document.body.querySelector(`[data-testid='${testId}']`)
  if (!(button instanceof HTMLButtonElement)) throw new TypeError(`缺少弹窗按钮：${testId}`)
  return button
}

/** 获取指定标签对应的受控复选框。 */
function tagCheckbox(wrapper: VueWrapper, name: string): HTMLInputElement {
  const deleteButton = wrapper.get(`button[aria-label='删除标签“${name}”']`).element
  const input = deleteButton.parentElement?.querySelector("input[type='checkbox']")
  if (!(input instanceof HTMLInputElement)) throw new TypeError(`缺少标签复选框：${name}`)
  return input
}

/** 打开新增标签弹窗并写入名称。 */
async function enterTagName(wrapper: VueWrapper, name: string): Promise<void> {
  await wrapper.get("button[aria-label='新增标签']").trigger("click")
  const input = tagNameInput()
  input.value = name
  input.dispatchEvent(new Event("input", { bubbles: true }))
  await flushPromises()
}

describe("ArticleEditorView 标签管理", () => {
  it("裁剪名称、刷新标签并默认选中新资源", async () => {
    // Given: 创建返回新标签，刷新也返回该资源。
    const base = createFakeAuthorRuntime()
    const oldTag: AuthorTag = { id: 1, name: "Vue", version: 1 }
    const created: AuthorTag = { id: 3, name: "架构", version: 1 }
    const listTags = vi.fn(base.taxonomy.listTags).mockResolvedValueOnce([oldTag]).mockResolvedValueOnce([oldTag, created])
    const createTag = vi.fn(base.taxonomy.createTag).mockResolvedValue(created)
    const wrapper = await mountEditor({ ...base, taxonomy: { ...base.taxonomy, listTags, createTag } })
    await flushPromises()

    // When: 用户提交首尾带空白的标签名称。
    await enterTagName(wrapper, "  架构  ")
    modalButton("submit-create-tag").click()
    await flushPromises()

    // Then: 请求名称已裁剪，列表刷新且新标签默认选中。
    expect(createTag).toHaveBeenCalledWith("架构")
    expect(listTags).toHaveBeenCalledTimes(2)
    expect(tagCheckbox(wrapper, created.name).checked).toBe(true)
    expect(tagNameInput().value).toBe("")
    wrapper.unmount()
  })

  it("成功刷新返回陈旧列表时仍合并并选中新标签", async () => {
    // Given: 创建成功后的刷新仍只返回旧标签。
    const base = createFakeAuthorRuntime()
    const oldTag: AuthorTag = { id: 1, name: "Vue", version: 1 }
    const created: AuthorTag = { id: 4, name: "最终一致", version: 2 }
    const listTags = vi.fn(base.taxonomy.listTags).mockResolvedValueOnce([oldTag]).mockResolvedValueOnce([oldTag])
    const wrapper = await mountEditor({ ...base, taxonomy: { ...base.taxonomy, listTags, createTag: vi.fn().mockResolvedValue(created) } })
    await flushPromises()

    // When: 用户创建标签并收到陈旧刷新。
    await enterTagName(wrapper, created.name)
    modalButton("submit-create-tag").click()
    await flushPromises()

    // Then: 创建返回资源仍只出现一次并保持选中。
    expect(wrapper.findAll(`button[aria-label='删除标签“${created.name}”']`)).toHaveLength(1)
    expect(tagCheckbox(wrapper, created.name).checked).toBe(true)
    wrapper.unmount()
  })

  it("刷新失败时仍保留新标签且取消新增不改变已有选择", async () => {
    // Given: Vue 已选中，首次取消后再次创建，但创建后的刷新失败。
    const base = createFakeAuthorRuntime()
    const oldTag: AuthorTag = { id: 1, name: "Vue", version: 1 }
    const created: AuthorTag = { id: 5, name: "可用标签", version: 1 }
    const listTags = vi.fn(base.taxonomy.listTags).mockResolvedValueOnce([oldTag]).mockRejectedValueOnce(new TypeError("刷新失败"))
    const wrapper = await mountEditor({ ...base, taxonomy: { ...base.taxonomy, listTags, createTag: vi.fn().mockResolvedValue(created) } })
    await flushPromises()
    tagCheckbox(wrapper, oldTag.name).click()
    await flushPromises()

    // When: 用户取消一次新增，再重新提交标签。
    await enterTagName(wrapper, "放弃名称")
    modalButton("cancel-create-tag").click()
    await flushPromises()
    expect(tagCheckbox(wrapper, oldTag.name).checked).toBe(true)
    await enterTagName(wrapper, created.name)
    modalButton("submit-create-tag").click()
    await flushPromises()

    // Then: 刷新失败不丢失新资源，已有选择与新选择同时保留。
    expect(tagCheckbox(wrapper, oldTag.name).checked).toBe(true)
    expect(tagCheckbox(wrapper, created.name).checked).toBe(true)
    expect(wrapper.text()).toContain("标签列表刷新失败")
    wrapper.unmount()
  })

  it("直接删除已选标签并抵抗陈旧刷新，删除点击不切换复选框", async () => {
    // Given: 已选标签的删除请求与陈旧刷新均由测试控制。
    const base = createFakeAuthorRuntime()
    const target: AuthorTag = { id: 1, name: "Vue", version: 3 }
    const deletion = deferred<void>()
    const listTags = vi.fn(base.taxonomy.listTags).mockResolvedValueOnce([target]).mockResolvedValueOnce([target])
    const deleteTag = vi.fn().mockImplementation(() => deletion.promise)
    const wrapper = await mountEditor({ ...base, taxonomy: { ...base.taxonomy, listTags, deleteTag } })
    await flushPromises()
    tagCheckbox(wrapper, target.name).click()
    await flushPromises()

    // When: 用户点击独立删除图标，服务端随后完成删除。
    await wrapper.get(`button[aria-label='删除标签“${target.name}”']`).trigger("click")
    await flushPromises()
    expect(tagCheckbox(wrapper, target.name).checked).toBe(true)
    expect(deleteTag).toHaveBeenCalledWith({ id: target.id, version: target.version })
    deletion.resolve()
    await flushPromises()

    // Then: 无确认弹窗，陈旧刷新也不能恢复标签与选择。
    expect(document.body.textContent).not.toContain("确认删除标签")
    expect(wrapper.find(`button[aria-label='删除标签“${target.name}”']`).exists()).toBe(false)
    wrapper.unmount()
  })

  it("409 删除失败时保留标签与选择", async () => {
    // Given: 已选标签仍被文章引用。
    const base = createFakeAuthorRuntime()
    const target: AuthorTag = { id: 1, name: "Vue", version: 1 }
    const deleteTag = vi.fn().mockRejectedValue(new HttpRequestError("仍被引用", 409, "CONFLICT", null))
    const wrapper = await mountEditor({ ...base, taxonomy: { ...base.taxonomy, listTags: vi.fn().mockResolvedValue([target]), deleteTag } })
    await flushPromises()
    tagCheckbox(wrapper, target.name).click()
    await flushPromises()

    // When: 用户直接删除该标签。
    await wrapper.get(`button[aria-label='删除标签“${target.name}”']`).trigger("click")
    await flushPromises()

    // Then: 标签和选择保持，反馈解释引用约束。
    expect(tagCheckbox(wrapper, target.name).checked).toBe(true)
    expect(wrapper.text()).toContain("删除标签失败")
    expect(wrapper.text()).toContain("该标签仍被文章引用，无法删除")
    wrapper.unmount()
  })

  it("412 刷新目标版本并在第二次点击时使用新版本", async () => {
    // Given: 首次删除版本过期，刷新返回版本 2。
    const base = createFakeAuthorRuntime()
    const stale: AuthorTag = { id: 1, name: "Vue", version: 1 }
    const refreshed: AuthorTag = { ...stale, version: 2 }
    const listTags = vi.fn(base.taxonomy.listTags).mockResolvedValueOnce([stale]).mockResolvedValueOnce([refreshed]).mockResolvedValueOnce([])
    const deleteTag = vi.fn().mockRejectedValueOnce(new HttpRequestError("版本冲突", 412, "PRECONDITION", null)).mockResolvedValueOnce(undefined)
    const wrapper = await mountEditor({ ...base, taxonomy: { ...base.taxonomy, listTags, deleteTag } })
    await flushPromises()

    // When: 用户先点击旧版本，再按提示点击刷新后的标签。
    await wrapper.get(`button[aria-label='删除标签“${stale.name}”']`).trigger("click")
    await flushPromises()
    expect(wrapper.text()).toContain("标签已发生变化，请再次点击删除")
    await wrapper.get(`button[aria-label='删除标签“${stale.name}”']`).trigger("click")
    await flushPromises()

    // Then: 两次调用严格使用旧版本与刷新后的新版本。
    expect(deleteTag).toHaveBeenNthCalledWith(1, { id: stale.id, version: stale.version })
    expect(deleteTag).toHaveBeenNthCalledWith(2, { id: refreshed.id, version: refreshed.version })
    wrapper.unmount()
  })

  it("标签写入期间阻止重复操作与排队选择事件", async () => {
    // Given: 标签创建请求保持挂起，草稿原本未选择标签。
    const base = createFakeAuthorRuntime()
    const created: AuthorTag = { id: 7, name: "延迟标签", version: 1 }
    const creation = deferred<AuthorTag>()
    const createTag = vi.fn().mockImplementation(() => creation.promise)
    const deleteTag = vi.fn(base.taxonomy.deleteTag)
    const wrapper = await mountEditor({ ...base, taxonomy: { ...base.taxonomy, createTag, deleteTag } })
    await flushPromises()
    await enterTagName(wrapper, created.name)

    // When: 创建挂起期间重复提交，并模拟已排队的标签选择事件。
    modalButton("submit-create-tag").click()
    await flushPromises()
    modalButton("submit-create-tag").click()
    wrapper.findComponent(ArticleSettings).vm.$emit("update:tagIds", [1])
    await wrapper.get("button[aria-label='删除标签“Vue”']").trigger("click")
    await flushPromises()

    // Then: 所有分类与标签入口锁定，重复写入和排队选择均未生效。
    expect(createTag).toHaveBeenCalledTimes(1)
    expect(deleteTag).not.toHaveBeenCalled()
    expect(wrapper.get("[role='combobox']").attributes("disabled")).toBeDefined()
    creation.resolve(created)
    await flushPromises()
    expect(tagCheckbox(wrapper, "Vue").checked).toBe(false)
    expect(tagCheckbox(wrapper, created.name).checked).toBe(true)
    wrapper.unmount()
  })
})
