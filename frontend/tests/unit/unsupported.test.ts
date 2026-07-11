import { describe, expect, it } from "vitest"
import { canAdmin, canAuthor } from "@/types/auth"
import { unsupportedAuthApi } from "@/request/api/auth"
import { unsupportedSearchApi } from "@/request/api/search"

describe("truthful unsupported adapters", () => {
  it("returns stable unsupported auth and search results", async () => {
    // Given / When: production-only future contracts are invoked.
    const login = await unsupportedAuthApi.login({ username: "author", password: "secret" })
    const search = await unsupportedSearchApi.search({ query: "vue", pageIndex: 1, pageSize: 20 })
    // Then: neither adapter invents production success.
    expect(login).toEqual({ kind: "unsupported", feature: "auth" })
    expect(search).toEqual({ kind: "unsupported", feature: "search" })
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
