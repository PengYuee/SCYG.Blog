import type { LoginData, LoginResponse } from "@/request/generated"
import { zLoginResponse } from "@/request/generated/zod.gen"
import type { HttpTransport } from "@/request/transport"
import { parseBoundary } from "@/types/api"

const LOGIN_ROUTE: LoginData["url"] = "/api/v1/auth/login"

/** 后端当前提供的用户名密码登录能力。 */
export interface AuthApi {
  readonly login: (request: Readonly<LoginData["body"]>) => Promise<LoginResponse>
}

/** 创建复用共享 transport 的认证 API。 */
export function createAuthApi(client: HttpTransport): AuthApi {
  return {
    async login(request): Promise<LoginResponse> {
      const body: LoginData["body"] = { username: request.username.trim(), password: request.password }
      return parseBoundary(zLoginResponse, (await client.post(LOGIN_ROUTE, body)).data, "login")
    },
  }
}
