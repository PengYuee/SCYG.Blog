import { describe, expect, expectTypeOf, it } from "vitest"
import type { AuthApi, LoginRequest, LogoutRequest, MeRequest, RefreshRequest } from "@/types/auth"
import { canAdmin, canAuthor } from "@/types/auth"
import { unsupportedAuthApi } from "@/request/api/auth"
import { unsupportedSearchApi } from "@/request/api/search"

describe("truthful unsupported adapters", () => {
  it("uses q in the typed Search request", async () => {
    // Given: the exact future remote-search contract.
    const request = { q: "vue", pageIndex: 1, pageSize: 20, articleTypeId: 2, tagId: 3 }
    expectTypeOf(request).toMatchTypeOf<Parameters<typeof unsupportedSearchApi.search>[0]>()
    // When: production search is invoked.
    const search = await unsupportedSearchApi.search(request)
    // Then: the adapter truthfully reports unsupported.
    expect(search).toEqual({ kind: "unsupported", feature: "search" })
  })

  it("exposes Login Refresh Me and Logout as stable unsupported methods", async () => {
    // Given / When: production-only future contracts are invoked.
    expectTypeOf(unsupportedAuthApi).toMatchTypeOf<AuthApi>()
    expectTypeOf<LoginRequest>().toMatchTypeOf<{ readonly username: string; readonly password: string }>()
    expectTypeOf<RefreshRequest>().toMatchTypeOf<{ readonly refreshToken: string }>()
    expectTypeOf<MeRequest>().toMatchTypeOf<Readonly<Record<string, never>>>()
    expectTypeOf<LogoutRequest>().toMatchTypeOf<{ readonly refreshToken: string }>()
    const results = await Promise.all([
      unsupportedAuthApi.login({ username: "author", password: "secret" }),
      unsupportedAuthApi.refresh({ refreshToken: "opaque" }),
      unsupportedAuthApi.me({}),
      unsupportedAuthApi.logout({ refreshToken: "opaque" }),
    ])
    // Then: no method invents production success or claims.
    expect(results).toEqual([
      { kind: "unsupported", feature: "auth" },
      { kind: "unsupported", feature: "auth" },
      { kind: "unsupported", feature: "auth" },
      { kind: "unsupported", feature: "auth" },
    ])
  })

  it("derives capabilities only from explicit readonly claims", () => {
    // Given: explicit claims and an empty production identity.
    const author = { roles: ["author"], permissions: ["article:write"] } as const
    const empty = { roles: [], permissions: [] } as const
    // When / Then: capability helpers never infer admin claims.
    expect(canAuthor(author)).toBe(true)
    expect(canAdmin(author)).toBe(false)
    expect(canAuthor(empty)).toBe(false)
    expect(canAdmin(empty)).toBe(false)
  })
})
