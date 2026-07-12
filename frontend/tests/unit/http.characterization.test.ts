import type { AxiosError, AxiosResponse } from "axios"
import { beforeEach, describe, expect, it } from "vitest"
import { configureHttp, HttpRequestError, http, normalizeHttpError } from "@/request/http"

describe("HTTP error characterization", () => {
  beforeEach(() => {
    delete http.defaults.baseURL
  })
  it("starts without a Vite-origin API fallback", () => {
    // Given / When: 共享客户端尚未读取运行时配置。
    const baseUrl = http.defaults.baseURL

    // Then: 客户端不会静默退回 Vite 同源或 /api。
    expect(baseUrl).toBeUndefined()
  })

  it("resolves relative business APIs against RuntimeConfig.serverUrl", () => {
    // Given: 部署配置指向独立于 Vite 的后端服务。
    configureHttp({ serverUrl: "http://localhost:5000" })

    // When: Axios 解析一条相对业务接口路径。
    const requestUrl = http.getUri({ url: "/Article/GetArticleList" })

    // Then: 最终请求地址使用运行时后端而非 Vite 来源。
    expect(requestUrl).toBe("http://localhost:5000/Article/GetArticleList")
    expect(requestUrl).not.toContain("localhost:4173")
  })
  it("preserves explicitly supplied HttpRequestError fields", () => {
    // Given: a stable code, status and original cause.
    const cause = new TypeError("transport")

    // When: the request error is constructed.
    const error = new HttpRequestError("request failed", 503, "UPSTREAM", cause)

    // Then: callers can inspect every stable field.
    expect(error).toMatchObject({ name: "HttpRequestError", message: "request failed", status: 503, code: "UPSTREAM", cause })
  })

  it("normalizes an Axios response message before transport fallbacks", () => {
    // Given: Axios returned a structured backend error response.
    const response = { status: 422, data: { message: "invalid article" } } satisfies Pick<AxiosResponse, "status" | "data">
    const axiosError = {
      name: "AxiosError",
      message: "Request failed",
      code: "ERR_BAD_REQUEST",
      isAxiosError: true,
      response,
      toJSON: () => ({}),
    } satisfies Partial<AxiosError> & { readonly isAxiosError: true }

    // When: the response interceptor normalizes the rejection.
    const error = normalizeHttpError(axiosError)

    // Then: backend message, HTTP status and Axios code remain observable.
    expect(error).toMatchObject({ message: "invalid article", status: 422, code: "ERR_BAD_REQUEST" })
  })

  it("normalizes non-Axios values with the unknown error contract", () => {
    // Given: a rejection outside Axios.
    const cause = new RangeError("unexpected")

    // When: the interceptor boundary receives it.
    const error = normalizeHttpError(cause)

    // Then: it receives the stable unknown fallback.
    expect(error).toMatchObject({ message: "请求发生未知错误", status: undefined, code: "UNKNOWN", cause })
  })
})
