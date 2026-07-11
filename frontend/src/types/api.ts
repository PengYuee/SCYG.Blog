import { z } from "zod"

/** 外部 API 响应无法映射为领域数据。 */
export class ApiParseError extends Error {
  /** 稳定错误名称。 */
  readonly name = "ApiParseError"
  /** 稳定错误代码。 */
  readonly code = "API_PARSE_ERROR"

  /** 创建带边界名称和原始原因的解析错误。 */
  constructor(readonly boundary: string, cause: unknown) {
    super(`无法解析 ${boundary} 响应`, { cause })
  }
}

/** 只读分页结果。 */
export type PageResult<T> = {
  /** 当前页数据。 */
  readonly items: readonly T[]
  /** 当前页码。 */
  readonly pageIndex: number
  /** 每页数量。 */
  readonly pageSize: number
  /** 总数据量。 */
  readonly totalItems: number
  /** 总页数。 */
  readonly totalPages: number
}

/** 解析 schema 并将 Zod 失败转换为稳定边界错误。 */
export function parseBoundary<T>(schema: z.ZodType<T>, input: unknown, boundary: string): T {
  const result = schema.safeParse(input)
  if (!result.success) throw new ApiParseError(boundary, result.error)
  return result.data
}

/** 将可信相对路径或 HTTP(S) 地址归一化为绝对图片地址。 */
export function normalizeImageUrl(value: string, serverUrl: string): string {
  if (value.startsWith("//")) throw new ApiParseError("image URL", value)
  let url: URL
  try {
    url = new URL(value, `${serverUrl.replace(/\/$/, "")}/`)
  } catch (error) {
    throw new ApiParseError("image URL", error)
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") throw new ApiParseError("image URL", value)
  return url.href
}

/** 稳定的不支持功能结果。 */
export type UnsupportedResult = {
  /** 结果判别字段。 */
  readonly kind: "unsupported"
  /** 未启用的功能。 */
  readonly feature: "auth" | "search"
}
