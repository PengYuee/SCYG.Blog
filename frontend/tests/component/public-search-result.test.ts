import { flushPromises, mount, type VueWrapper } from "@vue/test-utils"
import { defineComponent, reactive, ref } from "vue"
import { createMemoryHistory, createRouter, type Router } from "vue-router"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import type { AgentRun } from "@/request/generated"
import type { AiRunController, AiRunState } from "@/services/ai-run"
import type * as AiRunModule from "@/services/ai-run"
import PublicSearchResult from "@/components/ai/PublicSearchResult.vue"
import ArticleSection from "@/components/article/ArticleSection.vue"
import { HttpRequestError } from "@/request/http-error"
import type { ArticleSummary } from "@/types/article"

const mocks = vi.hoisted(() => ({ create: vi.fn(), ensure: vi.fn(), detail: vi.fn<(id: number) => Promise<ArticleSummary>>(), invalidate: undefined as (() => void) | undefined }))
vi.mock("@/request/api-services", () => ({ useApiServices: () => ({ ai: {}, article: { detail: mocks.detail } }) }))
vi.mock("@/services/anonymous-ai-session", () => ({
  ensureAnonymousSession: mocks.ensure,
  onAnonymousSessionChange: (listener: () => void) => { mocks.invalidate = listener; return () => { mocks.invalidate = undefined } },
}))
vi.mock("@/services/ai-run", async (importOriginal) => ({ ...await importOriginal<typeof AiRunModule>(), createAiRun: mocks.create }))
function lifecycle(runId: string): AiRunController {
  const state = reactive<AiRunState>({ run: { runId, status: "running", capability: "search", recipeId: "search-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: `/api/v1/runs/${runId}/events`, pendingInteraction: null, failure: null, result: null }, error: null, connection: "connected", busy: false, waitUntil: 0, now: Date.now(), activities: [], original: "", interactionDrafts: {}, pendingOperation: null })
  return { state, connect: vi.fn(async () => undefined), refresh: vi.fn(async () => undefined), stop: vi.fn(), mutate: vi.fn(async () => undefined), retryPending: vi.fn(async () => undefined), persist: vi.fn() }
}
const article = (id: number): ArticleSummary => ({
  id, title: `公开文章 ${id}`, slug: `article-${id}`, digest: `摘要 ${id}`, markdown: "正文",
  articleTypeId: 1, articleType: { id: 1, name: "后端", imageUrl: null }, tagIds: [], status: 2,
  support: 0, comment: 0, visited: id, version: 1, createdAt: "2026-10-10T00:00:00Z", updatedAt: null,
})
function completed(runId: string, ids: readonly number[]): AgentRun {
  return {
    runId, status: "succeeded", capability: "search", recipeId: "search-v1", recipeVersion: "v1",
    createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: `/api/v1/runs/${runId}/events`,
    pendingInteraction: null, failure: null,
    result: { conclusion: "UNRENDERED_CONCLUSION_SENTINEL", articles: ids.map(id => ({ article_id: id, title: `引用 ${id}`, visibility: "published", evidence: "UNRENDERED_EVIDENCE_SENTINEL" })) },
  }
}
const wrappers: VueWrapper[] = []
let router: Router
beforeEach(async () => {
  router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/", component: { template: "<div />" } }, { path: "/articles/:id", component: { template: "<div />" } }] })
  await router.push("/")
  await router.isReady()
  mocks.ensure.mockResolvedValue({ accessToken: "anonymous-token" })
  mocks.detail.mockImplementation(async id => article(id))
  vi.stubGlobal("ResizeObserver", class { observe(): void {} unobserve(): void {} disconnect(): void {} })
})
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  mocks.create.mockReset(); mocks.ensure.mockReset(); mocks.detail.mockReset(); mocks.invalidate = undefined
  sessionStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals()
})
async function mountResult(runId = "first"): Promise<VueWrapper> {
  const wrapper = mount(PublicSearchResult, { props: { runId }, global: { plugins: [router] } })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
describe("prop-driven public search result", () => {
  it("keeps its subscription while hidden, renders anonymous results and cancels only explicitly", async () => {
    // Given: 已运行的公共搜索，组件只随场景隐藏。
    const current = lifecycle("first")
    mocks.create.mockReturnValue(current)
    const visible = ref(true)
    const host = defineComponent({ components: { PublicSearchResult }, setup: () => ({ visible }), template: '<PublicSearchResult v-show="visible" run-id="first" />' })
    const wrapper = mount(host, { global: { plugins: [router] } })
    wrappers.push(wrapper)
    await flushPromises()
    expect(mocks.ensure).toHaveBeenCalledWith({}, false, expect.any(AbortSignal))
    expect(mocks.create).toHaveBeenCalledWith({}, "anonymous", "first", "scyg.ai.anonymous.anonymous-token.run.first")
    expect(wrapper.getComponent(ArticleSection).props("list")).toBe(true)
    expect(wrapper.find(".search-result-spinner").exists()).toBe(true)
    expect(wrapper.find("details").exists()).toBe(false)
    visible.value = false
    await flushPromises()
    expect(current.stop).not.toHaveBeenCalled()
    expect(current.mutate).not.toHaveBeenCalled()
    visible.value = true
    await flushPromises()
    expect(current.connect).toHaveBeenCalledOnce()
    // When: 用户明确打开取消确认。
    await wrapper.get('[data-testid="cancel-public-search"]').trigger("click")
    await flushPromises()
    expect(current.mutate).not.toHaveBeenCalled()
    const confirmation = [...document.querySelectorAll<HTMLButtonElement>('[role="dialog"] button')].find((button) => button.textContent?.trim() === "确认取消任务")
    if (confirmation === undefined) throw new Error("Missing cancellation confirmation")
    confirmation.click()
    await flushPromises()
    expect(current.mutate).toHaveBeenCalledWith("cancel")
    // Then: 隐藏与显示不结束订阅；卸载才停止本地连接。
    wrapper.unmount()
    wrappers.splice(wrappers.indexOf(wrapper), 1)
    expect(current.stop).toHaveBeenCalledOnce()
  })
  it.each(["prop", "identity"])("does not apply stale refresh errors after %s changes", async (change) => {
    const first = lifecycle("first")
    const second = lifecycle("second")
    const deferred = Promise.withResolvers<AgentRun | undefined>()
    const initialFailure = new Error("initial read failure")
    first.state.error = initialFailure
    vi.mocked(first.refresh).mockReturnValue(deferred.promise)
    mocks.create.mockReturnValueOnce(first).mockReturnValueOnce(second)
    const wrapper = await mountResult()
    await wrapper.get('[data-testid="retry-public-search"]').trigger("click")
    if (change === "prop") await wrapper.setProps({ runId: "second" })
    else mocks.invalidate?.()
    await flushPromises()
    deferred.reject(new Error("stale failure"))
    await flushPromises()
    expect(first.state.error).toBe(initialFailure)
    expect(second.state.error).toBeNull()
    expect(first.stop).toHaveBeenCalledOnce()
    expect(wrapper.find(".search-result-spinner").exists()).toBe(change === "prop")
    expect(first.mutate).not.toHaveBeenCalled()
  })

  it("replaces the inline progress with the ordinary article cards when the snapshot completes", async () => {
    // Given: 搜索仍在执行，标题和说明已经显示。
    const current = lifecycle("first")
    mocks.create.mockReturnValue(current)
    const wrapper = await mountResult()
    const heading = wrapper.get("h2").element
    expect(wrapper.find(".search-result-spinner").exists()).toBe(true)

    // When: 收到包含两篇文章的完成快照。
    current.state.run = completed("first", [2, 1, 2])
    await flushPromises()

    // Then: 标题不替换，卡片保持引用顺序并去重，说明位于标题下。
    expect(wrapper.get("h2").element).toBe(heading)
    expect(wrapper.find(".search-result-spinner").exists()).toBe(false)
    expect(wrapper.getComponent(ArticleSection).props("articles").map((item: ArticleSummary) => item.id)).toEqual([2, 1])
    expect(wrapper.findAll(".article-list article")).toHaveLength(2)
    expect(wrapper.findAll(".article-list a").map(link => link.attributes("href"))).toEqual(["/articles/2", "/articles/1"])
    expect(wrapper.text()).not.toContain("UNRENDERED_CONCLUSION_SENTINEL")
    expect(wrapper.text()).not.toContain("UNRENDERED_EVIDENCE_SENTINEL")
    expect(wrapper.find(".ai-panel").exists()).toBe(false)
  })

  it("uses the accepted run topic and does not fetch non-public citations", async () => {
    // Given: Run 的提交主题独立保存，返回中混有非公开引用。
    sessionStorage.setItem("scyg.ai.anonymous.anonymous-token.run.first.query", JSON.stringify("数据库迁移"))
    const current = lifecycle("first")
    current.state.run = {
      ...completed("first", []),
      result: { conclusion: "", articles: [
        { article_id: 1, title: "公开", visibility: "published", evidence: "" },
        { article_id: 2, title: "非公开", visibility: "draft", evidence: "" },
      ] },
    }
    mocks.create.mockReturnValue(current)

    // When: 读取完成结果。
    const wrapper = await mountResult()

    // Then: 说明使用已提交主题，只读取可公开引用的文章。
    expect(wrapper.get('[data-testid="ai-search-status"]').text()).toContain("数据库迁移")
    expect(mocks.detail.mock.calls.map(call => call[0])).toEqual([1])
    expect(wrapper.getComponent(ArticleSection).props("articles").map((item: ArticleSummary) => item.id)).toEqual([1])
  })

  it("shows completed results instead of an earlier connection error", async () => {
    // Given: 搜索曾短暂断连，但最终快照已成功。
    sessionStorage.setItem("scyg.ai.anonymous.anonymous-token.run.first.query", JSON.stringify("TOPIC_SENTINEL"))
    const current = lifecycle("first")
    current.state.error = new Error("STALE_CONNECTION_SENTINEL")
    current.state.run = completed("first", [1])
    mocks.create.mockReturnValue(current)

    // When: 读取最终结果。
    const wrapper = await mountResult()

    // Then: 最终快照决定展示，不让历史断连错误掩盖成功结果。
    expect(wrapper.get('[data-testid="ai-search-status"]').text()).toContain("TOPIC_SENTINEL")
    expect(wrapper.find('[data-testid="retry-public-search"]').exists()).toBe(false)
    expect(wrapper.findAll(".article-list article")).toHaveLength(1)
  })

  it("does not clear a newer cancellation error when an older refresh is aborted", async () => {
    // Given: 一次重试查询还在途，随后取消操作产生了新的未知结果。
    const current = lifecycle("first")
    current.state.error = new Error("initial connection failure")
    const refreshed = Promise.withResolvers<AgentRun | undefined>()
    vi.mocked(current.refresh).mockReturnValue(refreshed.promise)
    mocks.create.mockReturnValue(current)
    const wrapper = await mountResult()
    await wrapper.get('[data-testid="retry-public-search"]').trigger("click")
    const cancellationError = new Error("unknown cancellation")
    current.state.pendingOperation = "cancel"
    current.state.error = cancellationError

    // When: 被取消中止的旧查询按 controller 契约返回 undefined。
    refreshed.resolve(undefined)
    await flushPromises()

    // Then: 旧查询不能清除较新的操作失败状态。
    expect(current.state.error).toBe(cancellationError)
    expect(current.state.pendingOperation).toBe("cancel")
  })

  it.each(["failed", "cancelled"] as const)("retains readable articles returned with a %s snapshot", async (status) => {
    // Given: 终态仍带有此前已返回的文章引用。
    const current = lifecycle("first")
    current.state.run = { ...completed("first", [1]), status }
    mocks.create.mockReturnValue(current)

    // When: 展示终态快照。
    const wrapper = await mountResult()

    // Then: 不丢弃已有结果，也不把失败或取消伪装为仍在执行。
    expect(wrapper.findAll(".article-list article")).toHaveLength(1)
    expect(wrapper.find(".search-result-spinner").exists()).toBe(false)
    expect(current.mutate).not.toHaveBeenCalled()
  })

  it("keeps successful matches and can retry a failed article read without creating a new run", async () => {
    // Given: 两篇引用中一篇暂时读取失败。
    const current = lifecycle("first")
    current.state.run = completed("first", [1, 2])
    mocks.create.mockReturnValue(current)
    mocks.detail.mockImplementation(async id => { if (id === 2) throw new HttpRequestError("unavailable", 503, "UNAVAILABLE", undefined); return article(id) })
    const wrapper = await mountResult()
    expect(wrapper.getComponent(ArticleSection).props("articles").map((item: ArticleSummary) => item.id)).toEqual([1])

    // When: 用户重试文章读取。
    mocks.detail.mockImplementation(async id => article(id))
    await wrapper.get('[data-testid="retry-search-articles"]').trigger("click")
    await flushPromises()

    // Then: 原 Run 补齐结果，不创建或取消其他任务。
    expect(wrapper.getComponent(ArticleSection).props("articles").map((item: ArticleSummary) => item.id)).toEqual([1, 2])
    expect(wrapper.find('[data-testid="retry-search-articles"]').exists()).toBe(false)
    expect(mocks.create).toHaveBeenCalledOnce()
    expect(current.mutate).not.toHaveBeenCalled()
  })

  it("does not reread a successful match when retrying another failed article", async () => {
    // Given: 第一篇已读取，第二篇需要重试。
    const current = lifecycle("first")
    current.state.run = completed("first", [1, 2])
    mocks.create.mockReturnValue(current)
    mocks.detail.mockImplementation(async id => { if (id === 2) throw new HttpRequestError("busy", 503, "UNAVAILABLE", undefined); return article(id) })
    const wrapper = await mountResult()

    // When: 第二次只有第二篇可读取，第一篇接口此时临时不可用。
    mocks.detail.mockImplementation(async id => { if (id === 1) throw new HttpRequestError("busy", 503, "UNAVAILABLE", undefined); return article(id) })
    await wrapper.get('[data-testid="retry-search-articles"]').trigger("click")
    await flushPromises()

    // Then: 已有成功结果保留，且只重试失败文章。
    expect(wrapper.getComponent(ArticleSection).props("articles").map((item: ArticleSummary) => item.id)).toEqual([1, 2])
    expect(mocks.detail.mock.calls.map(call => call[0])).toEqual([1, 2, 2])
  })

  it("reconciles an unknown cancellation with a snapshot read rather than replaying it", async () => {
    // Given: 取消结果未知，原订阅已停止。
    const current = lifecycle("first")
    current.state.pendingOperation = "cancel"
    current.state.error = new Error("unknown cancellation")
    mocks.create.mockReturnValue(current)
    vi.mocked(current.refresh).mockImplementation(async () => {
      const result = completed("first", [])
      current.state.run = result
      current.state.pendingOperation = null
      return result
    })
    const wrapper = await mountResult()

    // When: 用户查询当前结果。
    await wrapper.get('[data-testid="retry-public-search"]').trigger("click")
    await flushPromises()

    // Then: 通过 GET 收敛终态，不重放取消或创建。
    expect(current.refresh).toHaveBeenCalledOnce()
    expect(current.retryPending).not.toHaveBeenCalled()
    expect(current.mutate).not.toHaveBeenCalled()
    expect(mocks.create).toHaveBeenCalledOnce()
    expect(current.state.pendingOperation).toBeNull()
  })

  it("allows an existing run to reconnect after a restored cooldown", async () => {
    // Given: 恢复时仍在冷却期，尚未获取首份快照，也没有新错误。
    const current = lifecycle("first")
    current.state.run = null
    current.state.now = 1000
    current.state.waitUntil = 2000
    mocks.create.mockReturnValue(current)
    vi.mocked(current.refresh).mockImplementation(async () => {
      const result = completed("first", [1])
      current.state.run = result
      return result
    })
    const wrapper = await mountResult()
    expect(wrapper.get('[data-testid="retry-public-search"]').attributes("disabled")).toBeDefined()

    // When: 当前时间达到已有冷却期限，用户手动恢复。
    current.state.now = 2000
    await flushPromises()
    expect(wrapper.get('[data-testid="retry-public-search"]').attributes("disabled")).toBeUndefined()
    await wrapper.get('[data-testid="retry-public-search"]').trigger("click")
    await flushPromises()

    // Then: 读取原 Run 并恢复结果，不创建替代任务。
    expect(current.refresh).toHaveBeenCalledOnce()
    expect(mocks.create).toHaveBeenCalledOnce()
    expect(wrapper.findAll(".article-list article")).toHaveLength(1)
  })

  it("shows an inline empty result when cited articles are no longer public", async () => {
    // Given: 引用文章已被公开接口移除。
    const current = lifecycle("first")
    current.state.run = completed("first", [1])
    mocks.create.mockReturnValue(current)
    mocks.detail.mockRejectedValue(new HttpRequestError("gone", 404, "NOT_FOUND", undefined))

    // When: 读取完成结果。
    const wrapper = await mountResult()

    // Then: 不伪造卡片，不展示大空面板，也不把 404 当作可重试的系统失败。
    expect(wrapper.findAll(".article-list article")).toHaveLength(0)
    expect(wrapper.find(".blog-card").exists()).toBe(false)
    expect(wrapper.find('[data-testid="retry-search-articles"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="ai-search-status"]').isVisible()).toBe(true)
  })

  it.each(["prop", "identity"])("rejects late article reads after the %s changes", async (change) => {
    // Given: 第一份结果的文章请求仍在途。
    const first = lifecycle("first")
    const second = lifecycle("second")
    first.state.run = completed("first", [1])
    second.state.run = completed("second", [2])
    const read = Promise.withResolvers<ArticleSummary>()
    mocks.detail.mockImplementation(id => id === 1 ? read.promise : Promise.resolve(article(id)))
    mocks.create.mockReturnValueOnce(first).mockReturnValueOnce(second)
    const wrapper = await mountResult()
    expect(mocks.detail).toHaveBeenCalledWith(1)

    // When: 切换 Run 或失效身份后，旧请求才完成。
    if (change === "prop") await wrapper.setProps({ runId: "second" })
    else mocks.invalidate?.()
    await flushPromises()
    read.resolve(article(1))
    await flushPromises()

    // Then: 迟到文章不能覆盖新结果或重新暴露失效身份的数据。
    expect(wrapper.getComponent(ArticleSection).props("articles").map((item: ArticleSummary) => item.id)).toEqual(change === "prop" ? [2] : [])
  })
})
