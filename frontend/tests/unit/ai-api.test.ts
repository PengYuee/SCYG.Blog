import { describe, expect, it, vi } from "vitest"
import { createAiApi } from "@/request/api/ai"
import type { AgentRun } from "@/request/generated"
import type { HttpTransport } from "@/request/transport"
const run: AgentRun = { runId: "run-safe", status: "queued", capability: "search", recipeId: "search-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", failure: null, result: null, pendingInteraction: null, streamUrl: "/api/v1/runs/run-safe/events" }
function transport(): HttpTransport { return { get: vi.fn(async () => ({ data: run })), post: vi.fn(async () => ({ data: run })), patch: vi.fn(), delete: vi.fn() } }
describe("AI API consumer boundary", () => {
  it("public create preserves snake_case and uses anonymous auth", async () => {
    const client = transport()
    const key = "61e6f420-41c1-42c3-8d36-d8a0a5f203f3"
    await createAiApi(client, "https://blog.test").create("search", { query: "公开搜索", max_results: 3 }, key, "anonymous")
    expect(client.post).toHaveBeenCalledWith("/api/v1/public/ai/search", { query: "公开搜索", max_results: 3 }, expect.objectContaining({ authPolicy: "anonymous", headers: { "Idempotency-Key": key } }))
  })
  it("cancel sends no body; resume preserves omission and explicit null", async () => {
    const client = transport()
    const api = createAiApi(client, "https://blog.test")
    await api.cancel(run.runId, "cancel-key", "anonymous")
    await api.resume(run.runId, { interactionId: "confirm", decision: "approve" }, "first-key")
    await api.resume(run.runId, { interactionId: "input", decision: "submit", payload: null }, "second-key")
    expect(client.post).toHaveBeenNthCalledWith(1, "/api/v1/runs/run-safe/cancel", undefined, expect.objectContaining({ authPolicy: "anonymous" }))
    expect(client.post).toHaveBeenNthCalledWith(2, "/api/v1/runs/run-safe/resume", { interactionId: "confirm", decision: "approve" }, expect.objectContaining({ authPolicy: "author" }))
    expect(client.post).toHaveBeenNthCalledWith(3, "/api/v1/runs/run-safe/resume", { interactionId: "input", decision: "submit", payload: null }, expect.objectContaining({ authPolicy: "author" }))
  })
  it("invalid snapshots do not enter business state", async () => {
    const client = transport()
    vi.mocked(client.get).mockResolvedValue({ data: { ...run, status: "imagined-success" } })
    await expect(createAiApi(client, "https://blog.test").get(run.runId, "author")).rejects.toMatchObject({ code: "API_PARSE_ERROR" })
  })
})
