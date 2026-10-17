import type { PublicSearchSession } from "@/request/generated"
import { zPublicSearchSession } from "@/request/generated/zod.gen"
import type { AiApi } from "@/request/api/ai"
import { configureRequestAuth } from "@/request/auth-policy"
import { HttpRequestError } from "@/request/http-error"
const STORAGE_KEY = "scyg.ai.anonymous.session"
let current: PublicSearchSession | undefined
let pending: Promise<PublicSearchSession> | undefined
let generation = 0
let storageValid = true
const listeners = new Set<() => void>()
export function onAnonymousSessionChange(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
export function invalidateAnonymousSession(): void {
  generation += 1
  current = undefined
  pending = undefined
  storageValid = false
  try { sessionStorage.removeItem(STORAGE_KEY) } catch { /* An unavailable storage cannot retain an active in-memory session. */ }
  for (const listener of listeners) listener()
}
function token(): string | undefined {
  if (current !== undefined && Date.parse(current.expiresAt) <= Date.now()) invalidateAnonymousSession()
  return current?.accessToken
}
export function restoreAnonymousSession(): PublicSearchSession | undefined {
  configureRequestAuth("anonymous", token, invalidateAnonymousSession)
  if (current !== undefined) { token(); return current }
  try {
    const raw = storageValid ? sessionStorage.getItem(STORAGE_KEY) : null
    if (raw !== null) {
      const parsed = zPublicSearchSession.safeParse(JSON.parse(raw))
      if (parsed.success && Date.parse(parsed.data.expiresAt) > Date.now()) current = parsed.data
      else invalidateAnonymousSession()
    }
  } catch { invalidateAnonymousSession() }
  return current
}
export async function ensureAnonymousSession(api: AiApi, allowNew: boolean, signal?: AbortSignal): Promise<PublicSearchSession> {
  const restored = restoreAnonymousSession()
  if (restored !== undefined) return restored
  if (!allowNew) throw new HttpRequestError("匿名凭证已丢失或过期，无法恢复原任务。请返回搜索页明确开始新的会话。", 401, "ANONYMOUS_SESSION_EXPIRED", undefined)
  if (pending !== undefined) return pending
  const own = generation
  pending = api.session(signal).then((session) => {
    if (own !== generation || signal?.aborted) throw new HttpRequestError("匿名会话已失效，请明确开始新的会话。", 401, "ANONYMOUS_SESSION_EXPIRED", undefined)
    current = session
    storageValid = false
    try { sessionStorage.setItem(STORAGE_KEY, JSON.stringify(session)); storageValid = true } catch { /* The returned credential is usable only in memory, not refresh-recoverable. */ }
    return session
  }).finally(() => { if (own === generation) pending = undefined })
  return pending
}
