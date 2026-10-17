import { reactive } from "vue"
import { z } from "zod"
import { zAgentResumeRequest } from "@/request/generated/zod.gen"
import type { AgentRun, AgentResumeRequest } from "@/request/generated"
import type { AiApi, AiAuth } from "@/request/api/ai"
import { HttpRequestError } from "@/request/http-error"
import { readTabState, saveTabState } from "@/services/ai-session-state"
export const RUN_STATUS_LABELS: Record<AgentRun["status"], string> = { queued: "排队中", running: "运行中", waitingForApproval: "等待你的回应", succeeded: "已完成", failed: "运行失败", cancelled: "已取消" }
export function isTerminalRun(run: AgentRun): boolean { return run.status === "succeeded" || run.status === "failed" || run.status === "cancelled" }
type PendingOperation = { kind: "resume" | "cancel"; key: string; body?: AgentResumeRequest; createdAt: number }
const runStorageModel = z.object({
  cursor: z.string().refine((value) => !/[\r\n]/.test(value)).optional(),
  pending: z.object({ kind: z.enum(["resume", "cancel"]), key: z.uuid({ version: "v4" }), body: zAgentResumeRequest.optional(), createdAt: z.number().finite().nonnegative() }).optional(),
  interactionDrafts: z.record(z.string(), z.unknown()).default({}),
  original: z.string().optional(),
  waitUntil: z.number().finite().nonnegative().optional(),
}).catch({ interactionDrafts: {} })
export interface AiRunState { run: AgentRun | null; error: unknown; connection: string; busy: boolean; waitUntil: number; now: number; activities: string[]; original: string; interactionDrafts: Record<string, unknown>; pendingOperation: "resume" | "cancel" | null }
export interface AiRunController {
  state: AiRunState
  connect(resetCursor?: boolean): Promise<void>
  refresh(): Promise<AgentRun | undefined>
  stop(): void
  mutate(kind: "resume" | "cancel", body?: AgentResumeRequest): Promise<void>
  retryPending(): Promise<void>
  persist(): void
}
export function createAiRun(api: AiApi, auth: AiAuth, runId: string, storageKey: string): AiRunController {
  const saved = runStorageModel.parse(readTabState<unknown>(storageKey, {}))
  const state = reactive<AiRunState>({ run: null, error: null, connection: "未连接", busy: false, waitUntil: saved.waitUntil ?? 0, now: Date.now(), activities: [], original: saved.original ?? "", interactionDrafts: saved.interactionDrafts ?? {}, pendingOperation: saved.pending?.kind ?? null })
  let cursor = typeof saved.cursor === "string" ? saved.cursor : undefined
  let pending = saved.pending
  let generation = 0
  let controller: AbortController | undefined
  let snapshot: Promise<AgentRun | undefined> | undefined
  let disposed = false
  let snapshotRateLimited = false
  const seen = new Set<string>()
  let timer: number | undefined
  function persist(): void {
    if (disposed) return
    state.pendingOperation = pending?.kind ?? null
    saveTabState(storageKey, { cursor, pending, interactionDrafts: state.interactionDrafts, original: state.original, waitUntil: state.waitUntil })
  }
  function errorState(error: unknown): void {
    state.error = error
    if (error instanceof HttpRequestError && error.status === 429) {
      state.waitUntil = Date.now() + (error.retryAfterSeconds ?? 0) * 1000
      snapshotRateLimited = true
      state.connection = "连接已停止，请手动重连"
    }
    persist()
  }
  async function refresh(expected = generation): Promise<AgentRun | undefined> {
    if (disposed || expected !== generation || Date.now() < state.waitUntil) return undefined
    if (snapshot !== undefined) return snapshot
    if (controller === undefined || controller.signal.aborted) controller = new AbortController()
    const signal = controller.signal
    const work = api.get(runId, auth, signal).then((run) => {
      if (disposed || generation !== expected || signal?.aborted) return undefined
      state.run = run
      if (pending !== undefined && (isTerminalRun(run) || (pending.kind === "resume" && pending.body?.interactionId !== run.pendingInteraction?.interactionId))) {
        pending = undefined
        persist()
      }
      return run
    }).catch((error: unknown) => {
      if (disposed || generation !== expected || signal?.aborted) return undefined
      errorState(error)
      if (error instanceof HttpRequestError && error.status === 429) controller?.abort()
      throw error
    }).finally(() => { if (snapshot === work) snapshot = undefined })
    snapshot = work
    return work
  }
  async function delay(milliseconds: number, signal: AbortSignal): Promise<void> {
    await new Promise<void>((resolve) => {
      const timeout = setTimeout(done, milliseconds)
      function done(): void { clearTimeout(timeout); signal.removeEventListener("abort", done); resolve() }
      signal.addEventListener("abort", done, { once: true })
      if (signal.aborted) done()
    })
  }
  async function connect(resetCursor = false): Promise<void> {
    if (disposed || state.busy || Date.now() < state.waitUntil) return
    snapshotRateLimited = false
    stop()
    const own = generation
    controller = new AbortController()
    const signal = controller.signal
    if (resetCursor) { cursor = undefined; persist(); state.activities.push("重新连接未携带游标，活动记录可能不完整") }
    state.error = null
    let retries = 0
    let notificationPending = false
    let notificationActive = false
    async function updateFromNotification(): Promise<void> {
      notificationPending = true
      if (notificationActive) return
      notificationActive = true
      try {
        while (notificationPending && !signal.aborted && generation === own && !snapshotRateLimited) {
          notificationPending = false
          const updated = await refresh(own)
          if (updated !== undefined && isTerminalRun(updated)) { state.connection = "任务已结束"; controller?.abort(); return }
        }
      } catch { /* Snapshot failures are guarded and recorded by refresh. */ }
      finally { notificationActive = false }
    }
    while (!signal.aborted && generation === own) {
      try {
        state.connection = "查询最新状态"
        const run = await refresh(own)
        if (run === undefined || signal.aborted) return
        if (isTerminalRun(run)) { state.connection = "任务已结束"; return }
        state.connection = "已连接"
        await api.events(run, auth, signal, cursor, (frame) => {
          if (generation !== own || signal.aborted) return
          let event: unknown
          try { event = JSON.parse(frame.data) } catch { state.activities.push("收到无法解析的活动通知"); return }
          if (typeof event !== "object" || event === null || !("event_id" in event) || typeof event.event_id !== "string" || !("run_id" in event) || event.run_id !== runId) return
          if (frame.cursor !== undefined) { cursor = frame.cursor; persist() }
          if (seen.has(event.event_id)) return
          seen.add(event.event_id)
          if ("kind" in event && typeof event.kind === "string") state.activities.push(event.kind)
          if (state.activities.length > 100) state.activities.shift()
          void updateFromNotification()
        })
        if (signal.aborted) return
        const latest = await refresh(own)
        if (latest === undefined || isTerminalRun(latest)) { state.connection = "任务已结束"; return }
        throw new HttpRequestError("事件连接已中断", undefined, "STREAM_EOF", undefined)
      } catch (error) {
        if (signal.aborted || generation !== own) return
        errorState(error)
        let recoveryError: unknown
        if (!snapshotRateLimited) {
          try {
            const latest = await refresh(own)
            if (latest !== undefined && isTerminalRun(latest)) { state.connection = "任务已结束"; return }
          } catch (snapshotError) { recoveryError = snapshotError }
        }
        if (signal.aborted || generation !== own) return
        const retryable = (failure: unknown): boolean => !(failure instanceof HttpRequestError) || failure.status === undefined || failure.status === 503
        if (snapshotRateLimited || !retryable(error) || !retryable(recoveryError) || retries >= 3) { state.connection = "连接已停止，请手动重连"; return }
        const wait = Math.max(1000 * 2 ** retries, error instanceof HttpRequestError ? (error.retryAfterSeconds ?? 0) * 1000 : 0, recoveryError instanceof HttpRequestError ? (recoveryError.retryAfterSeconds ?? 0) * 1000 : 0)
        retries += 1
        state.connection = `连接中断，等待第 ${retries}/3 次恢复`
        await delay(wait, signal)
      }
    }
  }
  function stop(): void {
    generation += 1
    controller?.abort()
    controller = undefined
    snapshot = undefined
  }
  async function mutate(kind: PendingOperation["kind"], body?: AgentResumeRequest): Promise<void> {
    if (disposed || state.busy || Date.now() < state.waitUntil || (state.run !== null && isTerminalRun(state.run))) return
    if (pending !== undefined && (pending.kind !== kind || JSON.stringify(pending.body) !== JSON.stringify(body))) {
      state.error = new Error("上一操作结果尚未确认，请先重试原操作或查询最新状态，不要提交不同决定。")
      return
    }
    if (pending === undefined) pending = { kind, key: crypto.randomUUID(), ...(body === undefined ? {} : { body: structuredClone(body) }), createdAt: Date.now() }
    if (Date.now() - pending.createdAt >= 24 * 60 * 60 * 1000) { state.error = new Error("操作已超过幂等窗口，请查询状态，不再自动重放。 "); return }
    persist()
    stop()
    controller = new AbortController()
    state.busy = true
    state.error = null
    const own = generation
    const operation = pending
    try {
      const run = kind === "cancel" ? await api.cancel(runId, operation.key, auth, controller?.signal) : await api.resume(runId, operation.body!, operation.key, controller?.signal)
      if (generation !== own) return
      state.run = run
      pending = undefined
      persist()
    } catch (error) {
      if (generation !== own) return
      errorState(error)
      if (error instanceof HttpRequestError && error.status !== undefined && error.status < 500 && error.status !== 429) {
        pending = undefined
        persist()
        if (error.status === 409) await refresh(own).catch(() => { /* refresh owns guarded error state. */ })
      }
    } finally { if (generation === own) state.busy = false }
    if (generation === own && state.error === null) await connect()
  }
  function dispose(): void { disposed = true; stop(); clearInterval(timer); state.busy = false }
  timer = window.setInterval(() => { state.now = Date.now() }, 250)
  return { state, connect, refresh, stop: dispose, mutate, persist, async retryPending() { if (pending !== undefined) await mutate(pending.kind, pending.body) } }
}
