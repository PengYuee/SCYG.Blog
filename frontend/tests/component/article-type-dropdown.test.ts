import { flushPromises, mount, type VueWrapper } from "@vue/test-utils"
import { describe, expect, it } from "vitest"
import ArticleSettings from "@/components/editor/ArticleSettings.vue"
import ArticleTypeDropdown from "@/components/editor/ArticleTypeDropdown.vue"
import type { AuthorArticleType } from "@/services/author-contracts"

/** 下拉框测试使用的版本化分类资源。 */
const ARTICLE_TYPES: readonly AuthorArticleType[] = [
  { id: 1, name: "前端", imageUrl: null, menu: 1, version: 3 },
  { id: 2, name: "后端", imageUrl: null, menu: 2, version: 5 },
]

/** 挂载受控文章分类下拉框。 */
const mountDropdown = (articleTypeId = 0, articleTypes: readonly AuthorArticleType[] = ARTICLE_TYPES, disabled = false): VueWrapper => mount(ArticleTypeDropdown, {
  props: { articleTypeId, articleTypes, disabled },
  attachTo: document.body,
})

/** 聚焦并打开下拉框，等待虚拟活动项更新完成。 */
const openDropdown = async (wrapper: VueWrapper): Promise<void> => {
  const combobox = wrapper.get("[role='combobox']")
  combobox.element.focus()
  await combobox.trigger("click")
  await flushPromises()
}

