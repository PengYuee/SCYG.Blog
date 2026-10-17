import { flushPromises, mount } from "@vue/test-utils"
import { reactive } from "vue"
import { createMemoryHistory, createRouter } from "vue-router"
import { afterEach, describe, expect, it, vi } from "vitest"
import type { AgentRun } from "@/request/generated"
import type { AiRunController, AiRunState } from "@/services/ai-run"
import type * as AiRunModule from "@/services/ai-run"
import AiRunView from "@/components/ai/AiRunView.vue"

const mocks = vi.hoisted(() => ({ create: vi.fn(), invalidate: undefined as (() => void) | undefined }))
vi.mock("@/request/api-services", () => ({ useApiServices: () => ({ ai: {} }) }))
vi.mock("@/services/auth-session", () => ({ useAuthSession: () => ({ accessToken: () => "token" }) }))
vi.mock("@/services/ai-session-state", () => ({ authorTabSubject: () => "subject", onAuthorSessionChange: (callback: () => void) => { mocks.invalidate = callback; return () => undefined } }))
vi.mock("@/services/ai-run", async (importOriginal) => ({ ...await importOriginal<typeof AiRunModule>(), createAiRun: mocks.create }))
function lifecycle(runId: string): AiRunController {
  const state = reactive<AiRunState>({ run: { runId, status: "running", capability: "write", recipeId: "writing-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: `/api/v1/runs/${runId}/events`, pendingInteraction: null, failure: null, result: null }, error: null, connection: "已连接", busy: false, waitUntil: 0, now: Date.now(), activities: [], original: "", interactionDrafts: {}, pendingOperation: null })
  return { state, connect: vi.fn(async () => undefined), refresh: vi.fn(async () => undefined), stop: vi.fn(), mutate: vi.fn(async () => undefined), retryPending: vi.fn(async () => undefined), persist: vi.fn() }
}
afterEach(() => { mocks.create.mockReset(); mocks.invalidate = undefined })
describe("AI run view lifecycle ownership", () => {
  it.each(["route", "logout"])("late manual refresh rejection after %s cannot write into current view", async (change) => {
    const first = lifecycle("first")
    const second = lifecycle("second")
    const { promise, reject } = Promise.withResolvers<AgentRun | undefined>()
    vi.mocked(first.refresh).mockReturnValue(promise)
    mocks.create.mockReturnValueOnce(first).mockReturnValueOnce(second)
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: "/author/ai/runs/:runId", component: { template: "<div />" } }] })
    await router.push("/author/ai/runs/first")
    const wrapper = mount(AiRunView, { props: { anonymous: false }, global: { plugins: [router], stubs: { AiResult: true, AiInteraction: true } } })
    await flushPromises()
    await wrapper.findAll("button").find((button) => button.text() === "查询最新状态")?.trigger("click")
    expect(first.refresh).toHaveBeenCalledOnce()
    if (change === "route") await router.push("/author/ai/runs/second")
    else mocks.invalidate?.()
    await flushPromises()
    reject(new Error("旧任务延迟失败"))
    await flushPromises()
    expect(wrapper.text()).not.toContain("旧任务延迟失败")
    expect(first.state.error).toBeNull()
    expect(second.state.error).toBeNull()
    if (change === "route") expect(wrapper.text()).toContain("second")
    else expect(wrapper.text()).toContain("作者会话已改变")
    wrapper.unmount()
  })
})
