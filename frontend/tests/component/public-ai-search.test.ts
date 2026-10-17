import { mount } from "@vue/test-utils"
import { defineComponent } from "vue"
import { afterEach, describe, expect, it, vi } from "vitest"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import type { AgentRun } from "@/request/generated"
import type { HttpTransport } from "@/request/transport"
import { HttpRequestError } from "@/request/http-error"
import { invalidateAnonymousSession } from "@/services/anonymous-ai-session"
import { usePublicAiSearch } from "@/services/public-ai-search"

const identity = { accessToken: "public-token", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 100, maxResults: 3 }
const run: AgentRun = { runId: "public-run", status: "queued", capability: "search", recipeId: "search-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: "/api/v1/runs/public-run/events", pendingInteraction: null, failure: null, result: null }
function setup() {
  const transport: HttpTransport = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
  const services = createApiServices(transport, "https://blog.test")
  const session = vi.spyOn(services.ai, "session").mockResolvedValue(identity)
  const create = vi.spyOn(services.ai, "create")
  let search!: ReturnType<typeof usePublicAiSearch>
  const component = defineComponent({ setup() { search = usePublicAiSearch(); return () => null } })
  const options = { global: { provide: { [apiServicesKey]: services } } }
  const wrapper = mount(component, options)
  return { wrapper, search, create, session, remount() { return mount(component, options) }, current() { return search } }
}
afterEach(() => { invalidateAnonymousSession(); sessionStorage.clear(); vi.restoreAllMocks(); vi.useRealTimers() })
describe("standalone public AI search", () => {
  it("only creates on submit, returns run identity, applies limits and restores last run", async () => {
    const fixture = setup()
    expect(fixture.session).not.toHaveBeenCalled()
    expect(fixture.create).not.toHaveBeenCalled()
    fixture.search.query.value = "public question"
    fixture.search.maxResults.value = 2
    fixture.create.mockResolvedValue(run)
    expect(await fixture.search.submit()).toBe("public-run")
    expect(fixture.search.limits).toEqual({ queryMaxChars: 100, maxResults: 3 })
    expect(fixture.create).toHaveBeenCalledWith("search", { query: "public question", max_results: 2 }, expect.any(String), "anonymous", expect.any(AbortSignal))
    expect(fixture.search.pending.value).toBeUndefined()
    expect(JSON.parse(sessionStorage.getItem("scyg.ai.anonymous.public-token.run.public-run.query") ?? "null")).toBe("public question")
    fixture.wrapper.unmount()
    const restored = fixture.remount()
    expect(fixture.current().lastRun.value).toBe("public-run")
    expect(fixture.current().query.value).toBe("public question")
    expect(fixture.create).toHaveBeenCalledOnce()
    restored.unmount()
  })
  it("retains unknown create input and key across remount and ignores edited draft on retry", async () => {
    const fixture = setup()
    fixture.create.mockRejectedValueOnce(new HttpRequestError("unknown", undefined, "NETWORK", undefined)).mockResolvedValueOnce(run)
    fixture.search.query.value = "original"
    fixture.search.maxResults.value = 2
    expect(await fixture.search.submit()).toBeUndefined()
    const original = fixture.create.mock.calls[0]
    expect(fixture.search.pending.value?.key).toBe(original?.[2])
    fixture.wrapper.unmount()
    const restored = fixture.remount()
    const search = fixture.current()
    search.query.value = "edited"
    search.maxResults.value = 1
    expect(await search.submit()).toBe("public-run")
    expect(fixture.create.mock.calls[1]?.slice(0, 4)).toEqual(original?.slice(0, 4))
    expect(fixture.session).toHaveBeenCalledOnce()
    expect(JSON.parse(sessionStorage.getItem("scyg.ai.anonymous.public-token.run.public-run.query") ?? "null")).toBe("original")
    restored.unmount()
  })
  it("invalidation aborts an in-flight create and prevents late completion from restoring it", async () => {
    const fixture = setup()
    const deferred = Promise.withResolvers<AgentRun>()
    const entered = Promise.withResolvers<void>()
    fixture.create.mockImplementation(() => { entered.resolve(); return deferred.promise })
    fixture.search.query.value = "question"
    fixture.search.maxResults.value = 2
    const submitted = fixture.search.submit()
    await entered.promise
    const signal = fixture.create.mock.calls[0]?.[4]
    invalidateAnonymousSession()
    expect(signal?.aborted).toBe(true)
    deferred.resolve(run)
    expect(await submitted).toBeUndefined()
    expect(fixture.search.invalidated.value).toBe(true)
    expect(fixture.search.lastRun.value).toBeUndefined()
    expect(fixture.search.canSubmit.value).toBe(false)
    fixture.wrapper.unmount()
  })
  it("rate limiting retains pending work and requires a manual retry after the deadline", async () => {
    vi.useFakeTimers()
    const fixture = setup()
    fixture.search.query.value = "question"
    fixture.search.maxResults.value = 2
    fixture.create.mockRejectedValue(new HttpRequestError("busy", 429, "RATE_LIMITED", undefined, undefined, 2))
    await fixture.search.submit()
    expect(fixture.search.waitSeconds.value).toBe(2)
    expect(fixture.search.canSubmit.value).toBe(false)
    await vi.advanceTimersByTimeAsync(2000)
    expect(fixture.search.canSubmit.value).toBe(true)
    expect(fixture.create).toHaveBeenCalledOnce()
    fixture.wrapper.unmount()
  })
  it("starts a fresh operation only after explicit restart following identity invalidation", async () => {
    // Given: 上一次创建失败且匿名身份随后失效。
    const fixture = setup()
    fixture.search.query.value = "old question"
    fixture.search.maxResults.value = 2
    fixture.create.mockRejectedValueOnce(new HttpRequestError("unknown", undefined, "NETWORK", undefined))
    await fixture.search.submit()
    const oldKey = fixture.search.pending.value?.key
    invalidateAnonymousSession()
    expect(fixture.search.canSubmit.value).toBe(false)
    // When: 用户明确开始新搜索，再输入新问题并提交。
    fixture.search.restart()
    fixture.session.mockResolvedValue({ ...identity, accessToken: "new-public-token" })
    fixture.create.mockResolvedValue(run)
    fixture.search.query.value = "new question"
    fixture.search.maxResults.value = 2
    expect(await fixture.search.submit()).toBe("public-run")
    // Then: 新身份使用新操作，旧未知请求不被重放。
    expect(fixture.session).toHaveBeenCalledTimes(2)
    expect(fixture.create.mock.calls[1]?.[2]).not.toBe(oldKey)
    expect(fixture.create.mock.calls[1]?.[1]).toEqual({ query: "new question", max_results: 2 })
    fixture.wrapper.unmount()
  })
  it("abandons expired local operation only on explicit action without creating another task", async () => {
    // Given: 创建结果未知，且原操作已超过幂等窗口。
    vi.useFakeTimers()
    const fixture = setup()
    fixture.search.query.value = "question"
    fixture.search.maxResults.value = 2
    fixture.create.mockRejectedValue(new HttpRequestError("unknown", undefined, "NETWORK", undefined))
    await fixture.search.submit()
    vi.setSystemTime(Date.now() + 86400001)
    await vi.advanceTimersByTimeAsync(250)
    expect(fixture.search.pendingExpired.value).toBe(true)
    // When: 确认 UI 调用显式放弃动作。
    fixture.search.abandonExpiredCreate()
    // Then: 只解除本地未知操作，不自动创建新任务。
    expect(fixture.search.pending.value).toBeUndefined()
    expect(fixture.create).toHaveBeenCalledOnce()
    fixture.wrapper.unmount()
  })
})
