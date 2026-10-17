import { flushPromises, mount } from "@vue/test-utils"
import { createMemoryHistory, createRouter } from "vue-router"
import { afterEach, expect, it, vi } from "vitest"
import HomeView from "@/views/public/HomeView.vue"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import type { HttpTransport } from "@/request/transport"
import type { AgentRun } from "@/request/generated"
import { invalidateAnonymousSession } from "@/services/anonymous-ai-session"
import { scrollRestorationKey } from "@/services/scroll-restoration"

afterEach(() => { invalidateAnonymousSession(); sessionStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks() })
it.each(["mode", "history", "destination", "overlap"])("respects %s navigation when AI creation completes later", async (navigation) => {
  // Given: 同一首页的 AI 创建保持在途。
  vi.stubGlobal("matchMedia", vi.fn(() => ({ matches: false })))
  const transport: HttpTransport = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
  const services = createApiServices(transport, "https://blog.test")
  vi.spyOn(services.article, "list").mockResolvedValue({ items: [], pageIndex: 0, pageSize: 9, totalItems: 0, totalPages: 0 })
  vi.spyOn(services.publicTaxonomy, "listArticleTypes").mockResolvedValue([])
  vi.spyOn(services.publicTaxonomy, "listTags").mockResolvedValue([])
  vi.spyOn(services.ai, "session").mockResolvedValue({ accessToken: "history-identity", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 })
  const run: AgentRun = { runId: "created-run", status: "succeeded", capability: "search", recipeId: "search-v1", recipeVersion: "v1", createdAt: "2026-10-16T00:00:00Z", updatedAt: "2026-10-16T00:00:00Z", streamUrl: "/api/v1/runs/created-run/events", pendingInteraction: null, failure: null, result: null }
  vi.spyOn(services.ai, "get").mockImplementation(async (id) => ({ ...run, runId: id }))
  const deferred = Promise.withResolvers<AgentRun>()
  const entered = Promise.withResolvers<void>()
  vi.spyOn(services.ai, "create").mockImplementation(() => { entered.resolve(); return deferred.promise })
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/", component: { template: "<div />" } }, { path: "/login", name: "login", component: { template: "<div />" } }] })
  await router.push("/?mode=ai")
  if (navigation === "history") await router.push("/?mode=ai&runId=previous-run")
  const wrapper = mount(HomeView, { props: { observerFactory: () => ({ observe: vi.fn(), disconnect: vi.fn() }) }, attachTo: document.body, global: { plugins: [router], provide: { [apiServicesKey]: services, [scrollRestorationKey]: { markReady: vi.fn() } } } })
  wrapper.get("#home-results").element.scrollIntoView = vi.fn()
  await wrapper.get("#hero-search-input").setValue("问题")
  await wrapper.get("form").trigger("submit")
  await entered.promise
  // When: 创建结束前，用户切模式或使用历史后退。
  if (navigation === "mode") {
    await wrapper.get('[aria-label="搜索方式"] button:nth-child(1)').trigger("click")
    await flushPromises()
  } else if (navigation === "history") {
    const changed = Promise.withResolvers<void>()
    const remove = router.afterEach(() => { remove(); changed.resolve() })
    router.back()
    await changed.promise
    await flushPromises()
  } else {
    const modeGate = Promise.withResolvers<void>()
    if (navigation === "overlap") {
      const modeEntered = Promise.withResolvers<void>()
      router.beforeEach((to) => {
        if (to.path === "/" && to.query["mode"] === undefined) { modeEntered.resolve(); return modeGate.promise }
      })
      await wrapper.get('[aria-label="搜索方式"] button:nth-child(1)').trigger("click")
      await modeEntered.promise
    }
    const destination = Promise.withResolvers<{ template: string }>()
    const destinationEntered = Promise.withResolvers<void>()
    router.addRoute({ path: "/login", name: "login", component: () => { destinationEntered.resolve(); return destination.promise } })
    const leaving = router.push("/login")
    await destinationEntered.promise
    deferred.resolve(run)
    await flushPromises()
    modeGate.resolve()
    destination.resolve({ template: "<div>login</div>" })
    await leaving
    expect(router.currentRoute.value.path).toBe("/login")
    expect(router.currentRoute.value.query["runId"]).toBeUndefined()
    wrapper.unmount()
    return
  }
  deferred.resolve(run)
  await flushPromises()
  await vi.dynamicImportSettled()
  await flushPromises()
  // Then: 模式保持当前选择；历史导航不会被迟到创建撤销。
  expect(router.currentRoute.value.query["mode"]).toBe(navigation === "mode" ? undefined : "ai")
  expect(router.currentRoute.value.query["runId"]).toBe(navigation === "mode" ? "created-run" : undefined)
  if (navigation === "history") expect(wrapper.findComponent({ name: "PublicSearchResult" }).exists()).toBe(false)
  wrapper.unmount()
})
