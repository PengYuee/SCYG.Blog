import type { AuthApi } from "@/types/auth"

/** 生产认证尚未启用，始终返回稳定 unsupported。 */
export const unsupportedAuthApi: AuthApi = {
  /** 不发起网络请求。 */ async login() { return { kind: "unsupported", feature: "auth" } },
  /** 不发起网络请求。 */ async refresh() { return { kind: "unsupported", feature: "auth" } },
  /** 不发起网络请求。 */ async me() { return { kind: "unsupported", feature: "auth" } },
  /** 不发起网络请求。 */ async logout() { return { kind: "unsupported", feature: "auth" } },
}
