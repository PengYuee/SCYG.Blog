import type { AgentRun, AgentResumeRequest, PublicSearchSession } from "@/request/generated"
import { zAgentRun, zPublicSearchSession } from "@/request/generated/zod.gen"
import type { HttpTransport } from "@/request/transport"
import { parseBoundary } from "@/types/api"
import { streamSse, type SseFrame } from "@/request/sse"
export type AiAuth = "author" | "anonymous"
export type AiInput = Record<string, unknown>
export interface AiApi {
  session(signal?: AbortSignal): Promise<PublicSearchSession>
  create(capability: AgentRun["capability"], input: AiInput, key: string, auth: AiAuth, signal?: AbortSignal): Promise<AgentRun>
  get(runId: string, auth: AiAuth, signal?: AbortSignal): Promise<AgentRun>
  resume(runId: string, body: AgentResumeRequest, key: string, signal?: AbortSignal): Promise<AgentRun>
  cancel(runId: string, key: string, auth: AiAuth, signal?: AbortSignal): Promise<AgentRun>
  events(run: AgentRun, auth: AiAuth, signal: AbortSignal, cursor: string | undefined, emit: (frame: SseFrame) => void): Promise<void>
}
export function createAiApi(client: HttpTransport, serverUrl: string): AiApi {
  return {
    async session(signal) { return parseBoundary(zPublicSearchSession, (await client.post("/api/v1/public/ai/session", undefined, { authPolicy: "none", signal })).data, "anonymous search session") },
    async create(capability, input, key, auth, signal) {
      if (auth === "anonymous" && capability !== "search") throw new Error("匿名会话仅支持公共搜索")
      const path = auth === "anonymous" ? "/api/v1/public/ai/search" : `/api/v1/ai/${capability}`
      return parseBoundary(zAgentRun, (await client.post(path, input, { authPolicy: auth, signal, headers: { "Idempotency-Key": key } })).data, "AI Run")
    },
    async get(runId, auth, signal) { return parseBoundary(zAgentRun, (await client.get(`/api/v1/runs/${encodeURIComponent(runId)}`, { authPolicy: auth, signal })).data, "AI Run") },
    async resume(runId, body, key, signal) { return parseBoundary(zAgentRun, (await client.post(`/api/v1/runs/${encodeURIComponent(runId)}/resume`, body, { authPolicy: "author", signal, headers: { "Idempotency-Key": key } })).data, "AI Run") },
    async cancel(runId, key, auth, signal) { return parseBoundary(zAgentRun, (await client.post(`/api/v1/runs/${encodeURIComponent(runId)}/cancel`, undefined, { authPolicy: auth, signal, headers: { "Idempotency-Key": key } })).data, "AI Run") },
    events(run, auth, signal, cursor, emit) { return streamSse(serverUrl, run.streamUrl, auth, signal, cursor, emit) },
  }
}
