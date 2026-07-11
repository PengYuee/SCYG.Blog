import { canAuthor } from "@/types/auth"
import type { AuthRuntimeConfig, AuthState } from "@/stores/auth"

/** 作者路由不可用的稳定用户可操作原因。 */
export const AUTHORING_UNAVAILABLE_REASON = "Authoring is unavailable because backend authentication is not supported."

/** 作者路由真实可用性，不伪造登录跳转。 */
export type AuthorRouteAvailability =
  | { readonly kind: "available" }
  | { readonly kind: "unavailable"; readonly code: "AUTHORING_UNAVAILABLE"; readonly reason: string }

/** 根据注入模式与当前认证状态判断作者路由是否真实可用。 */
export function authorRouteAvailability(config: AuthRuntimeConfig, state: AuthState): AuthorRouteAvailability {
  // 生产写作在后端 claims 可解析前保持关闭；Fake 标志无法绕过此分支。
  if (config.mode === "production") return unavailable()
  if (!config.fakeAuthEnabled) return unavailable()
  if (state.kind !== "authenticated" || state.source !== "fake" || !canAuthor(state.claims)) return unavailable()
  return { kind: "available" }
}

/** 构造稳定且不包含虚假重定向的不可用结果。 */
function unavailable(): AuthorRouteAvailability {
  return { kind: "unavailable", code: "AUTHORING_UNAVAILABLE", reason: AUTHORING_UNAVAILABLE_REASON }
}
