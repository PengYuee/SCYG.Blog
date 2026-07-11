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
    // Given / When / Then: a missing required field cannot enter the app.
    expect(() => parseRuntimeConfig({})).toThrow(RuntimeConfigError)
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
})
