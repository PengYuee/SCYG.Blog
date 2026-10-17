import { flushPromises, mount } from "@vue/test-utils"
import { createMemoryHistory, createRouter } from "vue-router"
import { afterEach, describe, expect, it, vi } from "vitest"
import AiWorkspace from "@/components/ai/AiWorkspace.vue"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import { authSessionKey, type AuthSessionController } from "@/services/auth-session"
import { HttpRequestError } from "@/request/http-error"
import type { HttpTransport } from "@/request/transport"
import { authorTabSubject, clearAuthorAiState, saveTabState } from "@/services/ai-session-state"
import AiArticlePicker from "@/components/ai/AiArticlePicker.vue"
import type { ArticleDetail } from "@/types/article"
const article: ArticleDetail = { id: 1, title: "私有候选 A", slug: "a", digest: "私有摘要", markdown: "正文", articleTypeId: 1, articleType: { id: 1, name: "分类", imageUrl: null }, tagIds: [], status: 1, support: 0, comment: 0, visited: 0, version: 1, createdAt: "2026-10-10T00:00:00Z", updatedAt: null }
const session = { accessToken: "author-token", tokenType: "Bearer" as const, expiresAt: "2099-01-01T00:00:00Z" }
const auth: AuthSessionController = { restore: () => ({ kind: "authenticated", session }), login: async () => session, logout: () => ({ kind: "anonymous" }), currentState: () => ({ kind: "authenticated", session }), accessToken: () => session.accessToken, handleUnauthorized: () => undefined }
afterEach(() => { clearAuthorAiState(); sessionStorage.clear(); vi.restoreAllMocks(); vi.useRealTimers() })
describe("AI workspace operation recovery", () => {
  it("unknown create keeps original body and key across refresh instead of duplicating logical work", async () => {
    const client: HttpTransport = { get: vi.fn(), post: vi.fn(async () => { throw new HttpRequestError("请求结果不明", undefined, "NETWORK", undefined) }), patch: vi.fn(), delete: vi.fn() }
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/ai", component: { template: "<div />" } }] })
    await router.push("/author/ai")
    const options = { props: { anonymous: false }, global: { plugins: [router], provide: { [apiServicesKey]: createApiServices(client, "https://blog.test"), [authSessionKey]: auth } } }
    let wrapper = mount(AiWorkspace, options)
    await wrapper.get("textarea").setValue("保留原始问题")
    await wrapper.get("form").trigger("submit")
    await flushPromises()
    expect(wrapper.text()).toContain("上一创建操作结果尚未确认")
    wrapper.unmount()
    wrapper = mount(AiWorkspace, options)
    expect(wrapper.get("textarea").element.value).toBe("保留原始问题")
    await wrapper.get("form").trigger("submit")
    await flushPromises()
    const calls = vi.mocked(client.post).mock.calls
    expect(calls).toHaveLength(2)
    expect(calls[0]?.[1]).toEqual({ query: "保留原始问题", max_results: 5 })
    expect(calls[1]?.[1]).toEqual(calls[0]?.[1])
    const firstConfig = calls[0]?.[2]
    const secondConfig = calls[1]?.[2]
    if (typeof firstConfig !== "object" || firstConfig === null || !("headers" in firstConfig) || typeof secondConfig !== "object" || secondConfig === null || !("headers" in secondConfig)) throw new Error("Expected request headers")
    expect(secondConfig.headers).toEqual(firstConfig.headers)
    wrapper.unmount()
  })
  it("429 keeps input, disables resubmit until deadline, and never creates automatically", async () => {
    vi.useFakeTimers()
    const client: HttpTransport = { get: vi.fn(), post: vi.fn(async () => { throw new HttpRequestError("繁忙", 429, "RATE_LIMITED", undefined, undefined, 2) }), patch: vi.fn(), delete: vi.fn() }
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/ai", component: { template: "<div />" } }] })
    await router.push("/author/ai")
    const wrapper = mount(AiWorkspace, { props: { anonymous: false }, global: { plugins: [router], provide: { [apiServicesKey]: createApiServices(client, "https://blog.test"), [authSessionKey]: auth } } })
    await wrapper.get("textarea").setValue("原问题")
    await wrapper.get("form").trigger("submit")
    await vi.advanceTimersByTimeAsync(0)
    expect(wrapper.get('button[type="submit"]').attributes("disabled")).toBeDefined()
    await wrapper.get("form").trigger("submit")
    expect(client.post).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(2500)
    expect(wrapper.get('button[type="submit"]').attributes("disabled")).toBeUndefined()
    expect(client.post).toHaveBeenCalledOnce()
    expect(wrapper.get("textarea").element.value).toBe("原问题")
    wrapper.unmount()
  })
  it("expired unknown create requires confirmation, keeps an editable draft, and never replays its old key", async () => {
    vi.useFakeTimers()
    const client: HttpTransport = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
    const services = createApiServices(client, "https://blog.test")
    const create = vi.spyOn(services.ai, "create").mockRejectedValue(new HttpRequestError("请求结果不明", undefined, "NETWORK", undefined))
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/ai", component: { template: "<div />" } }] })
    await router.push("/author/ai")
    const options = { props: { anonymous: false }, global: { plugins: [router], provide: { [apiServicesKey]: services, [authSessionKey]: auth } } }
    let wrapper = mount(AiWorkspace, options)
    await wrapper.get("textarea").setValue("原草稿")
    await wrapper.get("form").trigger("submit")
    await vi.advanceTimersByTimeAsync(0)
    const oldKey = create.mock.calls[0]?.[2]
    wrapper.unmount()
    vi.setSystemTime(Date.now() + 86400001)
    wrapper = mount(AiWorkspace, options)
    expect(wrapper.text()).toContain("旧操作可能已执行并产生费用")
    await wrapper.get("form").trigger("submit")
    await vi.advanceTimersByTimeAsync(500)
    expect(create).toHaveBeenCalledOnce()
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false)
    await wrapper.get('button[type="button"]').trigger("click")
    expect(wrapper.get('button[type="submit"]').attributes("disabled")).toBeDefined()
    expect(wrapper.get("textarea").element.value).toBe("原草稿")
    confirm.mockReturnValue(true)
    await wrapper.get('button[type="button"]').trigger("click")
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining("不会取消远端任务"))
    expect(wrapper.get("textarea").element.value).toBe("原草稿")
    expect(wrapper.get("fieldset").attributes("disabled")).toBeUndefined()
    await wrapper.get("textarea").setValue("编辑后的草稿")
    await wrapper.get("form").trigger("submit")
    await vi.advanceTimersByTimeAsync(0)
    expect(create).toHaveBeenCalledTimes(2)
    expect(create.mock.calls[1]?.[1]).toEqual({ query: "编辑后的草稿", max_results: 5 })
    expect(create.mock.calls[1]?.[2]).not.toBe(oldKey)
    expect(create.mock.calls[1]?.[2]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    wrapper.unmount()
  })

  it("cross-tab author invalidation hides private drafts, pending work, last run and article candidates", async () => {
    const client: HttpTransport = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
    const services = createApiServices(client, "https://blog.test")
    vi.spyOn(services.article, "manageList").mockResolvedValue({ items: [article], totalItems: 1, totalPages: 1, pageIndex: 0, pageSize: 10 })
    const draft = { capability: "write", query: "私有搜索", maxResults: 5, topic: "私有主题", requirements: "私有要求", content: "私有正文", message: "私有问题", requestRequirements: false, referenceMode: "manual", referenceQuery: "私有关键词", referenceIds: [] }
    const key = `scyg.ai.author.${authorTabSubject(session.accessToken)}.workspace`
    saveTabState(key, { form: draft, lastRun: "private-run" })
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/ai", component: { template: "<div />" } }] })
    await router.push("/author/ai")
    const wrapper = mount(AiWorkspace, { props: { anonymous: false }, global: { plugins: [router], provide: { [apiServicesKey]: services, [authSessionKey]: auth } } })
    await wrapper.getComponent(AiArticlePicker).findAll("button")[0]?.trigger("click")
    await flushPromises()
    expect(wrapper.text()).toContain("私有候选 A")
    const { promise, reject } = Promise.withResolvers<never>()
    const create = vi.spyOn(services.ai, "create").mockReturnValue(promise)
    await wrapper.get("form").trigger("submit")
    const signal = create.mock.calls[0]?.[4]
    clearAuthorAiState()
    expect(signal?.aborted).toBe(true)
    reject(new Error("旧请求在失效后完成"))
    await flushPromises()
    expect(wrapper.text()).toContain("作者会话已变更")
    expect(wrapper.find("form").exists()).toBe(false)
    expect(wrapper.findComponent(AiArticlePicker).exists()).toBe(false)
    expect(wrapper.text()).not.toContain("私有")
    expect(wrapper.find('a[href*="private-run"]').exists()).toBe(false)
    expect(sessionStorage.getItem(key)).toBeNull()
    wrapper.unmount()
  })

  it("picker pagination stays on submitted query A until query B is explicitly submitted at page zero", async () => {
    const client: HttpTransport = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
    const services = createApiServices(client, "https://blog.test")
    const list = vi.spyOn(services.article, "manageList")
      .mockResolvedValueOnce({ items: [article], totalItems: 3, totalPages: 3, pageIndex: 0, pageSize: 10 })
      .mockResolvedValueOnce({ items: [{ ...article, id: 2, title: "A 第二页" }], totalItems: 3, totalPages: 3, pageIndex: 1, pageSize: 10 })
      .mockRejectedValueOnce(new Error("B 查询失败"))
      .mockResolvedValueOnce({ items: [{ ...article, id: 3, title: "B 第一页" }], totalItems: 1, totalPages: 1, pageIndex: 0, pageSize: 10 })
    const wrapper = mount(AiArticlePicker, { props: { modelValue: [], disabled: false }, global: { provide: { [apiServicesKey]: services } } })
    await wrapper.get('input[type="search"]').setValue("A")
    await wrapper.findAll("button")[0]?.trigger("click")
    await flushPromises()
    await wrapper.get('input[type="search"]').setValue("B")
    await wrapper.findAll("button").find((button) => button.text() === "加载更多文章")?.trigger("click")
    await flushPromises()
    expect(list.mock.calls[1]?.[0]).toEqual({ q: "A", pageModel: { pageIndex: 1, pageSize: 10 } })
    expect(wrapper.text()).toContain("A 第二页")
    await wrapper.findAll("button")[0]?.trigger("click")
    await flushPromises()
    expect(list.mock.calls[2]?.[0]).toEqual({ q: "B", pageModel: { pageIndex: 0, pageSize: 10 } })
    expect(wrapper.text()).not.toContain("私有候选 A")
    expect(wrapper.text()).not.toContain("A 第二页")
    await wrapper.findAll("button")[0]?.trigger("click")
    await flushPromises()
    expect(list.mock.calls[3]?.[0]).toEqual({ q: "B", pageModel: { pageIndex: 0, pageSize: 10 } })
    expect(wrapper.text()).toContain("B 第一页")
    expect(wrapper.text()).not.toContain("A 第二页")
    wrapper.unmount()
  })

  it("a known rejected input remains editable and can be submitted as a new operation", async () => {
    const client: HttpTransport = { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
    const services = createApiServices(client, "https://blog.test")
    const create = vi.spyOn(services.ai, "create").mockRejectedValue(new HttpRequestError("输入不合法", 400, "INVALID_INPUT", undefined))
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/ai", component: { template: "<div />" } }] })
    await router.push("/author/ai")
    const wrapper = mount(AiWorkspace, { props: { anonymous: false }, global: { plugins: [router], provide: { [apiServicesKey]: services, [authSessionKey]: auth } } })
    await wrapper.get("textarea").setValue("原输入")
    await wrapper.get("form").trigger("submit")
    await flushPromises()
    expect(wrapper.get("textarea").element.value).toBe("原输入")
    expect(wrapper.get("fieldset").attributes("disabled")).toBeUndefined()
    expect(wrapper.text()).not.toContain("上一创建操作结果尚未确认")
    await wrapper.get("textarea").setValue("修正输入")
    await wrapper.get("form").trigger("submit")
    await flushPromises()
    expect(create).toHaveBeenCalledTimes(2)
    expect(create.mock.calls[1]?.[1]).toEqual({ query: "修正输入", max_results: 5 })
    expect(create.mock.calls[1]?.[2]).not.toBe(create.mock.calls[0]?.[2])
    wrapper.unmount()
  })
})
