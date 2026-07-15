import { describe, expect, it, vi } from "vitest"
import { createApiServices } from "@/request/api-services"
import type { HttpTransport } from "@/request/transport"
import { AuthorRuntimeUnavailableError, createAuthorRuntime } from "@/services/author-runtime"

/** 创建不会执行网络请求的完整 transport。 */
function transport(): HttpTransport {
  return { get: vi.fn(), post: vi.fn(), put: vi.fn(), delete: vi.fn() }
}

describe("author runtime environment boundary", () => {
  it("refuses to impersonate an author outside explicit development mode", () => {
    // Given: 当前 Vitest 非 development 环境与完整真实 API 容器。
    const services = createApiServices(transport(), "https://api.test")
    // When / Then: 运行时在接触任何 API 前以中文类型化错误拒绝固定身份。
    expect(() => createAuthorRuntime(services)).toThrow(AuthorRuntimeUnavailableError)
    expect(() => createAuthorRuntime(services)).toThrow("当前环境未启用可信作者运行时")
  })
})
