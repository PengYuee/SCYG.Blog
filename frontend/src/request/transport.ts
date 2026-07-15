/** 请求适配器使用的最小响应结构。 */
export type TransportResponse = {
  /** 外部响应数据，必须由领域适配器解析。 */
  readonly data: unknown
}

/** JSON 对象或由浏览器编码的 multipart 表单请求体。 */
export type TransportBody = Readonly<Record<string, unknown>> | FormData

/** 请求适配器使用的最小 HTTP transport。 */
export interface HttpTransport {
  /** 发送 GET 请求。 */
  get(url: string, config?: unknown): Promise<TransportResponse>
  /** 发送 POST 请求。 */
  post(url: string, data?: TransportBody, config?: unknown): Promise<TransportResponse>
  /** 发送 PUT 请求。 */
  put(url: string, data?: TransportBody, config?: unknown): Promise<TransportResponse>
  /** 发送 DELETE 请求。 */
  delete(url: string, config?: unknown): Promise<TransportResponse>
}
