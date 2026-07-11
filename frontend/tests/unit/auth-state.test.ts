import { createPinia, setActivePinia } from "pinia"
import { beforeEach, describe, expect, it } from "vitest"
import { parseAuthRuntimeConfig, createAuthStore, type AuthState } from "@/stores/auth"
import { canAdmin, canAuthor } from "@/types/auth"
import { unsupportedAuthApi } from "@/request/api/auth"

const fakeAuthorClaims = { roles: ["author"], permissions: ["article:write"] } as const

describe("T5 auth state", () => {
  beforeEach(() => setActivePinia(createPinia()))

  it("preserves the current unsupported Auth contract and explicit capability baseline", async () => {
    // Given: T3 production Auth and explicit author claims.
    // When: Auth is queried and capabilities are derived.
    const result = await unsupportedAuthApi.me({})
    // Then: unsupported remains truthful and no admin capability is inferred.
    expect(result).toEqual({ kind: "unsupported", feature: "auth" })
    expect(canAuthor(fakeAuthorClaims)).toBe(true)
    expect(canAdmin(fakeAuthorClaims)).toBe(false)
  })

  it("parses mode and disables a fake flag in production", () => {
    // Given / When: hostile production configuration contains the fake flag.
    const config = parseAuthRuntimeConfig({ mode: "production", fakeAuthEnabled: true })
    // Then: production makes fake Auth impossible.
    expect(config).toEqual({ mode: "production", fakeAuthEnabled: false })
  })

  it.each([
    { mode: "preview", fakeAuthEnabled: true },
    { mode: "test", fakeAuthEnabled: "true" },
    { mode: "development" },
  ])("rejects malformed runtime configuration %#", (input) => {
    // Given / When / Then: malformed external mode or flag input is rejected at its boundary.
    expect(() => parseAuthRuntimeConfig(input)).toThrowError(expect.objectContaining({ code: "AUTH_CONFIG_INVALID" }))
  })

  it("creates an authenticated fake author only with an explicit non-production flag", () => {
    // Given: validated test configuration and readonly explicit claims.
    const useAuthStore = createAuthStore(parseAuthRuntimeConfig({ mode: "test", fakeAuthEnabled: true }), fakeAuthorClaims)
    // When: the injected store is created.
    const store = useAuthStore()
    // Then: capabilities come from T3 helpers and no token or user is invented.
    expect(store.state).toEqual({ kind: "authenticated", source: "fake", claims: fakeAuthorClaims })
    expect(store.canAuthor).toBe(true)
    expect(store.canAdmin).toBe(false)
    expect(JSON.stringify(store.state)).not.toMatch(/token|user/i)
  })

  it("exposes every discriminated restoration outcome without boolean auth flags", () => {
    // Given: an explicit anonymous store.
    const useAuthStore = createAuthStore(parseAuthRuntimeConfig({ mode: "development", fakeAuthEnabled: false }))
    const store = useAuthStore()
    // When: lifecycle transitions are applied.
    const states: AuthState[] = [store.markRestoring(), store.markExpired("session expired"), store.markUnsupported("backend auth unavailable"), store.markAnonymous()]
    // Then: each outcome is represented by one discriminant.
    expect(states.map((state) => state.kind)).toEqual(["restoring", "expired", "unsupported", "anonymous"])
  })
})
