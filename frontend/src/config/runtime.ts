import { z } from "zod"

/** 已验证的浏览器运行时配置。 */
export type RuntimeConfig = {
  /** 后端服务绝对根地址。 */
  readonly serverUrl: string
}

/** 运行时配置加载或解析失败。 */
export class RuntimeConfigError extends Error {
  /** 稳定错误名称。 */
  readonly name = "RuntimeConfigError"
  /** 稳定错误代码。 */
  readonly code: "CONFIG_FETCH_FAILED" | "CONFIG_INVALID"

  /** 创建带原始原因的配置错误。 */
  constructor(message: string, code: RuntimeConfigError["code"], cause: unknown) {
    super(message, { cause })
    this.code = code
  }
}

const runtimeConfigSchema = z.strictObject({
  serverUrl: z.url().refine((value) => value.startsWith("http://") || value.startsWith("https://"), "serverUrl must use HTTP(S)"),
})

/** 解析外部运行时配置。 */
export function parseRuntimeConfig(input: unknown): RuntimeConfig {
  const result = runtimeConfigSchema.safeParse(input)
  if (!result.success) throw new RuntimeConfigError("运行时配置无效", "CONFIG_INVALID", result.error)
  return { serverUrl: result.data.serverUrl.replace(/\/$/, "") }
}

/** 从 public/config.json 异步加载运行时配置。 */
export async function loadRuntimeConfig(fetcher: typeof fetch = fetch): Promise<RuntimeConfig> {
  let response: Response
  try {
    response = await fetcher("/config.json", { signal: AbortSignal.timeout(10_000) })
  } catch (error) {
    throw new RuntimeConfigError("运行时配置加载失败", "CONFIG_FETCH_FAILED", error)
  }
  if (!response.ok) throw new RuntimeConfigError("运行时配置加载失败", "CONFIG_FETCH_FAILED", response.status)
  return parseRuntimeConfig(await response.json())
}