describe("ArticleTypeDropdown", () => {
  it("pins create first and keeps every option outside the Tab order", async () => {
    // Given: 两个版本化分类。
    const wrapper = mountDropdown()

    // When: 用户打开分类下拉框。
    await openDropdown(wrapper)

    // Then: 新增分类固定在首位，真实分类拥有删除按钮，所有选项只接受虚拟焦点。
    expect(wrapper.findAll("[role='option']").map((option) => option.text())).toEqual(["新增分类", "前端", "后端"])
    expect(wrapper.findAll("[role='option']").map((option) => option.attributes("tabindex"))).toEqual(["-1", "-1", "-1"])
    expect(wrapper.findAll("button[aria-label^='删除分类']").map((button) => button.attributes("aria-label"))).toEqual(["删除分类“前端”", "删除分类“后端”"])
    expect(wrapper.get("[role='combobox']").attributes("aria-controls")).toBe(wrapper.get("[role='listbox']").attributes("id"))

    wrapper.unmount()
  })

  it("emits mouse selection without mutating the controlled label", async () => {
    // Given: 尚未选择分类的受控下拉框。
    const wrapper = mountDropdown()
    await openDropdown(wrapper)

    // When: 用户点击前端分类。
    await wrapper.findAll("[role='option']")[1]?.trigger("click")
    await flushPromises()

    // Then: 组件仅发出更新，父级回写前仍显示原受控值。
    expect(wrapper.emitted("update:articleTypeId")).toEqual([[1]])
    expect(wrapper.get("[role='combobox']").text()).toContain("请选择分类")
    await wrapper.setProps({ articleTypeId: 1 })
    expect(wrapper.get("[role='combobox']").text()).toContain("前端")

    wrapper.unmount()
  })

  it("isolates native delete clicks from category selection", async () => {
    // Given: 展开的分类下拉框与可原生聚焦的删除按钮。
    const wrapper = mountDropdown()
    await openDropdown(wrapper)
    const deleteButton = wrapper.get("button[aria-label='删除分类“前端”']")

    // When: 原生按钮通过 click 激活，兼容浏览器的 Enter 与 Space 激活语义。
    await deleteButton.trigger("click")

    // Then: 删除事件携带完整版本资源，不触发选择或关闭，按钮仍在原生 Tab 序列。
    expect(deleteButton.attributes("tabindex")).toBeUndefined()
    expect(deleteButton.attributes("disabled")).toBeUndefined()
    expect(wrapper.emitted("request-delete-article-type")).toEqual([[ARTICLE_TYPES[0]]])
    expect(wrapper.emitted("update:articleTypeId")).toBeUndefined()
    expect(wrapper.get("[role='combobox']").attributes("aria-expanded")).toBe("true")

    wrapper.unmount()
  })

  it("keeps trigger focus while virtual navigation updates active descendant", async () => {
    // Given: 焦点位于未选择分类的组合框。
    const wrapper = mountDropdown()
    const combobox = wrapper.get("[role='combobox']")
    combobox.element.focus()

    // When: 用户展开并下移到首个真实分类。
    await combobox.trigger("keydown", { key: "ArrowDown" })
    await flushPromises()
    const createOptionId = wrapper.findAll("[role='option']")[0]?.attributes("id")
    expect(combobox.attributes("aria-activedescendant")).toBe(createOptionId)
    expect(document.activeElement).toBe(combobox.element)
    await combobox.trigger("keydown", { key: "ArrowDown" })
    const firstCategoryId = wrapper.findAll("[role='option']")[1]?.attributes("id")

    // Then: aria-activedescendant 指向前端分类，回车选择后焦点仍在触发器。
    expect(combobox.attributes("aria-activedescendant")).toBe(firstCategoryId)
    expect(document.activeElement).toBe(combobox.element)
    await combobox.trigger("keydown", { key: "Enter" })
    await flushPromises()
    expect(wrapper.emitted("update:articleTypeId")).toEqual([[1]])
    expect(combobox.attributes("aria-expanded")).toBe("false")
    expect(document.activeElement).toBe(combobox.element)

    wrapper.unmount()
  })

  it("supports Home End Space and practical typeahead with virtual focus", async () => {
    // Given: 当前受控选择为前端，真实焦点位于组合框。
    const wrapper = mountDropdown(1)
    const combobox = wrapper.get("[role='combobox']")
    combobox.element.focus()
    await combobox.trigger("keydown", { key: " " })
    await flushPromises()
    expect(combobox.attributes("aria-activedescendant")).toBe(wrapper.findAll("[role='option']")[1]?.attributes("id"))

    // When: 用户依次跳到首项、末项并用中文前缀定位后端。
    await combobox.trigger("keydown", { key: "Home" })
    expect(combobox.attributes("aria-activedescendant")).toBe(wrapper.findAll("[role='option']")[0]?.attributes("id"))
    await combobox.trigger("keydown", { key: "End" })
    expect(combobox.attributes("aria-activedescendant")).toBe(wrapper.findAll("[role='option']")[2]?.attributes("id"))
    await combobox.trigger("keydown", { key: "后" })
    await combobox.trigger("keydown", { key: " " })
    await flushPromises()

    // Then: 空格激活后端分类且真实焦点从未离开组合框。
    expect(wrapper.emitted("update:articleTypeId")).toEqual([[2]])
    expect(document.activeElement).toBe(combobox.element)

    wrapper.unmount()
  })

  it("keeps popup open for Tab access and closes from delete-button Escape", async () => {
    // Given: 展开的组合框仍持有真实焦点。
    const wrapper = mountDropdown()
    await openDropdown(wrapper)
    const combobox = wrapper.get("[role='combobox']")

    // When: 用户按 Tab 并让浏览器把焦点移动到首个删除按钮。
    await combobox.trigger("keydown", { key: "Tab" })
    const deleteButton = wrapper.get("button[aria-label='删除分类“前端”']")
    deleteButton.element.focus()
    await flushPromises()

    // Then: 弹层保持打开；删除按钮按 Escape 后关闭并恢复触发器焦点。
    expect(document.activeElement).toBe(deleteButton.element)
    expect(combobox.attributes("aria-expanded")).toBe("true")
    await deleteButton.trigger("keydown", { key: "Escape" })
    await flushPromises()
    expect(combobox.attributes("aria-expanded")).toBe("false")
    expect(document.activeElement).toBe(combobox.element)

    wrapper.unmount()
  })

  it("closes safely and blocks every entry point when disabled", async () => {
    // Given: 已打开的受控下拉框。
    const wrapper = mountDropdown()
    await openDropdown(wrapper)

    // When: 父级在变更期间启用禁用锁。
    await wrapper.setProps({ disabled: true })
    await flushPromises()

    // Then: 弹层安全关闭，触发器禁用且无法再次打开。
    const combobox = wrapper.get("[role='combobox']")
    expect(combobox.attributes("disabled")).toBeDefined()
    expect(combobox.attributes("aria-expanded")).toBe("false")
    await combobox.trigger("click")
    expect(wrapper.find("[role='listbox']").exists()).toBe(false)

    wrapper.unmount()
  })

  it("keeps create available when categories are empty", async () => {
    // Given: 没有任何真实分类。
    const wrapper = mountDropdown(0, [])
    await openDropdown(wrapper)

    // When: 用户点击唯一的新增分类选项。
    await wrapper.get("[role='option']").trigger("click")
    await flushPromises()

    // Then: 创建请求仍被发出，且没有删除按钮。
    expect(wrapper.emitted("request-create-article-type")).toEqual([[]])
    expect(wrapper.find("button[aria-label^='删除分类']").exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get("[role='combobox']").element)

    wrapper.unmount()
  })

  it("forwards the controlled disabled state through ArticleSettings", () => {
    // Given: 文章设置由父级启用变更锁。
    // When: 设置组件挂载并透传 disabled。
    const wrapper = mount(ArticleSettings, {
      props: { title: "测试文章", slug: "test-article", articleTypeId: 0, tagIds: [], articleTypes: ARTICLE_TYPES, tags: [], disabled: true },
    })

    // Then: 分类组合框收到原生禁用状态。
    expect(wrapper.get("[role='combobox']").attributes("disabled")).toBeDefined()
  })
})