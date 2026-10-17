import { computed, onBeforeUnmount, reactive, ref, watch } from "vue"
import { z } from "zod"
import { useApiServices } from "@/request/api-services"
import { HttpRequestError } from "@/request/http-error"
import { ensureAnonymousSession, onAnonymousSessionChange, restoreAnonymousSession } from "@/services/anonymous-ai-session"
import { readTabState, saveTabState } from "@/services/ai-session-state"

const workspaceModel = z.object({
  form: z.object({ query: z.string(), maxResults: z.number().int() }).optional(),
  lastRun: z.string().regex(/^[A-Za-z0-9._~-]{1,128}$/).optional(),
  waitUntil: z.number().finite().nonnegative().optional(),
  pending: z.object({
    key: z.uuid(),
    input: z.object({ query: z.string(), max_results: z.number().int() }),
    capability: z.literal("search"),
    createdAt: z.number().finite().nonnegative(),
  }).optional(),
})

export function usePublicAiSearch() {
  const api = useApiServices().ai
  const session = restoreAnonymousSession()
  let storageKey = `scyg.ai.anonymous.${session?.accessToken ?? "new"}.workspace`
  const decoded = workspaceModel.safeParse(readTabState<unknown>(storageKey, {}))
  const stored = decoded.success ? decoded.data : {}
  const limits = reactive({ queryMaxChars: session?.queryMaxChars ?? 2000, maxResults: session?.maxResults ?? 5 })
  const query = ref(stored.form?.query ?? "")
  const maxResults = ref(stored.form?.maxResults ?? 5)
  const pending = ref(stored.pending)
  const lastRun = ref<string | undefined>(stored.lastRun)
  const busy = ref(false)
  const error = ref<unknown>(null)
  const invalidated = ref(false)
  const waitUntil = ref(stored.waitUntil ?? 0)
  const now = ref(Date.now())
  let controller = new AbortController()
  let generation = 0
  const timer = window.setInterval(() => { now.value = Date.now() }, 250)
  const waitSeconds = computed(() => Math.max(0, Math.ceil((waitUntil.value - now.value) / 1000)))
  const pendingExpired = computed(() => pending.value !== undefined && now.value - pending.value.createdAt >= 86400000)
  const valid = computed(() => query.value.trim().length > 0 && query.value.length <= limits.queryMaxChars && Number.isInteger(maxResults.value) && maxResults.value >= 1 && maxResults.value <= limits.maxResults)
  const disabled = computed(() => busy.value || invalidated.value || controller.signal.aborted || now.value < waitUntil.value)
  const canSubmit = computed(() => !disabled.value && !pendingExpired.value && (pending.value !== undefined || valid.value))

  function persist(): void {
    if (!invalidated.value) saveTabState(storageKey, {
      form: { query: query.value, maxResults: maxResults.value },
      pending: pending.value, lastRun: lastRun.value, waitUntil: waitUntil.value,
    })
  }
  watch([query, maxResults], persist, { flush: "sync" })
  const unsubscribe = onAnonymousSessionChange(() => {
    generation += 1
    controller.abort()
    invalidated.value = true
    pending.value = undefined
    lastRun.value = undefined
    waitUntil.value = 0
    query.value = ""
    maxResults.value = 5
    busy.value = false
    error.value = new Error("匿名会话已失效，旧任务不能使用新凭证恢复。")
  })
  onBeforeUnmount(() => { generation += 1; controller.abort(); clearInterval(timer); unsubscribe() })

  function abandonExpiredCreate(): void {
    if (disabled.value || !pendingExpired.value) return
    pending.value = undefined
    error.value = null
    persist()
  }
  function restart(): void {
    if (!invalidated.value) return
    generation += 1
    controller = new AbortController()
    storageKey = "scyg.ai.anonymous.new.workspace"
    pending.value = undefined
    lastRun.value = undefined
    waitUntil.value = 0
    error.value = null
    invalidated.value = false
    query.value = ""
    maxResults.value = Math.min(5, limits.maxResults)
    persist()
  }
  async function submit(): Promise<string | undefined> {
    if (!canSubmit.value) return undefined
    busy.value = true
    error.value = null
    const own = generation
    try {
      const identity = await ensureAnonymousSession(api, pending.value === undefined, controller.signal)
      if (own !== generation) return undefined
      limits.queryMaxChars = identity.queryMaxChars
      limits.maxResults = identity.maxResults
      storageKey = `scyg.ai.anonymous.${identity.accessToken}.workspace`
      if (!valid.value && pending.value === undefined) throw new Error("输入超过当前公共搜索限制，请修正后提交。")
      if (pending.value === undefined) {
        pending.value = { key: crypto.randomUUID(), input: { query: query.value, max_results: maxResults.value }, capability: "search", createdAt: Date.now() }
        persist()
      }
      if (Date.now() - pending.value.createdAt >= 86400000) throw new Error("未知操作超过幂等窗口，不能自动重放；旧操作可能已执行，请查询已有 Run，或明确确认放弃后再开始新操作。")
      const operation = pending.value
      const run = await api.create("search", operation.input, operation.key, "anonymous", controller.signal)
      if (own !== generation) return undefined
      saveTabState(`scyg.ai.anonymous.${identity.accessToken}.run.${run.runId}`, { interactionDrafts: {} })
      saveTabState(`scyg.ai.anonymous.${identity.accessToken}.run.${run.runId}.query`, operation.input.query)
      lastRun.value = run.runId
      pending.value = undefined
      persist()
      return run.runId
    } catch (failure) {
      if (own !== generation) return undefined
      error.value = failure
      if (failure instanceof HttpRequestError) {
        if (failure.status === 429 && failure.retryAfterSeconds !== undefined) waitUntil.value = Date.now() + failure.retryAfterSeconds * 1000
        if (failure.status !== undefined && failure.status < 500 && failure.status !== 429) pending.value = undefined
      }
      persist()
      return undefined
    } finally { if (own === generation) busy.value = false }
  }
  return { query, maxResults, limits, busy, error, waitSeconds, canSubmit, pending, pendingExpired, invalidated, lastRun, submit, restart, abandonExpiredCreate }
}
