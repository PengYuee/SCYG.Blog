import type { LoginData, LoginResponse } from "@/request/generated"

/** 登录输入直接复用 OpenAPI 生成的请求体。 */
export type LoginRequest = Readonly<LoginData["body"]>

/** 浏览器保存的短期 Bearer 会话。 */
export type AuthSession = Readonly<LoginResponse>
