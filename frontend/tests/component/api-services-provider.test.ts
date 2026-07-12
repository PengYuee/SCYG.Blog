import { mount } from "@vue/test-utils"
import { defineComponent, h } from "vue"
import { describe, expect, it, vi } from "vitest"
import { apiServicesKey, createApiServices, useApiServices } from "@/request/api-services"
import type { HttpTransport } from "@/request/transport"

/** 创建不会发出真实网络请求的完整传输层。 */
const createTransport = (): HttpTransport => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
})

/** 记录组件读取到的服务引用。 */
const observedServices: ReturnType<typeof useApiServices>[] = []
/** 读取并记录注入容器的测试消费者。 */
const ApiServicesConsumer = defineComponent({
  setup() {
    const services = useApiServices()
    observedServices.push(services)
    return () => h("output", "已提供 API 服务")
  },
})

describe("API services provider", () => {
  it("returns the exact provided container", () => {
    // Given: 应用边界创建并提供一个服务容器。
    observedServices.length = 0
    const services = createApiServices(createTransport(), "http://localhost:5000/api")

    // When: 消费者读取容器。
    mount(ApiServicesConsumer, { global: { provide: { [apiServicesKey]: services } } })

    // Then: 返回值与提供值严格同一引用。
    expect(observedServices).toEqual([services])
  })

  it("reuses one container and adapter identity across two consumers", () => {
    // Given: 两个消费者位于同一提供者下。
    observedServices.length = 0
    const services = createApiServices(createTransport(), "http://localhost:5000/api")
    const Parent = defineComponent({ setup: () => () => h("div", [h(ApiServicesConsumer), h(ApiServicesConsumer)]) })

    // When: 父组件挂载。
    mount(Parent, { global: { provide: { [apiServicesKey]: services } } })

    // Then: 容器及三个适配器均保持引用身份。
    expect(observedServices).toHaveLength(2)
    expect(observedServices[0]).toBe(services)
    expect(observedServices[1]).toBe(services)
    expect(observedServices[0]?.article).toBe(observedServices[1]?.article)
    expect(observedServices[0]?.articleType).toBe(observedServices[1]?.articleType)
    expect(observedServices[0]?.tag).toBe(observedServices[1]?.tag)
  })

  it("throws the approved Chinese error when provider is missing", () => {
    // Given / When / Then: 绕过应用边界挂载会以稳定中文错误失败。
    expect(() => mount(ApiServicesConsumer)).toThrowError("缺少 API 服务提供者")
  })
})
