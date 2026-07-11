import { describe, expect, it } from "vitest"
import { authorRouteAvailability } from "@/router/guards"
import { parseAuthRuntimeConfig, type AuthState } from "@/stores/auth"

describe("T5 truthful author route guard", () => {
  it("returns unavailable rather than fake login or authentication in production", () => {
    // Given: production and the truthful unsupported auth state.
    const state: AuthState = { kind: "unsupported", reason: "backend auth unavailable" }
    // When: an author path is evaluated.
    const result = authorRouteAvailability(parseAuthRuntimeConfig({ mode: "production", fakeAuthEnabled: false }), state)
    // Then: navigation receives an explicit unavailable outcome with no redirect.
    expect(result).toEqual({ kind: "unavailable", code: "AUTHORING_UNAVAILABLE", reason: "Authoring is unavailable because backend authentication is not supported." })
    expect(result).not.toHaveProperty("redirect")
  })

  it("makes author routes available to an explicit fake author in test mode", () => {
    // Given: explicit test fake mode and author claims.
    const state: AuthState = { kind: "authenticated", source: "fake", claims: { roles: ["author"], permissions: ["article:write"] } }
    // When / Then: availability is derived from the same truthful capability.
    expect(authorRouteAvailability(parseAuthRuntimeConfig({ mode: "test", fakeAuthEnabled: true }), state)).toEqual({ kind: "available" })
  })
})
