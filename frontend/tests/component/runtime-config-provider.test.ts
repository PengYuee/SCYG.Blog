import { mount } from "@vue/test-utils"
import { defineComponent, h } from "vue"
import { describe, expect, it } from "vitest"
import { RuntimeConfigError } from "@/config/runtime"
import { runtimeConfigKey, useRuntimeConfig } from "@/config/runtime-provider"

/** 读取并显示注入配置的测试组件。 */
const RuntimeConfigConsumer = defineComponent({
  setup() {
    const config = useRuntimeConfig()
    return () => h("output", config.serverUrl)
  },
})

describe("runtime config provider", () => {
  it("returns the typed config provided by the application", () => {
    // Given / When: 应用边界提供已解析配置。
    const wrapper = mount(RuntimeConfigConsumer, {
      global: { provide: { [runtimeConfigKey]: { serverUrl: "http://localhost:5000/api" } } },
    })

    // Then: 组件读取同一个运行时后端地址。
    expect(wrapper.text()).toBe("http://localhost:5000/api")
  })

  it("throws a Chinese typed error when the provider is missing", () => {
    // Given / When / Then: 绕过应用边界挂载会显式失败。
    expect(() => mount(RuntimeConfigConsumer)).toThrowError(new RuntimeConfigError("缺少运行时配置提供者", "CONFIG_INVALID", undefined))
  })
})