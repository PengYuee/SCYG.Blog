import { beforeEach, describe, expect, it, vi } from "vitest"
import { apiServicesKey } from "@/request/api-services"
import { runtimeConfigKey } from "@/config/runtime-provider"
import { authSessionKey } from "@/services/auth-session"
import { scrollRestorationKey } from "@/services/scroll-restoration"

/** 记录应用组合根的调用顺序。 */
const calls: string[] = []
const services = { auth: {}, article: {}, articleImage: {}, articleType: {}, tag: {}, publicTaxonomy: {} }
const { apiServicesKeyMock, app, authSession, authSessionKeyMock, configureAuthTokenProvider, configureUnauthorizedHandler, createApiServices, createAuthNavigationGuard, createAuthSessionController, http, router, scrollRestoration, scrollRestorationKeyMock, useAuthStore } = vi.hoisted(() => {
  const app = {
    use: vi.fn(() => { calls.push("use"); return app }),
    provide: vi.fn((key: symbol) => { calls.push(key.description ?? "provide"); return app }),
    mount: vi.fn(() => { calls.push("mount") }),
  }
  const authSession = { restore: vi.fn(), accessToken: vi.fn(), handleUnauthorized: vi.fn(), currentState: vi.fn(), login: vi.fn(), logout: vi.fn() }
  return {
    apiServicesKeyMock: Symbol("api-services"),
    app,
    authSession,
    authSessionKeyMock: Symbol("auth-session"),
    configureAuthTokenProvider: vi.fn(),
    configureUnauthorizedHandler: vi.fn(),
    createApiServices: vi.fn(() => services),
    createAuthNavigationGuard: vi.fn(() => "auth-guard"),
    createAuthSessionController: vi.fn(() => authSession),
    http: { name: "shared-http" },
    router: { beforeEach: vi.fn(), name: "router" },
    scrollRestoration: { install: vi.fn(), arm: vi.fn(), cancel: vi.fn(), markReady: vi.fn(), wait: vi.fn() },
    scrollRestorationKeyMock: Symbol("scroll-restoration"),
    useAuthStore: vi.fn(() => ({ name: "auth-store" })),
  }
})

vi.mock("vue", async (importOriginal) => ({ ...(await importOriginal<typeof import("vue")>()), createApp: vi.fn(() => app) }))
vi.mock("pinia", () => ({ createPinia: vi.fn(() => ({ name: "pinia" })) }))
vi.mock("@/router", () => ({ router }))
vi.mock("@/router/guards", () => ({ createAuthNavigationGuard }))
vi.mock("@/stores/auth", () => ({ useAuthStore }))
vi.mock("@/services/auth-session", () => ({ authSessionKey: authSessionKeyMock, createAuthSessionController }))
vi.mock("@/App.vue", () => ({ default: {} }))
vi.mock("@/request/http", () => ({ configureAuthTokenProvider, configureUnauthorizedHandler, http }))
vi.mock("@/request/api-services", () => ({ apiServicesKey: apiServicesKeyMock, createApiServices }))
vi.mock("@/services/scroll-restoration", () => ({ scrollRestorationKey: scrollRestorationKeyMock, scrollRestoration }))

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
    expect(createAuthSessionController).toHaveBeenCalledOnce()
    expect(authSession.restore).toHaveBeenCalledOnce()
    expect(configureAuthTokenProvider).toHaveBeenCalledWith(authSession.accessToken)
    expect(configureUnauthorizedHandler).toHaveBeenCalledWith(authSession.handleUnauthorized)
    expect(router.beforeEach).toHaveBeenCalledWith("auth-guard")
    expect(app.provide).toHaveBeenCalledWith(authSessionKey, authSession)
    expect(calls.indexOf("scroll-restoration")).toBeLessThan(calls.lastIndexOf("use"))
    expect(calls.indexOf("api-services")).toBeLessThan(calls.indexOf("mount"))
    expect(calls.indexOf("runtime-config")).toBeLessThan(calls.indexOf("mount"))
  })
})
