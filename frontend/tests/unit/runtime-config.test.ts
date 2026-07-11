import { describe, expect, it, vi } from "vitest"
import { RuntimeConfigError, loadRuntimeConfig, parseRuntimeConfig } from "@/config/runtime"

describe("runtime configuration boundary", () => {
  it("parses an HTTPS server URL", () => {
    // Given / When: current public config shape crosses the boundary.
    const config = parseRuntimeConfig({ serverUrl: "https://api.example.test/" })
    // Then: the validated URL is stable without a trailing slash.
    expect(config.serverUrl).toBe("https://api.example.test")
  })

  it("rejects a missing serverUrl with a typed failure", () => {
    // Given / When: a required field is missing.
    const error = captureConfigError(() => parseRuntimeConfig({}))
    // Then: the boundary returns the stable invalid code.
    expect(error.code).toBe("CONFIG_INVALID")
  })

  it("rejects a non-HTTP serverUrl with CONFIG_INVALID", () => {
    // Given / When: a non-HTTP URL crosses the boundary.
    const error = captureConfigError(() => parseRuntimeConfig({ serverUrl: "ftp://api.example.test" }))
    // Then: it is a configuration parse failure.
    expect(error.code).toBe("CONFIG_INVALID")
  })

  it("loads config asynchronously from the public runtime file", async () => {
    // Given: the runtime endpoint returns the deployed configuration.
    const fetcher = vi.fn(async () => new Response('{"serverUrl":"http://localhost:5000"}'))
    // When: configuration is loaded.
    const config = await loadRuntimeConfig(fetcher)
    // Then: the fixed public path and parsed value are used.
    expect(fetcher).toHaveBeenCalledWith("/config.json", expect.objectContaining({ signal: expect.any(AbortSignal) }))
    expect(config.serverUrl).toBe("http://localhost:5000")
  })

  it("converts malformed JSON to CONFIG_INVALID instead of SyntaxError", async () => {
    // Given: the runtime endpoint returns malformed JSON.
    const fetcher = vi.fn(async () => new Response("{"))
    // When: configuration is loaded.
    const error = await captureAsyncConfigError(() => loadRuntimeConfig(fetcher))
    // Then: raw JSON errors never escape the typed boundary.
    expect(error).toMatchObject({ name: "RuntimeConfigError", code: "CONFIG_INVALID" })
  })

  it("rejects non-object JSON with CONFIG_INVALID", async () => {
    // Given / When: valid JSON has the wrong top-level shape.
    const error = await captureAsyncConfigError(() => loadRuntimeConfig(async () => new Response('"wrong"')))
    // Then: the typed invalid code is stable.
    expect(error.code).toBe("CONFIG_INVALID")
  })

  it("keeps fetch rejection as CONFIG_FETCH_FAILED", async () => {
    // Given / When: transport rejects before a response exists.
    const error = await captureAsyncConfigError(() => loadRuntimeConfig(async () => Promise.reject(new TypeError("offline"))))
    // Then: transport and parse failures remain distinct.
    expect(error.code).toBe("CONFIG_FETCH_FAILED")
  })

  it("keeps non-2xx responses as CONFIG_FETCH_FAILED", async () => {
    // Given / When: the runtime endpoint responds with an error status.
    const error = await captureAsyncConfigError(() => loadRuntimeConfig(async () => new Response("missing", { status: 404 })))
    // Then: status failures retain the fetch code.
    expect(error.code).toBe("CONFIG_FETCH_FAILED")
  })
})

/** 捕获同步运行时配置错误。 */
function captureConfigError(action: () => unknown): RuntimeConfigError {
  try {
    action()
  } catch (error) {
    if (error instanceof RuntimeConfigError) return error
    throw error
  }
  throw new Error("Expected RuntimeConfigError")
}

/** 捕获异步运行时配置错误。 */
async function captureAsyncConfigError(action: () => Promise<unknown>): Promise<RuntimeConfigError> {
  try {
    await action()
  } catch (error) {
    if (error instanceof RuntimeConfigError) return error
    throw error
  }
  throw new Error("Expected RuntimeConfigError")
}
