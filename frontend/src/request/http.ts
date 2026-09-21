import axios from "axios"
import type { RuntimeConfig } from "@/config/runtime"

export { HttpRequestError } from "@/request/http-error"
import { HttpRequestError, parseProblemDetails } from "@/request/http-error"

/** Axios 实例：统一接口根地址与 10 秒超时，不包含任何 UI 副作用。 */
export const http = axios.create({
  timeout: 10_000,
  headers: { Accept: "application/json, application/problem+json" },
})

/** 使用已解析的运行时配置初始化共享 HTTP 客户端。 */
export function configureHttp(config: RuntimeConfig): void {
  http.defaults.baseURL = config.serverUrl
}

/**
 * 将未知 HTTP 失败归一化为稳定错误。
 * @param error 未知错误值。
 * @returns 可供调用方检查的 HttpRequestError。
 */
export function normalizeHttpError(error: unknown): HttpRequestError {
  if (!axios.isAxiosError(error)) {
    return new HttpRequestError("请求发生未知错误", undefined, "UNKNOWN", error)
  }
  const responseData: unknown = error.response?.data
  const problem = parseProblemDetails(responseData)
  const problemDetail = problem?.detail
  const responseMessage = typeof responseData === "object" && responseData !== null && "message" in responseData && typeof responseData.message === "string" && responseData.message.trim().length > 0 ? responseData.message : undefined
  const message = problemDetail ?? responseMessage ?? error.message ?? "网络请求失败"
  return new HttpRequestError(message, error.response?.status ?? problem?.status, error.code ?? "HTTP_ERROR", error, problem)
}

http.interceptors.response.use(
  /** 直接返回成功响应。 */
  (response) => response,
  /** 将未知失败归一化为 HttpRequestError。 */
  (error: unknown) => Promise.reject(normalizeHttpError(error)),
)
