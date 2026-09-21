import { beforeEach, describe, expect, it, vi } from "vitest"
import { apiServicesKey } from "@/request/api-services"
import { runtimeConfigKey } from "@/config/runtime-provider"
import { scrollRestorationKey } from "@/services/scroll-restoration"

/** 记录应用组合根的调用顺序。 */
const calls: string[] = []
const services = { article: {}, articleType: {}, tag: {} }
const { app, createApiServices, http, scrollRestoration } = vi.hoisted(() => {
  const app = {
    use: vi.fn(() => { calls.push("use"); return app }),
    provide: vi.fn((key: symbol) => { calls.push(key.description ?? "provide"); return app }),
    mount: vi.fn(() => { calls.push("mount") }),
  }
  return {
    app,
    createApiServices: vi.fn(() => services),
    http: { name: "shared-http" },
    scrollRestoration: { install: vi.fn(), arm: vi.fn(), cancel: vi.fn(), markReady: vi.fn(), wait: vi.fn() },
  }
})

vi.mock("vue", async (importOriginal) => ({ ...(await importOriginal<typeof import("vue")>()), createApp: vi.fn(() => app) }))
vi.mock("pinia", () => ({ createPinia: vi.fn(() => ({ name: "pinia" })) }))
vi.mock("@/router", () => ({ router: { name: "router" } }))
vi.mock("@/App.vue", () => ({ default: {} }))
vi.mock("@/request/http", () => ({ http }))
vi.mock("@/request/api-services", async (importOriginal) => ({ ...(await importOriginal<typeof import("@/request/api-services")>()), createApiServices }))
vi.mock("@/services/scroll-restoration", async (importOriginal) => ({ ...(await importOriginal<typeof import("@/services/scroll-restoration")>()), scrollRestoration }))

import { mountApplication } from "@/application"

describe("application composition root", () => {
  beforeEach(() => { calls.length = 0; vi.clearAllMocks() })

  it("creates and provides one API services container before mounting", () => {
    // Given: 已配置共享 HTTP 与运行时 API 根地址。
    const config = { serverUrl: "http://localhost:5000/api" }

    // When: 应用完成一次挂载。
    mountApplication(config)

    // Then: 容器只创建一次，并在挂载前与运行时配置一同提供。
    expect(createApiServices).toHaveBeenCalledOnce()
    expect(createApiServices).toHaveBeenCalledWith(http, config.serverUrl)
    expect(app.provide).toHaveBeenCalledWith(apiServicesKey, services)
    expect(app.provide).toHaveBeenCalledWith(runtimeConfigKey, config)
    expect(scrollRestoration.install).toHaveBeenCalledOnce()
    expect(app.provide).toHaveBeenCalledWith(scrollRestorationKey, scrollRestoration)
    expect(calls.indexOf("scroll-restoration")).toBeLessThan(calls.lastIndexOf("use"))
    expect(calls.indexOf("api-services")).toBeLessThan(calls.indexOf("mount"))
    expect(calls.indexOf("runtime-config")).toBeLessThan(calls.indexOf("mount"))
  })
})
