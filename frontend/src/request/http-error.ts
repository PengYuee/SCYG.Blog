/** 归一化 HTTP 错误，供调用方按稳定字段处理。 */
export class HttpRequestError extends Error {
  /** 错误类型名称。 */
  readonly name = "HttpRequestError"
  /** 可选 HTTP 状态码。 */
  readonly status: number | undefined
  /** Axios 错误代码或业务回退代码。 */
  readonly code: string

  /** 创建包含稳定状态码、错误码和原始原因的 HTTP 错误。 */
  constructor(message: string, status: number | undefined, code: string, cause: unknown) {
    super(message, { cause })
    this.status = status
    this.code = code
  }
}
