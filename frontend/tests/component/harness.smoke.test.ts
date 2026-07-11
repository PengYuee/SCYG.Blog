import { mount } from "@vue/test-utils"
import { defineComponent } from "vue"
import { describe, expect, it } from "vitest"

/** T2 自包含组件，只验证 jsdom 与 Vue Test Utils 的挂载链路。 */
const HarnessStatus = defineComponent({
  props: {
    ready: { type: Boolean, required: true },
  },
  template: '<output data-testid="harness-status">{{ ready ? "ready" : "booting" }}</output>',
})

describe("T2 component harness", () => {
  it("renders the binary ready state when mounted in jsdom", () => {
    // Given: the harness receives a ready state.
    const ready = true

    // When: Vue Test Utils mounts it in the component project.
    const wrapper = mount(HarnessStatus, { props: { ready } })

    // Then: a binary DOM observable confirms the component harness works.
    expect(wrapper.get('[data-testid="harness-status"]').text()).toBe("ready")
  })
})
