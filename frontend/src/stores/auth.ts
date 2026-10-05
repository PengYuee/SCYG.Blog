import { defineStore } from "pinia"
import { computed, readonly, ref } from "vue"
import type { AuthSession } from "@/types/auth"

/** 完整认证生命周期；每一时刻只能处于一个状态。 */
export type AuthState =
  | { readonly kind: "anonymous" }
  | { readonly kind: "restoring" }
  | { readonly kind: "authenticated"; readonly session: AuthSession }
  | { readonly kind: "expired"; readonly reason: string }

/** 全局认证状态；网络和持久化副作用由认证会话服务负责。 */
export const useAuthStore = defineStore("auth", () => {
  const state = ref<AuthState>({ kind: "anonymous" })
  const isAuthenticated = computed(() => state.value.kind === "authenticated")

  const markRestoring = (): AuthState => setState({ kind: "restoring" })
  const markAuthenticated = (session: AuthSession): AuthState => setState({ kind: "authenticated", session })
  const markExpired = (reason: string): AuthState => setState({ kind: "expired", reason })
  const markAnonymous = (): AuthState => setState({ kind: "anonymous" })

  function setState(nextState: AuthState): AuthState {
    state.value = nextState
    return nextState
  }

  return { state: readonly(state), isAuthenticated, markRestoring, markAuthenticated, markExpired, markAnonymous }
})
