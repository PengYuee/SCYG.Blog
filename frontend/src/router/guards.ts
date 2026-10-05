import type { NavigationGuard } from "vue-router"
import type { AuthSessionController } from "@/services/auth-session"

const DEFAULT_AUTHENTICATED_TARGET = "/author/articles/new"

/** 只接受当前站点内的绝对路径，拒绝协议相对跳转。 */
export function resolveAuthRedirect(value: unknown): string {
  return typeof value === "string" && value.startsWith("/") && !value.startsWith("//") ? value : DEFAULT_AUTHENTICATED_TARGET
}

/** 创建同时保护作者路由并阻止已登录用户停留在登录页的导航守卫。 */
export function createAuthNavigationGuard(session: Pick<AuthSessionController, "currentState">): NavigationGuard {
  return (to) => {
    const authenticated = session.currentState().kind === "authenticated"
    if (to.matched.some((record) => record.meta["requiresAuth"] === true) && !authenticated) {
      return { name: "login", query: { redirect: to.fullPath } }
    }
    if (to.matched.some((record) => record.meta["guestOnly"] === true) && authenticated) {
      return resolveAuthRedirect(to.query["redirect"])
    }
    return true
  }
}
