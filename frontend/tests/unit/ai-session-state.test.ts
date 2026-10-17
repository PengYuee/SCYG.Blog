import { AxiosError, AxiosHeaders } from "axios"
import { afterEach, describe, expect, it, vi } from "vitest"
import type { AiApi } from "@/request/api/ai"

const credential = { accessToken: "real-anonymous", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 }
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.resetModules() })
// Dynamic imports intentionally reload singleton session memory after resetModules to model independent browser lifetimes.

describe("AI session storage boundaries", () => {
  it.each(["getItem", "setItem"])("author subject remains stable across mounts when %s fails", async (failing) => {
    const data = new Map<string, string>()
    vi.stubGlobal("sessionStorage", {
      getItem: (key: string) => { if (failing === "getItem") throw new Error("blocked"); return data.get(key) ?? null },
      setItem: (key: string, value: string) => { if (failing === "setItem") throw new Error("quota"); data.set(key, value) },
      get length() { return data.size }, key: (index: number) => [...data.keys()][index] ?? null,
      removeItem: () => { throw new Error("blocked") },
    })
    const state = await import("@/services/ai-session-state")
    const subject = state.authorTabSubject("token-a")
    expect(state.authorTabSubject("token-a")).toBe(subject)
    expect(state.authorTabSubject("token-b")).not.toBe(subject)
    const current = state.authorTabSubject("token-a")
    const controller = new AbortController()
    const unsubscribe = state.onAuthorSessionChange(() => controller.abort())
    expect(() => state.clearAuthorAiState()).not.toThrow()
    expect(controller.signal.aborted).toBe(true)
    expect(state.authorTabSubject("token-a")).not.toBe(current)
    unsubscribe()
  })

  it("unreadable anonymous storage notifies listeners without throwing", async () => {
    vi.stubGlobal("sessionStorage", { getItem: () => { throw new Error("blocked") }, removeItem: () => { throw new Error("blocked") } })
    const state = await import("@/services/anonymous-ai-session")
    const controller = new AbortController()
    const unsubscribe = state.onAnonymousSessionChange(() => controller.abort())
    expect(state.restoreAnonymousSession()).toBeUndefined()
    expect(controller.signal.aborted).toBe(true)
    await expect(state.ensureAnonymousSession({} as AiApi, false)).rejects.toMatchObject({ status: 401 })
    unsubscribe()
  })

  it("quota failure keeps a real credential usable, and remove failure still terminates it at HTTP 401", async () => {
    vi.stubGlobal("sessionStorage", { getItem: () => null, setItem: () => { throw new Error("quota") }, removeItem: () => { throw new Error("blocked") } })
    const state = await import("@/services/anonymous-ai-session")
    const session = vi.fn(async () => credential)
    const api = { session } as unknown as AiApi
    expect(await state.ensureAnonymousSession(api, true)).toEqual(credential)
    expect(await state.ensureAnonymousSession(api, false)).toEqual(credential)
    expect(session).toHaveBeenCalledOnce()
    const controller = new AbortController()
    const unsubscribe = state.onAnonymousSessionChange(() => controller.abort())
    const { http } = await import("@/request/http")
    await expect(http.get("/api/v1/runs/run", { authPolicy: "anonymous", adapter: async (config) => {
      expect(config.headers.get("Authorization")).toBe("Bearer real-anonymous")
      throw new AxiosError("unauthorized", "ERR_BAD_REQUEST", config, undefined, { config, status: 401, statusText: "Unauthorized", data: {}, headers: new AxiosHeaders() })
    } })).rejects.toMatchObject({ status: 401 })
    expect(controller.signal.aborted).toBe(true)
    expect(state.restoreAnonymousSession()).toBeUndefined()
    await expect(state.ensureAnonymousSession(api, false)).rejects.toMatchObject({ status: 401 })
    expect(session).toHaveBeenCalledOnce()
    unsubscribe()
  })

  it("does not claim refresh recovery when the credential could not be stored", async () => {
    vi.stubGlobal("sessionStorage", { getItem: () => null, setItem: () => { throw new Error("quota") }, removeItem: vi.fn() })
    const state = await import("@/services/anonymous-ai-session")
    const session = vi.fn(async () => credential)
    const api = { session } as unknown as AiApi
    expect(await state.ensureAnonymousSession(api, true)).toEqual(credential)
    vi.resetModules()
    const refreshed = await import("@/services/anonymous-ai-session")
    expect(refreshed.restoreAnonymousSession()).toBeUndefined()
    await expect(refreshed.ensureAnonymousSession(api, false)).rejects.toMatchObject({ status: 401 })
    expect(session).toHaveBeenCalledOnce()
  })

  it("cannot resurrect a newly issued credential after invalidation", async () => {
    vi.stubGlobal("sessionStorage", { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() })
    const state = await import("@/services/anonymous-ai-session")
    const { promise, resolve } = Promise.withResolvers<typeof credential>()
    const issued = state.ensureAnonymousSession({ session: () => promise } as unknown as AiApi, true)
    state.invalidateAnonymousSession()
    resolve(credential)
    await expect(issued).rejects.toMatchObject({ status: 401 })
    expect(state.restoreAnonymousSession()).toBeUndefined()
  })
})
