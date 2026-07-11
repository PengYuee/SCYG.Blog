import type { AxiosError, AxiosResponse } from "axios"
import { describe, expect, it } from "vitest"
import { HttpRequestError, normalizeHttpError } from "@/request/http"

describe("HTTP error characterization", () => {
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
