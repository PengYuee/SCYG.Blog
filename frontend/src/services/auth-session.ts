import type { InjectionKey } from "vue"
import { inject } from "vue"
import { zLoginResponse } from "@/request/generated/zod.gen"
import type { AuthApi } from "@/request/api/auth"
import type { AuthState } from "@/stores/auth"
import type { AuthSession, LoginRequest } from "@/types/auth"

const SESSION_STORAGE_KEY = "scyg.auth.session"
const EXPIRED_REASON = "登录会话已过期，请重新登录"

/** 认证会话使用的最小 Pinia 状态边界。 */
type AuthStateStore = {
  readonly state: AuthState
  readonly markRestoring: () => AuthState
  readonly markAuthenticated: (session: AuthSession) => AuthState
  readonly markExpired: (reason: string) => AuthState
  readonly markAnonymous: () => AuthState
}

/** 会话持久化只依赖浏览器 Storage 的必要操作。 */
type SessionStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">

/** 登录、恢复、Bearer 读取和本地退出的统一运行时。 */
export interface AuthSessionController {
  readonly restore: () => AuthState
  readonly login: (request: LoginRequest) => Promise<AuthSession>
  readonly logout: () => AuthState
  readonly currentState: () => AuthState
  readonly accessToken: () => string | undefined
  readonly handleUnauthorized: () => void
}

/** 浏览器认证会话服务注入键。 */
export const authSessionKey: InjectionKey<AuthSessionController> = Symbol("auth-session")

/** 读取应用组合根提供的认证会话服务。 */
export function useAuthSession(): AuthSessionController {
  const session = inject(authSessionKey)
  if (session === undefined) throw new Error("缺少认证会话提供者")
  return session
}

/** 创建短期 Bearer 会话控制器；不解析或信任 JWT payload。 */
export function createAuthSessionController(
  api: AuthApi,
  store: AuthStateStore,
  storage: SessionStorage,
  now: () => number = Date.now,
): AuthSessionController {
  function discard(): void {
    storage.removeItem(SESSION_STORAGE_KEY)
  }

  function expire(): AuthState {
    discard()
    return store.markExpired(EXPIRED_REASON)
  }

  function reconcile(): AuthState {
    const state = store.state
    if (state.kind === "authenticated" && Date.parse(state.session.expiresAt) <= now()) return expire()
    return state
  }

  return {
    restore(): AuthState {
      store.markRestoring()
      const raw = storage.getItem(SESSION_STORAGE_KEY)
      if (raw === null) return store.markAnonymous()
      try {
        const session = zLoginResponse.parse(JSON.parse(raw))
        if (Date.parse(session.expiresAt) <= now()) return expire()
        return store.markAuthenticated(session)
      } catch {
        discard()
        return store.markAnonymous()
      }
    },
    async login(request): Promise<AuthSession> {
      const session = await api.login(request)
      if (Date.parse(session.expiresAt) <= now()) {
        expire()
        throw new Error("服务端返回了已过期的登录会话")
      }
      storage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session))
      store.markAuthenticated(session)
      return session
    },
    logout(): AuthState {
      discard()
      return store.markAnonymous()
    },
    currentState: reconcile,
    accessToken(): string | undefined {
      const state = reconcile()
      return state.kind === "authenticated" ? state.session.accessToken : undefined
    },
    handleUnauthorized(): void {
      if (store.state.kind === "authenticated") expire()
    },
  }
}
