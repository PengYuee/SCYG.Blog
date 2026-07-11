import { defineStore } from "pinia"
import { computed, readonly, ref } from "vue"
import { z } from "zod"
import { canAdmin as claimsCanAdmin, canAuthor as claimsCanAuthor, type AuthClaims } from "@/types/auth"

/** 浏览器认证运行模式。 */
export type AuthRuntimeMode = "production" | "development" | "test"

/** 经过边界解析的认证运行时配置。 */
export type AuthRuntimeConfig = {
  /** 当前部署模式。 */ readonly mode: AuthRuntimeMode
  /** 是否显式启用本地 Fake Auth；生产模式解析后始终为 false。 */ readonly fakeAuthEnabled: boolean
}

/** 完整认证生命周期；每一时刻只能处于一个状态。 */
export type AuthState =
  | { readonly kind: "anonymous" }
  | { readonly kind: "restoring" }
  | { readonly kind: "authenticated"; readonly source: "backend" | "fake"; readonly claims: AuthClaims }
  | { readonly kind: "expired"; readonly reason: string }
  | { readonly kind: "unsupported"; readonly reason: string }

/** 外部认证配置解析失败。 */
export class AuthRuntimeConfigError extends Error {
  /** 稳定错误名称。 */ readonly name = "AuthRuntimeConfigError"
  /** 稳定错误代码。 */ readonly code = "AUTH_CONFIG_INVALID"

  /** 创建保留原始 Zod 原因的配置错误。 */
  constructor(cause: unknown) {
    super("认证运行时配置无效", { cause })
  }
}

const authRuntimeConfigSchema = z.strictObject({
  mode: z.enum(["production", "development", "test"]),
  fakeAuthEnabled: z.boolean(),
})

/** 解析唯一认证模式边界，并硬关闭生产 Fake Auth。 */
export function parseAuthRuntimeConfig(input: unknown): AuthRuntimeConfig {
  const result = authRuntimeConfigSchema.safeParse(input)
  if (!result.success) throw new AuthRuntimeConfigError(result.error)
  return {
    mode: result.data.mode,
    fakeAuthEnabled: result.data.mode === "production" ? false : result.data.fakeAuthEnabled,
  }
}

/** 创建由已解析配置注入的认证 Pinia store 定义。 */
export function createAuthStore(config: AuthRuntimeConfig, fakeClaims?: AuthClaims) {
  return defineStore("auth", () => {
    /** Pinia 有意维护的单一认证状态。 */
    const state = ref<AuthState>(initialAuthState(config, fakeClaims))
    /** 当前状态是否具备作者能力。 */
    const canAuthor = computed(() => capabilityClaims(state.value, claimsCanAuthor))
    /** 当前状态是否具备管理能力。 */
    const canAdmin = computed(() => capabilityClaims(state.value, claimsCanAdmin))

    /** 进入后端身份恢复状态。 */
    const markRestoring = (): AuthState => setState({ kind: "restoring" })
    /** 进入明确过期状态。 */
    const markExpired = (reason: string): AuthState => setState({ kind: "expired", reason })
    /** 进入当前后端不支持认证的状态。 */
    const markUnsupported = (reason: string): AuthState => setState({ kind: "unsupported", reason })
    /** 清除内存身份并回到匿名状态。 */
    const markAnonymous = (): AuthState => setState({ kind: "anonymous" })

    /** 集中替换认证状态并返回新快照。 */
    function setState(nextState: AuthState): AuthState {
      state.value = nextState
      return nextState
    }

    return { state: readonly(state), canAuthor, canAdmin, markRestoring, markExpired, markUnsupported, markAnonymous }
  })
}

/** 根据显式配置和 claims 选择初始认证状态。 */
function initialAuthState(config: AuthRuntimeConfig, fakeClaims?: AuthClaims): AuthState {
  if (config.mode === "production") return { kind: "unsupported", reason: "backend auth unavailable" }
  if (!config.fakeAuthEnabled) return { kind: "anonymous" }
  if (fakeClaims === undefined) return { kind: "unsupported", reason: "explicit fake claims required" }
  return { kind: "authenticated", source: "fake", claims: fakeClaims }
}

/** 仅对 authenticated 分支调用 T3 claims 能力辅助函数。 */
function capabilityClaims(state: AuthState, capability: (claims: AuthClaims) => boolean): boolean {
  switch (state.kind) {
    case "authenticated": return capability(state.claims)
    case "anonymous":
    case "restoring":
    case "expired":
    case "unsupported": return false
  }
}
