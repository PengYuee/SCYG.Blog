import { mount, type VueWrapper } from "@vue/test-utils"
import { createMemoryHistory, createRouter } from "vue-router"
import { describe, expect, it, vi } from "vitest"
import BlogNavigation from "@/components/public/BlogNavigation.vue"
import { authSessionKey, type AuthSessionController } from "@/services/auth-session"

const routes = ["/", "/articles", "/login", "/author/articles/new"].map((path) => ({ path, component: { template: "<div />" } }))

async function mountNavigation(authenticated: boolean, path = "/", homeAction?: () => void): Promise<VueWrapper> {
  const router = createRouter({ history: createMemoryHistory(), routes })
  await router.push(path)
  await router.isReady()
  const state = authenticated ? { kind: "authenticated", session: { accessToken: "token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" } } as const : { kind: "anonymous" } as const
  const authSession = { currentState: () => state } as Pick<AuthSessionController, "currentState">
  return mount(BlogNavigation, {
    props: homeAction === undefined ? {} : { homeAction },
    global: { plugins: [router], provide: { [authSessionKey]: authSession } },
  })
}

describe("BlogNavigation authentication entry", () => {

  it("shows the author workspace for an authenticated session", async () => {
    const wrapper = await mountNavigation(true)
    expect(wrapper.get('a[href="/author/articles/new"]').text()).toBe("进入管理")
    expect(wrapper.findAll("a")).toHaveLength(2)
    expect(wrapper.get('a[href="/"]').text()).toBe("主页")
    expect(wrapper.find('a[href="/login"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it("shows login instead of authoring for an anonymous session", async () => {
    const wrapper = await mountNavigation(false)
    expect(wrapper.get('a[href="/login"]').text()).toBe("登录")
    expect(wrapper.find('a[href="/author/articles/new"]').exists()).toBe(false)
    expect(wrapper.findAll("a")).toHaveLength(2)
    expect(wrapper.get('a[href="/"]').text()).toBe("主页")
    wrapper.unmount()
  })

  it("uses a homepage button to invoke the supplied Hero action without changing the route", async () => {
    const homeAction = vi.fn()
    const wrapper = await mountNavigation(false, "/?mode=ai&runId=retained-run", homeAction)
    const home = wrapper.findAll("button").find((button) => button.text() === "主页")
    expect(home).toBeDefined()
    expect(home!.element.tagName).toBe("BUTTON")
    expect(home!.attributes("type")).toBe("button")
    expect(home!.attributes("href")).toBeUndefined()
    expect(wrapper.find('a[href="/"]').exists()).toBe(false)
    await home!.trigger("click")
    expect(homeAction).toHaveBeenCalledOnce()
    expect(wrapper.vm.$route.fullPath).toBe("/?mode=ai&runId=retained-run")
    expect(wrapper.get('a[href="/login"]').text()).toBe("登录")
    wrapper.unmount()
  })

  it("does not expose the homepage action on an article route", async () => {
    const wrapper = await mountNavigation(true, "/articles")
    expect(wrapper.findAll("button").some((button) => button.text() === "主页")).toBe(false)
    expect(wrapper.get('a[href="/"]').text()).toBe("主页")
    expect(wrapper.get('a[href="/author/articles/new"]').text()).toBe("进入管理")
    wrapper.unmount()
  })
})
