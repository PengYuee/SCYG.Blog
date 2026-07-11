import type { UnsupportedResult } from "./api"

/** 未来认证声明；生产适配器当前不创建声明。 */
export type AuthClaims = {
  /** 显式角色。 */ readonly roles: readonly string[]
  /** 显式权限。 */ readonly permissions: readonly string[]
}

/** 登录输入。 */
export type LoginRequest = { readonly username: string; readonly password: string }

/** 认证 API 契约。 */
export interface AuthApi {
  /** 尝试登录，未启用时返回稳定 unsupported。 */ login(request: LoginRequest): Promise<UnsupportedResult>
}

/** 仅根据明确作者声明判断写作能力。 */
export function canAuthor(claims: AuthClaims): boolean {
  return claims.roles.includes("author") && claims.permissions.includes("article:write")
}

/** 仅根据明确管理员声明判断管理能力。 */
export function canAdmin(claims: AuthClaims): boolean {
  return claims.roles.includes("admin") && claims.permissions.includes("admin:access")
}
