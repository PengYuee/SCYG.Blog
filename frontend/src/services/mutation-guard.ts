import { authorRouteAvailability } from "@/router/guards"
import type { AuthRuntimeConfig, AuthState } from "@/stores/auth"

/** 受保护的写操作领域。 */
export type MutationDomain = "article" | "taxonomy" | "image"

/** 写操作被策略阻断的稳定结构化错误。 */
export type MutationBlockedError = {
  /** 稳定错误代码。 */ readonly code: "MUTATION_BLOCKED"
  /** 被阻断的适配器领域。 */ readonly domain: MutationDomain
  /** 面向调用方的可操作原因。 */ readonly reason: string
}

/** 守卫执行结果；预期阻断不使用异常。 */
export type MutationResult<Value> =
  | { readonly ok: true; readonly value: Value }
  | { readonly ok: false; readonly error: MutationBlockedError }

/** 单一写操作守卫契约。 */
export type MutationGuard = {
  /** 惰性执行适配器，保证拒绝发生在调用之前。 */
  readonly execute: <Value>(domain: MutationDomain, operation: () => Promise<Value>) => Promise<MutationResult<Value>>
}

/** 创建供 article、taxonomy 与 image 适配器共享的生产写守卫。 */
export function createMutationGuard(config: AuthRuntimeConfig, currentState: () => AuthState): MutationGuard {
  return {
    /** 仅在作者路由能力真实可用后调用传入适配器。 */
    async execute<Value>(domain: MutationDomain, operation: () => Promise<Value>): Promise<MutationResult<Value>> {
      const availability = authorRouteAvailability(config, currentState())
      if (availability.kind === "unavailable") {
        return { ok: false, error: { code: "MUTATION_BLOCKED", domain, reason: availability.reason } }
      }
      return { ok: true, value: await operation() }
    },
  }
}
