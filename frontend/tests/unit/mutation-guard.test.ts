import { describe, expect, it, vi } from "vitest"
import { createMutationGuard } from "@/services/mutation-guard"
import type { AuthState } from "@/stores/auth"

const anonymous: AuthState = { kind: "anonymous" }
const authenticated: AuthState = { kind: "authenticated", session: { accessToken: "token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" } }

describe("authenticated mutation guard", () => {
  it.each(["article", "taxonomy", "image"] as const)("blocks anonymous %s mutations before adapter invocation", async (domain) => {
    const adapter = vi.fn(async () => `${domain}-called`)
    const result = await createMutationGuard(() => anonymous).execute(domain, adapter)

    expect(result).toEqual({ ok: false, error: { code: "MUTATION_BLOCKED", domain, reason: "请先登录后再执行写作操作" } })
    expect(adapter).not.toHaveBeenCalled()
  })

  it("allows an authenticated author mutation", async () => {
    const adapter = vi.fn(async () => "created")
    const result = await createMutationGuard(() => authenticated).execute("article", adapter)

    expect(result).toEqual({ ok: true, value: "created" })
    expect(adapter).toHaveBeenCalledOnce()
  })
})
