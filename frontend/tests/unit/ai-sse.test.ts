import { afterEach, describe, expect, it, vi } from "vitest"
import { SseParser, streamSse, type SseFrame } from "@/request/sse"
import { configureRequestAuth } from "@/request/auth-policy"
import { parseRetryAfter, HttpRequestError } from "@/request/http-error"
import type { Problem } from "@/request/generated"
afterEach(() => { configureRequestAuth("author", () => undefined, () => undefined); configureRequestAuth("anonymous", () => undefined, () => undefined) })
describe("AI streaming boundary", () => {
  it("handles CR split across chunks, multiline data and discards unfinished frames", () => {
    const frames: SseFrame[] = []
    const parser = new SseParser((frame) => frames.push(frame))
    parser.push(": heartbeat\r")
    parser.push("\nid: opaque:9\r\ndata: first\r")
    parser.push("\ndata: second\r\n\r")
    parser.push("\ndata: unfinished")
    expect(frames).toEqual([{ data: "first\nsecond", event: "message", cursor: "opaque:9" }])
  })
  it("decodes UTF8 across byte chunks and sends runtime-origin Bearer and opaque cursor", async () => {
    configureRequestAuth("anonymous", () => "anonymous-token", () => undefined)
    const bytes = new TextEncoder().encode("id: not-a-number\ndata: 中文\n\n")
    const frames: SseFrame[] = []
    const fetcher = vi.fn(async () => new Response(new ReadableStream({ start(controller) { for (const byte of bytes) controller.enqueue(new Uint8Array([byte])); controller.close() } }), { headers: { "Content-Type": "text/event-stream" } }))
    await streamSse("https://blog.test", "/api/v1/runs/run-safe/events", "anonymous", new AbortController().signal, "opaque/old", (frame) => frames.push(frame), fetcher)
    expect(frames[0]?.data).toBe("中文")
    expect(fetcher).toHaveBeenCalledWith("https://blog.test/api/v1/runs/run-safe/events", expect.objectContaining({ credentials: "omit", headers: { Accept: "text/event-stream", Authorization: "Bearer anonymous-token", "Last-Event-ID": "opaque/old" } }))
  })
  it("preserves the configured Blog deployment path for event subscriptions", async () => {
    const fetcher = vi.fn<typeof fetch>(async () => new Response("data: complete\n\n", { headers: { "Content-Type": "text/event-stream" } }))
    await streamSse("https://blog.test/deployment", "/api/v1/runs/run-safe/events", "anonymous", new AbortController().signal, undefined, () => undefined, fetcher)
    expect(fetcher.mock.calls[0]?.[0]).toBe("https://blog.test/deployment/api/v1/runs/run-safe/events")
  })
  it("never sends credentials to a different stream origin", async () => {
    const fetcher = vi.fn()
    await expect(streamSse("https://blog.test", "https://other.test/api/v1/runs/run-safe/events", "author", new AbortController().signal, undefined, () => undefined, fetcher)).rejects.toMatchObject({ code: "INVALID_STREAM_URL" })
    expect(fetcher).not.toHaveBeenCalled()
  })
  it("normalizes SSE 429 Retry-After including zero without invalidating author", async () => {
    const authorInvalidated = vi.fn()
    configureRequestAuth("author", () => "author", authorInvalidated)
    const problem = { type: "about:blank", title: "Busy", status: 429, detail: "请稍后重试", instance: "/api/v1/runs/run-safe/events", errors: {}, code: "rate_limited", requestId: "r1" } satisfies Problem
    const fetcher = vi.fn(async () => new Response(JSON.stringify(problem), { status: 429, headers: { "Retry-After": "0" } }))
    await expect(streamSse("https://blog.test", "/api/v1/runs/run-safe/events", "anonymous", new AbortController().signal, undefined, () => undefined, fetcher)).rejects.toMatchObject({ status: 429, retryAfterSeconds: 0, requestId: "r1" })
    expect(authorInvalidated).not.toHaveBeenCalled()
  })
  it.each([undefined, null, "", "-1", "1.2", "tomorrow", "9007199254740992", " 2 "])("ignores invalid Retry-After %s", (value) => { expect(parseRetryAfter(value)).toBeUndefined() })
  it("preserves valid waiting seconds", () => { expect(parseRetryAfter("0")).toBe(0); expect(parseRetryAfter("12")).toBe(12) })
  it("a late unauthorized stream cannot expire a newer session", async () => {
    let token = "old"
    const invalidated = vi.fn()
    configureRequestAuth("author", () => token, invalidated)
    let finish!: (response: Response) => void
    const fetcher = vi.fn(() => new Promise<Response>((resolve) => { finish = resolve }))
    const request = streamSse("https://blog.test", "/api/v1/runs/run-safe/events", "author", new AbortController().signal, undefined, () => undefined, fetcher)
    token = "new"
    finish(new Response("{}", { status: 401 }))
    await expect(request).rejects.toBeInstanceOf(HttpRequestError)
    expect(invalidated).not.toHaveBeenCalled()
  })
})
