import axios from "axios"
import type { RuntimeConfig } from "@/config/runtime"

export { HttpRequestError } from "@/request/http-error"
import { HttpRequestError } from "@/request/http-error"

/** Axios 实例：统一接口根地址与 10 秒超时，不包含任何 UI 副作用。 */
export const http = axios.create({
  timeout: 10_000,
  headers: { Accept: "application/json" },
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
  const responseMessage = typeof responseData === "object" && responseData !== null && "message" in responseData && typeof responseData.message === "string" ? responseData.message : undefined
  const message = responseMessage ?? error.message ?? "网络请求失败"
  return new HttpRequestError(message, error.response?.status, error.code ?? "HTTP_ERROR", error)
}

http.interceptors.response.use(
  /**
   * 直接返回成功响应。
   * @param response Axios 成功响应。
   * @returns 原始成功响应。
   * @throws 此回调不抛出异常。
   * @since 1.0.0
   */
  (response) => response,
  /**
   * 将未知失败归一化为 HttpRequestError。
   * @param error 未知错误值。
   * @returns 始终拒绝的 Promise。
   * @throws 通过 Promise 拒绝传播 HttpRequestError。
   * @since 1.0.0
   */
  (error: unknown) => {
    return Promise.reject(normalizeHttpError(error))
  },
)
