import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import type { AgentRun } from "@/request/generated"
import type { AiApi } from "@/request/api/ai"
import { createAiRun, type AiRunController } from "@/services/ai-run"
import { HttpRequestError } from "@/request/http-error"
const run: AgentRun = { runId: "run-safe", status: "running", capability: "write", recipeId: "writing-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: "/api/v1/runs/run-safe/events", pendingInteraction: null, failure: null, result: null }
let controller: AiRunController | undefined
const storage = new Map<string, string>()
function api(): AiApi {
  return { session: vi.fn(), create: vi.fn(), get: vi.fn(async () => run), resume: vi.fn(), cancel: vi.fn(), events: vi.fn(async (_run, _auth, signal) => { await new Promise<void>((resolve) => signal.addEventListener("abort", () => resolve(), { once: true })) }) }
}
beforeEach(() => {
  storage.clear()
  vi.useFakeTimers()
  vi.stubGlobal("window", { setInterval })
  vi.stubGlobal("sessionStorage", { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) })
})
afterEach(() => { controller?.stop(); controller = undefined; vi.useRealTimers(); vi.unstubAllGlobals() })
describe("AI authoritative lifecycle", () => {
  it("deep link reads before subscribing; leaving never sends cancel", async () => {
    const service = api()
    controller = createAiRun(service, "author", run.runId, "test")
    const connection = controller.connect()
    await vi.advanceTimersByTimeAsync(0)
    expect(service.get).toHaveBeenCalledOnce()
    expect(service.events).toHaveBeenCalledOnce()
    controller.stop()
    await connection
    expect(service.cancel).not.toHaveBeenCalled()
  })
  it("EOF queries snapshot and does not invent success", async () => {
    const service = api()
    vi.mocked(service.events).mockResolvedValue(undefined)
    vi.mocked(service.get).mockResolvedValueOnce(run).mockResolvedValueOnce({ ...run, status: "failed", failure: { code: "model_error", message: "生成失败" }, result: { title: "partial" } })
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.connect()
    expect(controller.state.run).toMatchObject({ status: "failed", result: { title: "partial" }, failure: { code: "model_error" } })
    expect(service.events).toHaveBeenCalledOnce()
  })
  it("503 recovery respects header and is bounded to three retries", async () => {
    const service = api()
    vi.mocked(service.events).mockRejectedValue(new HttpRequestError("busy", 503, "busy", undefined, undefined, 7))
    controller = createAiRun(service, "author", run.runId, "test")
    const connection = controller.connect()
    await vi.advanceTimersByTimeAsync(0)
    expect(service.events).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(6999)
    expect(service.events).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(14001)
    await connection
    expect(service.events).toHaveBeenCalledTimes(4)
  })
  it("429 waits and never automatically reconnects at expiry", async () => {
    const service = api()
    vi.mocked(service.events).mockRejectedValue(new HttpRequestError("busy", 429, "busy", undefined, undefined, 2))
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.connect()
    await controller.connect()
    expect(service.events).toHaveBeenCalledOnce()
    expect(service.get).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(3000)
    expect(service.events).toHaveBeenCalledOnce()
  })
  it("uncertain resume persists key and exact body across refresh", async () => {
    const service = api()
    vi.mocked(service.resume).mockRejectedValue(new HttpRequestError("network", undefined, "NETWORK", undefined))
    controller = createAiRun(service, "author", run.runId, "test")
    const body = { interactionId: "text-stage", decision: "submit", payload: "original requirements" }
    await controller.mutate("resume", body)
    controller.stop()
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.mutate("resume", body)
    const calls = vi.mocked(service.resume).mock.calls
    expect(calls[0]?.[2]).toBe(calls[1]?.[2])
    expect(calls[1]?.[1]).toEqual(body)
    await controller.mutate("resume", { ...body, payload: "different" })
    expect(service.resume).toHaveBeenCalledTimes(2)
  })
  it("400 permits corrected input with a new logical key", async () => {
    const service = api()
    vi.mocked(service.resume).mockRejectedValue(new HttpRequestError("invalid", 400, "INVALID", undefined))
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.mutate("resume", { interactionId: "stage", decision: "submit", payload: "bad" })
    await controller.mutate("resume", { interactionId: "stage", decision: "submit", payload: "corrected" })
    const calls = vi.mocked(service.resume).mock.calls
    expect(calls[0]?.[2]).not.toBe(calls[1]?.[2])
  })
  it("late snapshot is ignored after local disposal", async () => {
    const service = api()
    let finish!: (value: AgentRun) => void
    vi.mocked(service.get).mockImplementation(() => new Promise((resolve) => { finish = resolve }))
    controller = createAiRun(service, "author", run.runId, "test")
    const connection = controller.connect()
    controller.stop()
    finish(run)
    await connection
    expect(controller.state.run).toBeNull()
    expect(service.events).not.toHaveBeenCalled()
  })
  it.each([0, 2])("snapshot 429 (%s seconds) stops notification queries and requires manual recovery", async (seconds) => {
    const service = api()
    let notify!: Parameters<AiApi["events"]>[4]
    vi.mocked(service.events).mockImplementation(async (_run, _auth, signal, _cursor, callback) => {
      notify = callback
      const { promise, resolve } = Promise.withResolvers<void>()
      signal.addEventListener("abort", () => resolve(), { once: true })
      await promise
    })
    vi.mocked(service.get).mockResolvedValueOnce(run).mockRejectedValueOnce(new HttpRequestError("limited", 429, "LIMITED", undefined, undefined, seconds)).mockResolvedValue(run)
    controller = createAiRun(service, "author", run.runId, "test")
    const connection = controller.connect()
    await vi.advanceTimersByTimeAsync(0)
    notify({ event: "message", data: JSON.stringify({ event_id: "first", run_id: run.runId, kind: "changed" }) })
    await vi.advanceTimersByTimeAsync(0)
    await connection
    expect(controller.state.waitUntil).toBe(Date.now() + seconds * 1000)
    notify({ event: "message", data: JSON.stringify({ event_id: "second", run_id: run.runId, kind: "changed" }) })
    if (seconds > 0) {
      await controller.refresh()
      await controller.connect()
      expect(service.get).toHaveBeenCalledTimes(2)
    }
    await vi.advanceTimersByTimeAsync(3000)
    expect(service.get).toHaveBeenCalledTimes(2)
    expect(service.events).toHaveBeenCalledOnce()
    await controller.refresh()
    expect(service.get).toHaveBeenCalledTimes(3)
    expect(service.events).toHaveBeenCalledOnce()
  })
  it("manual snapshot 429 prevents reads until its deadline, without scheduling recovery", async () => {
    const service = api()
    vi.mocked(service.get).mockRejectedValueOnce(new HttpRequestError("limited", 429, "LIMITED", undefined, undefined, 2)).mockResolvedValue(run)
    controller = createAiRun(service, "author", run.runId, "test")
    await expect(controller.refresh()).rejects.toMatchObject({ status: 429 })
    await controller.refresh()
    await controller.connect()
    expect(service.get).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(3000)
    expect(service.get).toHaveBeenCalledOnce()
    await controller.refresh()
    expect(service.get).toHaveBeenCalledTimes(2)
    expect(service.events).not.toHaveBeenCalled()
  })
  it.each([400, 401, 404, 409].flatMap((status) => [503, undefined].map((recoveryStatus) => ({ status, recoveryStatus }))))("original SSE $status cannot become retryable after recovery $recoveryStatus", async ({ status, recoveryStatus }) => {
    const service = api()
    vi.mocked(service.events).mockRejectedValue(new HttpRequestError("original", status, "ORIGINAL", undefined))
    vi.mocked(service.get).mockResolvedValueOnce(run).mockRejectedValue(new HttpRequestError("recovery", recoveryStatus, "BUSY", undefined))
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.connect()
    await vi.advanceTimersByTimeAsync(60000)
    expect(service.get).toHaveBeenCalledTimes(2)
    expect(service.events).toHaveBeenCalledOnce()
    expect(controller.state.connection).toContain("手动重连")
  })
  it("nonretryable SSE still accepts an authoritative terminal snapshot", async () => {
    const service = api()
    vi.mocked(service.events).mockRejectedValue(new HttpRequestError("gone", 404, "GONE", undefined))
    vi.mocked(service.get).mockResolvedValueOnce(run).mockResolvedValueOnce({ ...run, status: "succeeded" })
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.connect()
    expect(controller.state.run?.status).toBe("succeeded")
    expect(controller.state.connection).toBe("任务已结束")
  })
  it.each(["refresh", "conflict"])("late %s rejection cannot recreate cleared account storage", async (mode) => {
    const service = api()
    const { promise, reject } = Promise.withResolvers<AgentRun>()
    vi.mocked(service.get).mockReturnValue(promise)
    vi.mocked(service.resume).mockRejectedValue(new HttpRequestError("conflict", 409, "CONFLICT", undefined))
    controller = createAiRun(service, "author", run.runId, "test")
    const work = mode === "refresh" ? controller.refresh() : controller.mutate("resume", { interactionId: "old", decision: "approve" })
    await vi.advanceTimersByTimeAsync(0)
    controller.stop()
    storage.clear()
    const previousError = controller.state.error
    reject(new HttpRequestError("late failure", 503, "BUSY", undefined))
    await work
    expect(storage.size).toBe(0)
    expect(controller.state.error).toBe(previousError)
  })
  it.each(["new", "consumed", "terminal"])("authoritative %s snapshot clears expired resume without replaying it", async (stage) => {
    const service = api()
    const oldBody = { interactionId: "old", decision: "approve" }
    const oldKey = crypto.randomUUID()
    storage.set("test", JSON.stringify({ pending: { kind: "resume", key: oldKey, body: oldBody, createdAt: Date.now() - 25 * 60 * 60 * 1000 } }))
    vi.mocked(service.get).mockResolvedValue({ ...run, status: stage === "terminal" ? "succeeded" : "running", pendingInteraction: stage === "new" ? { interactionId: "new", type: "confirmation", payload: null } : null })
    vi.mocked(service.resume).mockRejectedValue(new HttpRequestError("uncertain", undefined, "NETWORK", undefined))
    vi.mocked(service.cancel).mockRejectedValue(new HttpRequestError("uncertain", undefined, "NETWORK", undefined))
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.refresh()
    expect(controller.state.pendingOperation).toBeNull()
    await controller.retryPending()
    expect(service.resume).not.toHaveBeenCalled()
    if (stage === "new") {
      await controller.mutate("resume", { interactionId: "new", decision: "approve" })
      expect(service.resume).toHaveBeenCalledOnce()
      expect(vi.mocked(service.resume).mock.calls[0]?.[2]).not.toBe(oldKey)
    } else if (stage === "consumed") {
      await controller.mutate("cancel")
      expect(service.cancel).toHaveBeenCalledOnce()
    }
  })
  it("unchanged interaction retains an expired uncertain decision", async () => {
    const service = api()
    storage.set("test", JSON.stringify({ pending: { kind: "resume", key: crypto.randomUUID(), body: { interactionId: "old", decision: "approve" }, createdAt: Date.now() - 25 * 60 * 60 * 1000 } }))
    vi.mocked(service.get).mockResolvedValue({ ...run, pendingInteraction: { interactionId: "old", type: "confirmation", payload: null } })
    controller = createAiRun(service, "author", run.runId, "test")
    await controller.refresh()
    await controller.retryPending()
    await controller.mutate("cancel")
    expect(controller.state.pendingOperation).toBe("resume")
    expect(service.resume).not.toHaveBeenCalled()
    expect(service.cancel).not.toHaveBeenCalled()
  })
  it.each([false, true])("manual snapshot owns an abortable request after prior abort=%s", async (priorAbort) => {
    const service = api()
    controller = createAiRun(service, "author", run.runId, "test")
    if (priorAbort) {
      vi.mocked(service.get).mockRejectedValueOnce(new HttpRequestError("limited", 429, "LIMITED", undefined, undefined, 0))
      await expect(controller.refresh()).rejects.toMatchObject({ status: 429 })
    }
    const { promise, resolve } = Promise.withResolvers<AgentRun>()
    let requestSignal: AbortSignal | undefined
    vi.mocked(service.get).mockImplementation((_runId, _auth, signal) => { requestSignal = signal; return promise })
    const query = controller.refresh()
    await vi.advanceTimersByTimeAsync(0)
    expect(requestSignal).toBeDefined()
    expect(requestSignal?.aborted).toBe(false)
    controller.stop()
    expect(requestSignal?.aborted).toBe(true)
    resolve(run)
    expect(await query).toBeUndefined()
    expect(controller.state.run).toBeNull()
  })
})
