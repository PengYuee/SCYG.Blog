import { createPinia, setActivePinia } from "pinia"
import { beforeEach, describe, expect, it, vi } from "vitest"
import type { AuthApi } from "@/request/api/auth"
import { createAuthSessionController } from "@/services/auth-session"
import { useAuthStore } from "@/stores/auth"

const now = Date.parse("2026-09-23T00:00:00Z")
const session = { accessToken: "signed-token", tokenType: "Bearer", expiresAt: "2026-09-23T01:00:00Z" } as const

function memoryStorage(initial?: string) {
  const values = new Map<string, string>(initial === undefined ? [] : [["scyg.auth.session", initial]])
  return {
    getItem: vi.fn((key: string) => values.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => { values.set(key, value) }),
    removeItem: vi.fn((key: string) => { values.delete(key) }),
  }
}

describe("authentication session", () => {
  beforeEach(() => setActivePinia(createPinia()))

  it("persists a successful login and restores it for a new store", async () => {
    const api: AuthApi = { login: vi.fn().mockResolvedValue(session) }
    const storage = memoryStorage()
    const first = createAuthSessionController(api, useAuthStore(), storage, () => now)

    await expect(first.login({ username: "author", password: "secret" })).resolves.toEqual(session)
    expect(first.accessToken()).toBe("signed-token")

    setActivePinia(createPinia())
    const restored = createAuthSessionController(api, useAuthStore(), storage, () => now)
    expect(restored.restore()).toEqual({ kind: "authenticated", session })
  })

  it("discards an expired persisted session", () => {
    const storage = memoryStorage(JSON.stringify({ ...session, expiresAt: "2026-09-22T23:59:59Z" }))
    const controller = createAuthSessionController({ login: vi.fn() }, useAuthStore(), storage, () => now)

    expect(controller.restore()).toEqual({ kind: "expired", reason: "登录会话已过期，请重新登录" })
    expect(controller.accessToken()).toBeUndefined()
    expect(storage.removeItem).toHaveBeenCalledWith("scyg.auth.session")
  })

  it("clears an authenticated session after a 401 or local logout", async () => {
    const storage = memoryStorage()
    const controller = createAuthSessionController({ login: vi.fn().mockResolvedValue(session) }, useAuthStore(), storage, () => now)
    await controller.login({ username: "author", password: "secret" })

    controller.handleUnauthorized()
    expect(controller.currentState().kind).toBe("expired")
    expect(controller.logout()).toEqual({ kind: "anonymous" })
  })
})
