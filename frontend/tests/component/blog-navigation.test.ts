import { mount, type VueWrapper } from "@vue/test-utils"
import { createMemoryHistory, createRouter } from "vue-router"
import { describe, expect, it } from "vitest"
import BlogNavigation from "@/components/public/BlogNavigation.vue"
import { authSessionKey, type AuthSessionController } from "@/services/auth-session"

const routes = ["/", "/articles", "/login", "/author/articles/new"].map((path) => ({ path, component: { template: "<div />" } }))

async function mountNavigation(authenticated: boolean): Promise<VueWrapper> {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push("/")
  await router.isReady()
  const state = authenticated ? { kind: "authenticated", session: { accessToken: "token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" } } as const : { kind: "anonymous" } as const
  const authSession = { currentState: () => state } as Pick<AuthSessionController, "currentState">
  return mount(BlogNavigation, { global: { plugins: [router], provide: { [authSessionKey]: authSession } } })
}

describe("BlogNavigation authentication entry", () => {

  it("shows the author workspace for an authenticated session", async () => {
    const wrapper = await mountNavigation(true)
    expect(wrapper.get('a[href="/author/articles/new"]').text()).toBe("写作台")
    expect(wrapper.find('a[href="/login"]').exists()).toBe(false)
  })

  it("shows login instead of authoring for an anonymous session", async () => {
    const wrapper = await mountNavigation(false)
    expect(wrapper.get('a[href="/login"]').text()).toBe("登录")
    expect(wrapper.find('a[href="/author/articles/new"]').exists()).toBe(false)
    expect(wrapper.get('a[href="/"]').text()).toBe("首页")
    expect(wrapper.get('a[href="/articles"]').text()).toBe("文章")
  })
})
