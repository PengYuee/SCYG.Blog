import { test, expect } from "@playwright/test"
import type { AgentRun, Article } from "../../src/request/generated"
import { installReadFixtures } from "./t13/fixtures/api-fixtures"

declare global {
  interface Window { scygPublicDialogFocus?: Promise<void> }
}
const run: AgentRun = { runId: "edge-run", capability: "search", status: "succeeded", recipeId: "search-v1", recipeVersion: "v1", createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: "/api/v1/runs/edge-run/events", result: { conclusion: "公开搜索结论", articles: [{ article_id: 1, title: "公开证据文章", visibility: "published", evidence: "真实契约下的证据片段" }] }, failure: null, pendingInteraction: null }
const searchArticle: Article = { id: 1, title: "公开证据文章", slug: "public-source", digest: "公开文章摘要", content: "公开正文", articleTypeId: 1, articleType: { id: 1, name: "前端工程", image: null }, tagIds: [], status: 2, support: 0, comment: 0, visited: 1, version: 1, createdAt: "2026-10-10T00:00:00Z", updatedAt: null }
for (const kind of ["cancel", "abandon"] as const) {
  test(`public ${kind} confirmation returns keyboard focus after Escape`, async ({ page }) => {
    // Given: 一个有效匿名身份和可取消任务，或已过期的未知创建操作。
    await page.emulateMedia({ reducedMotion: kind === "cancel" ? "no-preference" : "reduce" })
    await page.addInitScript((scenario) => {
      sessionStorage.setItem("scyg.ai.anonymous.session", JSON.stringify({ accessToken: "focus-identity", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 }))
      if (scenario === "abandon") sessionStorage.setItem("scyg.ai.anonymous.focus-identity.workspace", JSON.stringify({
        form: { query: "未确认问题", maxResults: 5 },
        pending: { key: "12345678-1234-4234-8234-123456789012", input: { query: "未确认问题", max_results: 5 }, capability: "search", createdAt: Date.now() - 86400001 },
      }))
    }, kind)
    await page.route("**/config.json", (route) => route.fulfill({ json: { serverUrl: "http://ai-fixture.test" } }))
    await installReadFixtures(page)
    await page.route("http://ai-fixture.test/api/v1/runs/edge-run", (route) => route.fulfill({ json: { ...run, status: "queued", result: null } }))
    await page.route("http://ai-fixture.test/api/v1/runs/edge-run/events", (route) => route.fulfill({ headers: { "Content-Type": "text/event-stream" }, body: ": heartbeat\n\n" }))
    const writes: string[] = []
    page.on("request", (request) => { if (request.method() !== "GET") writes.push(new URL(request.url()).pathname) })
    await page.goto(kind === "cancel" ? "/?mode=ai&runId=edge-run" : "/?mode=ai")
    const id = kind === "cancel" ? "cancel-public-search" : "abandon-public-search"
    const trigger = page.getByTestId(id)
    await expect(trigger).toBeEnabled()
    await trigger.focus()
    await trigger.press("Enter")
    await expect(page.getByRole("button", { name: "关闭弹窗" })).toBeFocused()
    await page.evaluate((testId) => {
      const opener = document.querySelector<HTMLElement>(`[data-testid="${testId}"]`)
      if (opener === null) throw new Error("Missing confirmation opener")
      window.scygPublicDialogFocus = new Promise<void>((resolve, reject) => {
        const focused = (): void => { clearTimeout(timeout); resolve() }
        const timeout = setTimeout(() => { opener.removeEventListener("focus", focused); reject(new Error("Confirmation focus was not restored")) }, 5000)
        opener.addEventListener("focus", focused, { once: true })
      })
    }, id)
    // When: 通过 Escape 关闭确认。
    await page.keyboard.press("Escape")
    await page.evaluate(async () => {
      if (window.scygPublicDialogFocus === undefined) throw new Error("Missing focus receipt")
      await window.scygPublicDialogFocus
    })
    // Then: 焦点回到开启按钮，且没有取消任务或重新创建搜索。
    await expect(trigger).toBeFocused()
    await expect(page.getByRole("dialog")).toHaveCount(0)
    expect(writes).toEqual([])
  })
}
for (const theme of ["night", "sky"] as const) {
  test(`public search remains independent of author login (${theme})`, async ({ page }) => {
    await page.addInitScript((selected) => {
      localStorage.setItem("scyg-blog-theme", selected)
      localStorage.setItem("scyg.auth.session", JSON.stringify({ accessToken: "author-token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" }))
    }, theme)
    await page.route("**/config.json", (route) => route.fulfill({ json: { serverUrl: "http://ai-fixture.test" } }))
    let created = 0
    await page.route("http://ai-fixture.test/**", async (route) => {
      const request = route.request()
      const path = new URL(request.url()).pathname
      if (path === "/api/v1/public/ai/session") {
        expect(request.headers()["authorization"]).toBeUndefined()
        await route.fulfill({ json: { accessToken: "anonymous-token", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 } }); return
      }
      if (path === "/api/v1/public/ai/search") {
        created += 1
        expect(request.headers()["authorization"]).toBe("Bearer anonymous-token")
        expect(request.headers()["idempotency-key"]).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i)
        expect(request.postDataJSON()).toEqual({ query: "查找公开文章", max_results: 5 })
        await route.fulfill({ status: 202, json: run }); return
      }
      if (path === "/api/v1/runs/edge-run") {
        expect(request.headers()["authorization"]).toBe("Bearer anonymous-token")
        await route.fulfill({ json: run }); return
      }
      if (path === "/api/v1/articles/1") {
        expect(request.headers()["authorization"]).toBeUndefined()
        await route.fulfill({ json: searchArticle }); return
      }
      await route.fulfill({ status: 404, json: { type: "about:blank", title: "不存在", status: 404 } })
    })
    await page.goto("/ai/search")
    await page.locator("#content-search-input").fill("查找公开文章")
    await page.locator(".home-content-search").getByRole("button", { name: "AI 搜索", exact: true }).last().click()
    await expect(page).toHaveURL(/\/\?mode=ai&runId=edge-run$/)
    await expect(page.getByTestId("ai-search-status")).toContainText("查找公开文章")
    await expect(page.getByRole("region", { name: "AI 搜索结果", exact: true }).locator(".article-list article")).toHaveCount(1)
    await expect(page.locator(".home-result-column .ai-panel")).toHaveCount(0)
    await expect(page.getByRole("link", { name: "公开证据文章" })).toHaveAttribute("href", "/articles/1")
    await page.locator("#content-search-input").fill("未提交的新主题")
    await page.reload()
    await expect(page.getByTestId("ai-search-status")).toContainText("查找公开文章")
    await expect(page.locator("#content-search-input")).toHaveValue("未提交的新主题")
    await expect(page.getByRole("link", { name: "公开证据文章" })).toHaveAttribute("href", "/articles/1")
    expect(created).toBe(1)
    expect(await page.evaluate(() => JSON.parse(localStorage.getItem("scyg.auth.session") ?? "null").accessToken)).toBe("author-token")
  })
}
test("author writing preparation stages preserve draft on reload and submit real payloads", async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem("scyg.auth.session", JSON.stringify({ accessToken: "author-token", tokenType: "Bearer", expiresAt: "2099-01-01T00:00:00Z" })))
  await page.route("**/config.json", (route) => route.fulfill({ json: { serverUrl: "http://ai-fixture.test" } }))
  let phase = 0
  const waiting = (): AgentRun => ({ ...run, capability: "write", status: "waitingForApproval", result: null, pendingInteraction: phase === 0 ? { interactionId: "text-stage", type: "textInput", payload: { title: "补充要求", prompt: "请说明读者", min_length: 1, max_length: 16000 } } : phase === 1 ? { interactionId: "selection-stage", type: "selection", payload: { title: "选择参考", prompt: "多选文章", min_selections: 0, max_selections: 2, options: [{ article_id: 1, title: "候选一", digest: "摘要一", status: "published" }, { article_id: 2, title: "候选二", digest: "摘要二", status: "published" }] } } : { interactionId: "confirmation-stage", type: "confirmation", payload: { title: "待确认草稿", outline: "草稿大纲" } } })
  await page.route("http://ai-fixture.test/**", async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    if (path.endsWith("/events")) { await route.fulfill({ headers: { "Content-Type": "text/event-stream" }, body: ": heartbeat\n\n" }); return }
    if (path.endsWith("/resume")) {
      const body = request.postDataJSON()
      if (phase === 0) expect(body).toEqual({ interactionId: "text-stage", decision: "submit", payload: "给新手看的教程" })
      else if (phase === 1) expect(body).toEqual({ interactionId: "selection-stage", decision: "select", payload: { article_ids: [] } })
      else expect(body).toEqual({ interactionId: "confirmation-stage", decision: "approve" })
      phase += 1
      await route.fulfill({ json: phase < 3 ? waiting() : { ...run, capability: "write", result: { title: "生成标题", outline: "最终大纲", markdown: "未保存正文", source_article_ids: [] } } }); return
    }
    if (path === "/api/v1/runs/edge-run") { await route.fulfill({ json: phase < 3 ? waiting() : { ...run, capability: "write", result: { title: "生成标题", outline: "最终大纲", markdown: "未保存正文", source_article_ids: [] } } }); return }
    await route.fulfill({ status: 404 })
  })
  await page.goto("/author/ai/runs/edge-run")
  await page.getByLabel("补充写作要求").fill("给新手看的教程")
  await page.reload()
  await expect(page.getByLabel("补充写作要求")).toHaveValue("给新手看的教程")
  await page.getByRole("button", { name: "提交补充要求" }).click()
  await page.getByRole("button", { name: "提交参考选择（0 篇）" }).click()
  await page.getByRole("button", { name: "确认继续" }).click()
  await expect(page.getByText("未保存正文", { exact: true })).toBeVisible()
  await expect(page.getByText("这是尚未保存的生成草稿。复制不会新建、覆盖或发布文章。")).toBeVisible()
})
