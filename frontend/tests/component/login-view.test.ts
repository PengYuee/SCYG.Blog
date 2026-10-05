import { flushPromises, mount } from "@vue/test-utils"
import { createMemoryHistory, createRouter } from "vue-router"
import { describe, expect, it, vi } from "vitest"
import { authSessionKey, type AuthSessionController } from "@/services/auth-session"
import LoginView from "@/views/public/LoginView.vue"

const session = { accessToken: "signed-token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" } as const

async function mountLogin(login: AuthSessionController["login"], target = "/author/articles/new") {
  const controller: AuthSessionController = {
    restore: () => ({ kind: "anonymous" }),
    login,
    logout: () => ({ kind: "anonymous" }),
    currentState: () => ({ kind: "anonymous" }),
    accessToken: () => undefined,
    handleUnauthorized: () => undefined,
  }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/login", component: LoginView },
      { path: "/author/articles/new", component: { template: "<div>editor</div>" } },
    ],
  })
  await router.push({ path: "/login", query: { redirect: target } })
  await router.isReady()
  const wrapper = mount(LoginView, {
    global: {
      plugins: [router],
      provide: { [authSessionKey]: controller },
      stubs: { PublicLayout: { template: "<main><slot /></main>" } },
    },
  })
  return { wrapper, router }
}

describe("LoginView", () => {
  it("submits credentials and returns to the protected author route", async () => {
    const login = vi.fn().mockResolvedValue(session)
    const { wrapper, router } = await mountLogin(login)

    await wrapper.get("#login-username").setValue("author")
    await wrapper.get("#login-password").setValue("secret")
    await wrapper.get("form").trigger("submit")
    await flushPromises()

    expect(login).toHaveBeenCalledWith({ username: "author", password: "secret" })
    expect(router.currentRoute.value.path).toBe("/author/articles/new")
  })

  it("shows a login failure and clears the password", async () => {
    const { wrapper } = await mountLogin(vi.fn().mockRejectedValue(new Error("用户名或密码错误")))
    await wrapper.get("#login-username").setValue("author")
    await wrapper.get("#login-password").setValue("wrong")
    await wrapper.get("form").trigger("submit")
    await flushPromises()

    expect(wrapper.get("[role='alert']").text()).toBe("用户名或密码错误")
    expect((wrapper.get("#login-password").element as HTMLInputElement).value).toBe("")
  })
})
