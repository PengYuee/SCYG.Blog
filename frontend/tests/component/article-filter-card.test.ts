import { flushPromises, mount } from "@vue/test-utils"
import { describe, expect, it } from "vitest"
import ArticleFilterCard from "@/components/public/ArticleFilterCard.vue"

const items = Array.from({ length: 30 }, (_, index) => ({ id: index + 1, name: `分类 ${index + 1}` }))

describe("sidebar article filters", () => {
  it("keeps a late selected category first and visible when collapsed", () => {
    // Given: 分类有三十个，选中最后一项。
    // When: 挂载折叠的分类卡片。
    const wrapper = mount(ArticleFilterCard, { props: { kind: "category", items, selectedId: 30 } })

    // Then: 选中项置前，仅显示六项，并具有独立取消按钮。
    const options = wrapper.findAll("[data-filter-id]")
    expect(options).toHaveLength(6)
    expect(options[0]?.attributes("data-filter-id")).toBe("30")
    expect(options[0]?.attributes("aria-pressed")).toBe("true")
    expect(wrapper.findAll(".filter-clear")).toHaveLength(1)
    expect(wrapper.get(".filter-clear").element.parentElement).toBe(options[0]?.element.parentElement)
    wrapper.unmount()
  })

  it("expands all categories without adding a taxonomy search input", async () => {
    // Given: 分类默认折叠。
    const wrapper = mount(ArticleFilterCard, { props: { kind: "category", items } })

    // When: 展开全部。
    await wrapper.get(".filter-toggle").trigger("click")

    // Then: 所有选项可达，选择只发出当前单个标识。
    expect(wrapper.findAll("[data-filter-id]")).toHaveLength(30)
    expect(wrapper.get(".filter-toggle").attributes("aria-expanded")).toBe("true")
    expect(wrapper.find("input").exists()).toBe(false)
    await wrapper.get('[data-filter-id="30"]').trigger("click")
    expect(wrapper.emitted("change")).toEqual([[30]])
    expect(wrapper.get(".filter-toggle").attributes("aria-expanded")).toBe("true")
    wrapper.unmount()
  })

  it("returns focus to a visible option after clearing a late collapsed selection", async () => {
    // Given: 最后一项被选中，取消按钮获得焦点。
    const wrapper = mount(ArticleFilterCard, { attachTo: document.body, props: { kind: "category", items, selectedId: 30 } })
    wrapper.get(".filter-clear").element.focus()

    // When: 父级应用取消结果。
    await wrapper.get(".filter-clear").trigger("click")
    await wrapper.setProps({ selectedId: undefined })
    await flushPromises()

    // Then: 单选被清除，焦点回到当前仍可见的首个选项。
    expect(wrapper.emitted("change")).toEqual([[]])
    expect(wrapper.find(".filter-clear").exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get("[data-filter-id]").element)
    wrapper.unmount()
  })

  it("restores the activated option after reordering without focusing unrelated route changes", async () => {
    // Given: 第二个分类通过键盘获得焦点。
    const wrapper = mount(ArticleFilterCard, { attachTo: document.body, props: { kind: "category", items } })
    const option = wrapper.get('[data-filter-id="2"]')
    option.element.focus()

    // When: 应用该选项并将它移到第一位。
    await option.trigger("click")
    await wrapper.setProps({ selectedId: 2 })
    await flushPromises()

    // Then: 激活的选项保持焦点，后续外部选择变化不主动抢走焦点。
    expect(document.activeElement).toBe(wrapper.get('[data-filter-id="2"]').element)
    const outside = document.createElement("button")
    document.body.append(outside)
    outside.focus()
    await wrapper.setProps({ selectedId: 3 })
    await flushPromises()
    expect(document.activeElement).toBe(outside)
    outside.remove()
    wrapper.unmount()
  })

  it("retains a selected tag while collapsing the expanded options", async () => {
    // Given: 标签卡展示全部且选中最后一个标签。
    const wrapper = mount(ArticleFilterCard, { props: { kind: "tag", items, selectedId: 30 } })
    await wrapper.get(".filter-toggle").trigger("click")

    // When: 收起标签。
    await wrapper.get(".filter-toggle").trigger("click")

    // Then: 只展示八项，选中标签仍在首位，收起不改变筛选。
    expect(wrapper.findAll("[data-filter-id]")).toHaveLength(8)
    expect(wrapper.get("[data-filter-id]").attributes("data-filter-id")).toBe("30")
    expect(wrapper.emitted("change")).toBeUndefined()
    wrapper.unmount()
  })

  it("preserves the currently focused option when an unapplied selection is superseded", async () => {
    // Given: 选择第二项后父级没有应用，用户已移到第四项。
    const wrapper = mount(ArticleFilterCard, { attachTo: document.body, props: { kind: "category", items, selectedId: 1 } })
    wrapper.get('[data-filter-id="2"]').element.focus()
    await wrapper.get('[data-filter-id="2"]').trigger("click")
    wrapper.get('[data-filter-id="4"]').element.focus()

    // When: 外部路由应用了第三项。
    await wrapper.setProps({ selectedId: 3 })
    await flushPromises()

    // Then: 不消费被忽略的旧选择，键盘位置仍在第四项。
    expect(document.activeElement).toBe(wrapper.get('[data-filter-id="4"]').element)
    wrapper.unmount()
  })

  it("does not reclaim focus moved outside before a selection is applied", async () => {
    // Given: 用户激活第二项后，在父级应用前把焦点移到其他控件。
    const wrapper = mount(ArticleFilterCard, { attachTo: document.body, props: { kind: "tag", items } })
    wrapper.get('[data-filter-id="2"]').element.focus()
    await wrapper.get('[data-filter-id="2"]').trigger("click")
    const outside = document.createElement("button")
    document.body.append(outside)
    outside.focus()

    // When: 父级稍后应用第二项。
    await wrapper.setProps({ selectedId: 2 })
    await flushPromises()

    // Then: 筛选更新不抢回用户已经移走的焦点。
    expect(document.activeElement).toBe(outside)
    outside.remove()
    wrapper.unmount()
  })
})
