import { AxiosError, AxiosHeaders } from "axios"
import { afterEach, describe, expect, it, vi } from "vitest"
import { http, configureAuthTokenProvider, configureUnauthorizedHandler, normalizeHttpError } from "@/request/http"
import { configureRequestAuth } from "@/request/auth-policy"
import type { Problem } from "@/request/generated"
afterEach(() => { configureAuthTokenProvider(() => undefined); configureUnauthorizedHandler(() => undefined); configureRequestAuth("anonymous", () => undefined, () => undefined) })
describe("explicit HTTP auth domains", () => {
  it("anonymous 401 invalidates only anonymous and none sends no Bearer", async () => {
    const authorExpired = vi.fn()
    const anonymousExpired = vi.fn()
    configureAuthTokenProvider(() => "author")
    configureUnauthorizedHandler(authorExpired)
    configureRequestAuth("anonymous", () => "anonymous", anonymousExpired)
    await expect(http.get("/api/v1/runs/run", { authPolicy: "anonymous", adapter: async (config) => {
      expect(config.headers.get("Authorization")).toBe("Bearer anonymous")
      throw new AxiosError("unauthorized", "ERR_BAD_REQUEST", config, undefined, { config, status: 401, statusText: "Unauthorized", data: {}, headers: new AxiosHeaders() })
    } })).rejects.toMatchObject({ status: 401 })
    expect(anonymousExpired).toHaveBeenCalledOnce()
    expect(authorExpired).not.toHaveBeenCalled()
    await http.post("/api/v1/public/ai/session", undefined, { authPolicy: "none", adapter: async (config) => {
      expect(config.headers.get("Authorization")).toBeUndefined()
      return { config, status: 200, statusText: "OK", data: {}, headers: new AxiosHeaders() }
    } })
  })
  it("a token replacement during an old request is not invalidated by its 401", async () => {
    let token = "old"
    const expired = vi.fn()
    configureAuthTokenProvider(() => token)
    configureUnauthorizedHandler(expired)
    await expect(http.get("/api/v1/manage/articles", { authPolicy: "author", adapter: async (config) => {
      token = "new"
      throw new AxiosError("unauthorized", "ERR_BAD_REQUEST", config, undefined, { config, status: 401, statusText: "Unauthorized", data: {}, headers: new AxiosHeaders() })
    } })).rejects.toMatchObject({ status: 401 })
    expect(expired).not.toHaveBeenCalled()
  })
  it("Axios normalizes Retry-After and publicly coded Problem", () => {
    const problem = { type: "about:blank", title: "繁忙", status: 429, detail: "等待后手动重试", instance: "/api/v1/public/ai/search", errors: {}, code: "rate_limited", requestId: "public-request" } satisfies Problem
    const error = new AxiosError("busy", "ERR_BAD_RESPONSE", undefined, undefined, { config: { headers: new AxiosHeaders() }, status: 429, statusText: "Busy", headers: new AxiosHeaders({ "retry-after": "0" }), data: problem })
    expect(normalizeHttpError(error)).toMatchObject({ status: 429, retryAfterSeconds: 0, code: "rate_limited", detail: "等待后手动重试", requestId: "public-request" })
  })
})
