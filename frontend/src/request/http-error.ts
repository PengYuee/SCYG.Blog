import { problemDetailsSchema, type ProblemDetails } from "@/types/api"

/** 归一化 HTTP 错误，供调用方按稳定字段处理。 */
export class HttpRequestError extends Error {
  /** 错误类型名称。 */
  readonly name = "HttpRequestError"
  /** 可选 HTTP 状态码。 */
  readonly status: number | undefined
  /** Axios 错误代码或业务回退代码。 */
  readonly code: string
  /** 已验证的 RFC 9457 问题详情。 */
  readonly problem: ProblemDetails | undefined
  /** 结构化错误中的具体说明。 */
  readonly detail: string | undefined
  /** 结构化错误中的请求追踪标识。 */
  readonly requestId: string | undefined
  /** 结构化字段错误。 */
  readonly errors: ProblemDetails["errors"] | undefined

  /** 创建包含稳定状态码、错误码、结构化问题和原始原因的 HTTP 错误。 */
  constructor(message: string, status: number | undefined, code: string, cause: unknown, problem?: ProblemDetails) {
    super(message, { cause })
    this.status = status
    this.code = code
    this.problem = problem
    this.detail = problem?.detail
    this.requestId = problem?.request_id
    this.errors = problem?.errors
  }
}

/** 仅接受完整 RFC 9457 响应，畸形错误继续走通用回退。 */
export function parseProblemDetails(input: unknown): ProblemDetails | undefined {
  const result = problemDetailsSchema.safeParse(input)
  return result.success ? result.data : undefined
}
