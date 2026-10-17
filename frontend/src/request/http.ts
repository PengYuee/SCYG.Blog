import axios from "axios"
import type { RuntimeConfig } from "@/config/runtime"
import { HttpRequestError, parseProblemDetails, parseRetryAfter } from "@/request/http-error"
import { configureRequestAuth, rejectRequestSession, requestToken, type RequestAuth } from "@/request/auth-policy"
export { HttpRequestError } from "@/request/http-error"

declare module "axios" {
  interface AxiosRequestConfig { authPolicy?: RequestAuth; sessionToken?: string | undefined }
}
export const http = axios.create({ timeout: 10_000, headers: { Accept: "application/json, application/problem+json" } })
let authTokenProvider: () => string | undefined = () => undefined
let unauthorizedHandler: () => void = () => undefined
export function configureAuthTokenProvider(provider: () => string | undefined): void {
  authTokenProvider = provider
  configureRequestAuth("author", authTokenProvider, unauthorizedHandler)
}
export function configureUnauthorizedHandler(handler: () => void): void {
  unauthorizedHandler = handler
  configureRequestAuth("author", authTokenProvider, unauthorizedHandler)
}
export function configureHttp(config: RuntimeConfig): void { http.defaults.baseURL = config.serverUrl }
export function normalizeHttpError(error: unknown): HttpRequestError {
  if (!axios.isAxiosError(error)) return new HttpRequestError("请求发生未知错误", undefined, "UNKNOWN", error)
  const data: unknown = error.response?.data
  const problem = parseProblemDetails(data)
  const responseMessage = typeof data === "object" && data !== null && "message" in data && typeof data.message === "string" && data.message.trim() !== "" ? data.message : undefined
  return new HttpRequestError(problem?.detail ?? responseMessage ?? "网络请求失败，请检查状态后重试", error.response?.status ?? problem?.status, problem?.code ?? error.code ?? "HTTP_ERROR", error, problem, parseRetryAfter(error.response?.headers?.["retry-after"]))
}
http.interceptors.request.use((config) => {
  const domain = config.authPolicy ?? "none"
  const token = requestToken(domain)
  config.sessionToken = token
  if (token === undefined) config.headers.delete("Authorization")
  else config.headers.set("Authorization", `Bearer ${token}`)
  return config
})
http.interceptors.response.use((response) => response, (error: unknown) => {
  if (axios.isAxiosError(error) && error.response?.status === 401) rejectRequestSession(error.config?.authPolicy ?? "none", error.config?.sessionToken)
  return Promise.reject(normalizeHttpError(error))
})
