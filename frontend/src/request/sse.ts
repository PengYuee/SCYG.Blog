import { requestToken, rejectRequestSession, type RequestAuth } from "@/request/auth-policy"
import { HttpRequestError, responseHttpError } from "@/request/http-error"
export type SseFrame = { readonly data: string; readonly event: string; readonly cursor?: string }
/** Incremental SSE parser; an incomplete final frame is intentionally discarded. */
export class SseParser {
  private line = ""
  private skipLf = false
  private data: string[] = []
  private event = "message"
  private cursor: string | undefined
  constructor(private readonly emit: (frame: SseFrame) => void) {}
  push(chunk: string): void {
    for (const character of chunk) {
      if (this.skipLf) { this.skipLf = false; if (character === "\n") continue }
      if (character === "\r" || character === "\n") {
        this.consumeLine(this.line)
        this.line = ""
        this.skipLf = character === "\r"
      } else this.line += character
    }
  }
  private consumeLine(line: string): void {
    if (line === "") {
      if (this.data.length > 0) this.emit({ data: this.data.join("\n"), event: this.event, ...(this.cursor === undefined ? {} : { cursor: this.cursor }) })
      this.data = []
      this.event = "message"
      return
    }
    if (line.startsWith(":")) return
    const separator = line.indexOf(":")
    const field = separator < 0 ? line : line.slice(0, separator)
    let value = separator < 0 ? "" : line.slice(separator + 1)
    if (value.startsWith(" ")) value = value.slice(1)
    if (field === "data") this.data.push(value)
    else if (field === "event") this.event = value
    else if (field === "id" && !value.includes("\0")) this.cursor = value
  }
}
export async function streamSse(serverUrl: string, path: string, auth: RequestAuth, signal: AbortSignal, cursor: string | undefined, emit: (frame: SseFrame) => void, fetcher: typeof fetch = fetch): Promise<void> {
  const base = new URL(serverUrl)
  const url = new URL(path.startsWith("/") ? path.slice(1) : path, `${serverUrl.replace(/\/$/, "")}/`)
  if (url.origin !== base.origin || !/^\/api\/v1\/runs\/[A-Za-z0-9._~-]{1,128}\/events$/.test(path) || url.search || url.hash) throw new HttpRequestError("事件订阅地址无效", undefined, "INVALID_STREAM_URL", undefined)
  const token = requestToken(auth)
  const headers: Record<string, string> = { Accept: "text/event-stream" }
  if (token !== undefined) headers["Authorization"] = `Bearer ${token}`
  if (cursor !== undefined && cursor !== "") headers["Last-Event-ID"] = cursor
  const response = await fetcher(url.href, { headers, signal, credentials: "omit", redirect: "error" })
  if (!response.ok) {
    if (response.status === 401) rejectRequestSession(auth, token)
    const body: unknown = await response.json().catch(() => undefined)
    throw responseHttpError(response.status, body, response.headers.get("Retry-After"))
  }
  if (!response.headers.get("Content-Type")?.split(";")[0]?.trim().toLowerCase().includes("text/event-stream") || response.body === null) throw new HttpRequestError("事件流响应格式无效", response.status, "INVALID_STREAM", undefined)
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  const parser = new SseParser(emit)
  try {
    while (true) {
      const chunk = await reader.read()
      if (chunk.done) break
      parser.push(decoder.decode(chunk.value, { stream: true }))
    }
    parser.push(decoder.decode())
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
}
