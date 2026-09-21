import { mount, type VueWrapper } from "@vue/test-utils"
import { describe, expect, it } from "vitest"
import { PlusIcon, XCircleIcon } from "@heroicons/vue/24/outline"
import ArticleSettings from "@/components/editor/ArticleSettings.vue"
import type { AuthorTag } from "@/services/author-contracts"

/** 文章设置测试使用的版本化标签资源。 */
const TAGS: readonly AuthorTag[] = [
  { id: 1, name: "Vue 3", version: 3 },
  { id: 2, name: "TypeScript", version: 5 },
]

/** 挂载具有受控标签选择状态的文章设置。 */
const mountSettings = (tagIds: readonly number[] = [1], disabled = false): VueWrapper => mount(ArticleSettings, {
  props: {
    title: "测试文章",
    slug: "test-article",
    articleTypeId: 0,
    tagIds,
    articleTypes: [],
    tags: TAGS,
    disabled,
  },
})

/** 按可访问名称查找标签新增按钮。 */
const createTagButton = (wrapper: VueWrapper): ReturnType<VueWrapper["get"]> => wrapper.get("button[aria-label='新增标签']")

/** 按标签名称查找独立删除按钮。 */
const deleteTagButton = (wrapper: VueWrapper, name: string): ReturnType<VueWrapper["get"]> => wrapper.get(`button[aria-label='删除标签“${name}”']`)

describe("ArticleSettings 标签设置", () => {
  it("发出新增标签请求并使用指定图标", async () => {
    // Given: 可编辑的标签设置。
    const wrapper = mountSettings()
    const createButton = createTagButton(wrapper)

    // When: 用户点击新增标签。
    await createButton.trigger("click")

    // Then: 按钮包含 PlusIcon 并只发出新增请求。
    expect(createButton.text()).toBe("新增标签")
    expect(createButton.findComponent(PlusIcon).exists()).toBe(true)
    expect(wrapper.emitted("request-create-tag")).toEqual([[]])
  })

  it("从受控选择中添加未选标签", async () => {
    // Given: Vue 3 已选中，TypeScript 未选中。
    const wrapper = mountSettings()
    const typeScriptCheckbox = wrapper.findAll("input[type='checkbox']")[1]

    // When: 用户选择 TypeScript。
    await typeScriptCheckbox?.setValue(true)

    // Then: 发出的新选择保留原标识并追加目标标识。
    expect(wrapper.emitted("update:tagIds")).toEqual([[[1, 2]]])
  })

  it("从受控选择中取消已选标签", async () => {
    // Given: 两个标签均已选中。
    const wrapper = mountSettings([1, 2])
    const vueCheckbox = wrapper.findAll("input[type='checkbox']")[0]

    // When: 用户取消 Vue 3。
    await vueCheckbox?.setValue(false)

    // Then: 发出的新选择仅保留 TypeScript。
    expect(wrapper.emitted("update:tagIds")).toEqual([[[2]]])
  })

  it("删除事件携带完整标签且不改变复选框选择", async () => {
    // Given: Vue 3 已选中，删除入口与复选框彼此独立。
    const wrapper = mountSettings()
    const deleteButton = deleteTagButton(wrapper, "Vue 3")
    const selectedCheckbox = wrapper.findAll<HTMLInputElement>("input[type='checkbox']")[0]

    // When: 用户点击 Vue 3 的删除按钮。
    await deleteButton.trigger("click")

    // Then: 只发出完整标签删除请求，受控复选框仍保持选中。
    expect(deleteButton.findComponent(XCircleIcon).exists()).toBe(true)
    expect(wrapper.emitted("request-delete-tag")).toEqual([[TAGS[0]]])
    expect(wrapper.emitted("update:tagIds")).toBeUndefined()
    expect(selectedCheckbox?.element.checked).toBe(true)
  })

  it("disabled 时锁定新增、选择和删除入口", async () => {
    // Given: 父级锁定全部标签交互。
    const wrapper = mountSettings([1], true)
    const createButton = createTagButton(wrapper)
    const deleteButtons = TAGS.map((tag) => deleteTagButton(wrapper, tag.name))
    const checkboxes = wrapper.findAll("input[type='checkbox']")

    // When: 测试环境尝试触发所有标签入口。
    await createButton.trigger("click")
    await deleteButtons[0]?.trigger("click")
    await checkboxes[0]?.trigger("change")

    // Then: 全部原生控件禁用且没有标签事件逸出。
    expect(createButton.attributes("disabled")).toBeDefined()
    expect(deleteButtons.every((button) => button.attributes("disabled") !== undefined)).toBe(true)
    expect(checkboxes.every((checkbox) => checkbox.attributes("disabled") !== undefined)).toBe(true)
    expect(wrapper.emitted("request-create-tag")).toBeUndefined()
    expect(wrapper.emitted("request-delete-tag")).toBeUndefined()
    expect(wrapper.emitted("update:tagIds")).toBeUndefined()
  })
})
