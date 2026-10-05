import { createMemoryHistory, createRouter } from "vue-router"
import { describe, expect, it } from "vitest"
import { createAuthNavigationGuard, resolveAuthRedirect } from "@/router/guards"
import type { AuthState } from "@/stores/auth"

const authenticated: AuthState = { kind: "authenticated", session: { accessToken: "token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" } }

function guardedRouter(state: () => AuthState) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/login", name: "login", component: {}, meta: { guestOnly: true } },
      { path: "/author", component: {}, meta: { requiresAuth: true } },
    ],
  })
  router.beforeEach(createAuthNavigationGuard({ currentState: state }))
  return router
}

describe("authentication navigation guard", () => {
  it("redirects an anonymous author navigation to login with its return target", async () => {
    const router = guardedRouter(() => ({ kind: "anonymous" }))
    await router.push("/author")
    expect(router.currentRoute.value.fullPath).toBe("/login?redirect=/author")
  })

  it("allows an authenticated author and rejects external return targets", async () => {
    const router = guardedRouter(() => authenticated)
    await router.push("/author")
    expect(router.currentRoute.value.path).toBe("/author")
    expect(resolveAuthRedirect("//evil.example/path")).toBe("/author/articles/new")
  })
})
