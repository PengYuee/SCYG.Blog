const AUTHOR_PREFIX = "scyg.ai.author."
const listeners = new Set<() => void>()
let memorySubject: { token: string; subject: string } | undefined
let subjectStorageValid = true
export function onAuthorSessionChange(listener: () => void): () => void {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}
export function clearAuthorAiState(): void {
  memorySubject = undefined
  subjectStorageValid = false
  for (const listener of listeners) listener()
  try {
    for (let index = sessionStorage.length - 1; index >= 0; index -= 1) {
      const key = sessionStorage.key(index)
      if (key?.startsWith(AUTHOR_PREFIX)) sessionStorage.removeItem(key)
    }
  } catch { /* Clearing browser storage is best effort; listeners already invalidated memory. */ }
}
/** Subject keys are random browser identities, never decoded JWT claims. */
export function authorTabSubject(token: string): string {
  if (memorySubject?.token === token) return memorySubject.subject
  const key = `${AUTHOR_PREFIX}subject`
  try {
    const stored = subjectStorageValid ? sessionStorage.getItem(key) : null
    if (stored !== null) {
      const value = JSON.parse(stored) as { token?: unknown; subject?: unknown }
      if (value.token === token && typeof value.subject === "string") {
        memorySubject = { token, subject: value.subject }
        return value.subject
      }
    }
  } catch { /* Restricted or invalid storage cannot supply a persisted identity. */ }
  const subject = crypto.randomUUID()
  memorySubject = { token, subject }
  try { sessionStorage.setItem(key, JSON.stringify(memorySubject)); subjectStorageValid = true } catch { /* The real in-memory identity remains stable across mounts. */ }
  return subject
}
export function readTabState<T>(key: string, fallback: T): T {
  try { const value = sessionStorage.getItem(key); return value === null ? fallback : JSON.parse(value) as T } catch { return fallback }
}
export function saveTabState(key: string, value: unknown): void {
  try { sessionStorage.setItem(key, JSON.stringify(value)) } catch { /* Storage restrictions do not stop the in-memory operation. */ }
}
