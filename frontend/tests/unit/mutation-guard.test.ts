import { describe, expect, it, vi } from "vitest"
import { createMutationGuard } from "@/services/mutation-guard"
import { parseAuthRuntimeConfig, type AuthState } from "@/stores/auth"

const production = parseAuthRuntimeConfig({ mode: "production", fakeAuthEnabled: true })
const unsupported: AuthState = { kind: "unsupported", reason: "backend auth unavailable" }

describe("T5 production mutation guard", () => {
  it.each(["article", "taxonomy", "image"] as const)("blocks %s before adapter invocation", async (domain) => {
    // Given: production mode and a spy representing the real adapter boundary.
    const adapter = vi.fn(async () => `${domain}-called`)
    const guard = createMutationGuard(production, () => unsupported)
    // When: the domain mutation is requested lazily.
    const result = await guard.execute(domain, adapter)
    // Then: a stable actionable result is returned and the adapter sees zero calls.
    expect(result).toEqual({ ok: false, error: { code: "MUTATION_BLOCKED", domain, reason: "Authoring is unavailable because backend authentication is not supported." } })
    expect(adapter).toHaveBeenCalledTimes(0)
  })

  it("allows an explicit fake author outside production", async () => {
    // Given: explicit test fake configuration and author claims.
    const config = parseAuthRuntimeConfig({ mode: "test", fakeAuthEnabled: true })
    const state: AuthState = { kind: "authenticated", source: "fake", claims: { roles: ["author"], permissions: ["article:write"] } }
    const adapter = vi.fn(async () => "created")
    // When: the guarded operation executes.
    const result = await createMutationGuard(config, () => state).execute("article", adapter)
    // Then: the adapter is called exactly once and its value is preserved.
    expect(result).toEqual({ ok: true, value: "created" })
    expect(adapter).toHaveBeenCalledTimes(1)
  })
})
