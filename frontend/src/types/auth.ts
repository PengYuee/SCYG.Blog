import type { UnsupportedResult } from "./api"

/** 未来认证声明；生产适配器当前不创建声明。 */
export type AuthClaims = {
  /** 显式角色。 */ readonly roles: readonly string[]
  /** 显式权限。 */ readonly permissions: readonly string[]
}

/** 登录输入。 */
export type LoginRequest = { readonly username: string; readonly password: string }

/** 刷新认证输入；opaque 值只由未来后端定义。 */
export type RefreshRequest = { readonly refreshToken: string }

/** 查询当前身份输入；当前没有请求字段。 */
export type MeRequest = Readonly<Record<string, never>>

/** 注销输入；opaque 值只由未来后端定义。 */
export type LogoutRequest = { readonly refreshToken: string }

/** 认证 API 契约。 */
export interface AuthApi {
  /** 尝试登录，未启用时返回稳定 unsupported。 */ login(request: LoginRequest): Promise<UnsupportedResult>
  /** 尝试刷新认证，未启用时返回稳定 unsupported。 */ refresh(request: RefreshRequest): Promise<UnsupportedResult>
  /** 查询当前身份，未启用时返回稳定 unsupported。 */ me(request: MeRequest): Promise<UnsupportedResult>
  /** 尝试注销，未启用时返回稳定 unsupported。 */ logout(request: LogoutRequest): Promise<UnsupportedResult>
}

/** 仅根据明确作者声明判断写作能力。 */
export function canAuthor(claims: AuthClaims): boolean {
  return claims.roles.includes("author") && claims.permissions.includes("article:write")
}

/** 仅根据明确管理员声明判断管理能力。 */
export function canAdmin(claims: AuthClaims): boolean {
  return claims.roles.includes("admin") && claims.permissions.includes("admin:access")
}
