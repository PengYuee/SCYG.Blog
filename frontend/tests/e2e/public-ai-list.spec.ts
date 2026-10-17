import { expect, test, type Page } from "@playwright/test"
import type { AgentRun, Article } from "../../src/request/generated"
import { installMutationRecorder, installReadFixtures } from "./t13/fixtures/api-fixtures"
import { gotoReady } from "./t13/helpers/page-health"

const article = (id: number): Article => ({
  id, title: id === 41 ? "数据库迁移中的备份与回滚" : "PostgreSQL 迁移实践",
  slug: `search-${id}`, digest: "迁移前准备备份，在变更后核对结果，保留可恢复的操作路径。", content: "文章正文",
  articleTypeId: 1, articleType: { id: 1, name: "数据库", image: null }, tagIds: [], status: 2,
  support: 2, comment: 1, visited: 86, version: 1, createdAt: "2026-10-10T00:00:00Z", updatedAt: null,
})
const snapshot = (status: AgentRun["status"]): AgentRun => ({
  runId: "list-run", capability: "search", status, recipeId: "search-v1", recipeVersion: "v1",
  createdAt: "2026-10-10T00:00:00Z", updatedAt: "2026-10-10T00:00:00Z", streamUrl: "/api/v1/runs/list-run/events",
  pendingInteraction: null, failure: status === "failed" ? { code: "PROVIDER_ERROR", message: "UNRENDERED_FAILURE_SENTINEL" } : null,
  result: status === "succeeded" ? {
    conclusion: "UNRENDERED_CONCLUSION_SENTINEL",
    articles: [42, 41].map(id => ({ article_id: id, title: article(id).title, visibility: "published", evidence: "UNRENDERED_EVIDENCE_SENTINEL" })),
  } : null,
})

async function installRecoveryFixtures(page: Page): Promise<void> {
  await page.emulateMedia({ reducedMotion: "reduce" })
  await page.addInitScript(() => sessionStorage.setItem("scyg.ai.anonymous.session", JSON.stringify({ accessToken: "list-identity", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 })))
  await installReadFixtures(page)
  await page.route(/\/api\/v1\/articles\/(41|42)$/, route => route.fulfill({ json: article(Number(new URL(route.request().url()).pathname.split("/").at(-1))) }))
}

for (const theme of ["night", "sky"] as const) {
  test(`uses one article-list presentation from submission through completion in ${theme}`, async ({ page }, testInfo) => {
    // Given: 创建响应和完成通知都由精确事件门闩控制，不依赖固定延迟。
    await page.addInitScript(value => localStorage.setItem("scyg-blog-theme", value), theme)
    await installReadFixtures(page)
    const mutations = await installMutationRecorder(page)
    const createReceived = Promise.withResolvers<void>()
    const createResponse = Promise.withResolvers<void>()
    const streamReceived = Promise.withResolvers<void>()
    const completed = Promise.withResolvers<void>()
    const nextCreateReceived = Promise.withResolvers<void>()
    const nextCreateResponse = Promise.withResolvers<void>()
    let finished = false
    let creates = 0
    await page.route("**/api/v1/public/ai/session", route => route.fulfill({ json: { accessToken: "list-identity", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 } }))
    await page.route("**/api/v1/public/ai/search", async route => {
      expect(route.request().method()).toBe("POST")
      creates += 1
      if (creates > 1) {
        nextCreateReceived.resolve()
        await nextCreateResponse.promise
        await route.abort("blockedbyclient")
        return
      }
      createReceived.resolve()
      await createResponse.promise
      await route.fulfill({ status: 202, json: snapshot("queued") })
    })
    await page.route("**/api/v1/runs/list-run", route => route.fulfill({ json: snapshot(finished ? "succeeded" : "running") }))
    await page.route("**/api/v1/runs/list-run/events", async route => {
      streamReceived.resolve()
      await completed.promise
      await route.fulfill({ headers: { "Content-Type": "text/event-stream" }, body: 'id: done\ndata: {"event_id":"done","run_id":"list-run","kind":"completed"}\n\n' })
    })
    await page.route(/\/api\/v1\/articles\/(41|42)$/, route => route.fulfill({ json: article(Number(new URL(route.request().url()).pathname.split("/").at(-1))) }))
    try {
      await gotoReady(page, "/", page.locator("#hero-profile-name"))
      await page.getByTestId("hero-scroll-cue").click()
      const results = page.locator(".home-result-column")
      const ordinaryHeading = results.getByRole("heading", { name: "最新文章", exact: true })
      const ordinaryNote = page.getByTestId("home-announcement")
      const headerGeometry = await ordinaryHeading.boundingBox()
      const noteGeometry = await ordinaryNote.boundingBox()
      expect(headerGeometry).not.toBeNull()
      expect(noteGeometry).not.toBeNull()
      expect(noteGeometry?.y ?? 0).toBeGreaterThan((headerGeometry?.y ?? 0) + (headerGeometry?.height ?? 0))
      await page.screenshot({ path: testInfo.outputPath(`ordinary-${theme}.png`), animations: "disabled" })
      await page.locator(".home-content-search").getByRole("button", { name: "AI 搜索", exact: true }).first().click()
      await page.locator("#content-search-input").fill("数据库迁移")

      // When: 提交搜索，随后释放创建响应，再发送完成通知。
      await page.locator(".home-content-search").getByRole("button", { name: "AI 搜索", exact: true }).last().click()
      await createReceived.promise
      const submission = page.getByTestId("ai-submission-status")
      await expect(submission.locator(".search-result-spinner")).toBeVisible()
      await expect(results.locator(".ai-panel, details")).toHaveCount(0)
      await page.emulateMedia({ reducedMotion: "reduce" })
      expect(await submission.locator("svg").evaluate(element => getComputedStyle(element).animationName)).toBe("none")
      await page.emulateMedia({ reducedMotion: "no-preference" })
      expect(await submission.locator("svg").evaluate(element => getComputedStyle(element).animationName)).toBe("search-result-spin")
      await page.screenshot({ path: testInfo.outputPath(`submitting-${theme}.png`), animations: "disabled" })
      createResponse.resolve()
      await streamReceived.promise
      const status = page.getByTestId("ai-search-status")
      await expect(status.locator(".search-result-spinner")).toBeVisible()
      await expect(page.getByTestId("cancel-public-search")).toBeEnabled()
      await page.screenshot({ path: testInfo.outputPath(`running-${theme}.png`), animations: "disabled" })
      finished = true
      completed.resolve()
      const aiResults = page.getByRole("region", { name: "AI 搜索结果", exact: true })
      await expect(aiResults.locator(".article-list article")).toHaveCount(2)

      // Then: 同一层级的说明行替换执行状态，下面是普通卡片，不暴露任务内部信息。
      await expect(status).toContainText("数据库迁移")
      await expect(status.locator(".search-result-spinner")).toHaveCount(0)
      await expect(aiResults.locator(".ai-panel, details")).toHaveCount(0)
      await expect(aiResults).not.toContainText("UNRENDERED_CONCLUSION_SENTINEL")
      await expect(aiResults).not.toContainText("UNRENDERED_EVIDENCE_SENTINEL")
      const links = aiResults.locator(".article-list h3 a")
      expect(await links.evaluateAll(elements => elements.map(element => element.getAttribute("href")))).toEqual(["/articles/42", "/articles/41"])
      expect(await page.evaluate(() => {
        const ids = [...document.querySelectorAll("[id]")].map(element => element.id)
        return ids.length - new Set(ids).size
      })).toBe(0)
      const aiHeading = aiResults.getByRole("heading", { name: "最新文章", exact: true })
      const finalHeading = await aiHeading.boundingBox()
      expect(finalHeading?.y).toBe(headerGeometry?.y)
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0)
      await page.screenshot({ path: testInfo.outputPath(`completed-${theme}.png`), animations: "disabled" })
      await page.evaluate(() => window.scrollTo({ top: document.documentElement.scrollHeight, behavior: "instant" }))
      await page.screenshot({ path: testInfo.outputPath(`completed-${theme}-end.png`), animations: "disabled" })
      expect(creates).toBe(1)
      expect(mutations.requests).toEqual([])
      await page.evaluate(() => window.scrollTo({ top: 0, behavior: "instant" }))
      await page.locator("#content-search-input").fill("另一个文章主题")
      await page.locator(".home-content-search").getByRole("button", { name: "AI 搜索", exact: true }).last().click()
      await nextCreateReceived.promise
      await expect(submission.locator(".search-result-spinner")).toBeVisible()
      await expect(aiResults).toBeHidden()
      expect((await results.getByRole("heading", { name: "最新文章", exact: true }).boundingBox())?.y).toBe(headerGeometry?.y)
      await page.screenshot({ path: testInfo.outputPath(`submitting-again-${theme}.png`), animations: "disabled" })
    } finally {
      createResponse.resolve()
      completed.resolve()
      nextCreateResponse.resolve()
    }
  })
}

for (const state of ["empty", "failed"] as const) {
  test(`keeps ${state} feedback inline without exposing diagnostics`, async ({ page }) => {
    // Given: 直接恢复已结束的搜索，空结果或失败均不包含可展示文章。
    await page.addInitScript(() => sessionStorage.setItem("scyg.ai.anonymous.session", JSON.stringify({ accessToken: "list-identity", expiresAt: "2099-01-01T00:00:00Z", queryMaxChars: 2000, maxResults: 5 })))
    await installReadFixtures(page)
    await page.route("**/api/v1/runs/list-run", route => route.fulfill({ json: state === "empty" ? { ...snapshot("succeeded"), result: { conclusion: "", articles: [] } } : snapshot("failed") }))

    // When: 打开搜索结果。
    await page.goto("/?mode=ai&runId=list-run")
    const results = page.getByRole("region", { name: "AI 搜索结果", exact: true })
    await expect(page.getByTestId("ai-search-status").locator(".search-result-spinner")).toHaveCount(0)

    // Then: 保留标题和轻量状态，不生成假文章或技术详情。
    await expect(results.getByRole("heading", { name: "最新文章", exact: true })).toBeVisible()
    await expect(results.locator(".article-list article, .ai-panel, details")).toHaveCount(0)
    await expect(results).not.toContainText("UNRENDERED_FAILURE_SENTINEL")
  })
}

test("resumes the same run after restoring its cooldown", async ({ page }, testInfo) => {
  // Given: 冷却时间本身是验证对象，冻结并显式推进浏览器时钟。
  const epoch = new Date("2026-10-17T00:00:00Z")
  await page.clock.install({ time: epoch })
  await page.clock.pauseAt(new Date(epoch.getTime() + 1000))
  await installRecoveryFixtures(page)
  await page.addInitScript(() => sessionStorage.setItem("scyg.ai.anonymous.list-identity.run.list-run", JSON.stringify({ waitUntil: Date.now() + 2000, interactionDrafts: {} })))
  const mutations = await installMutationRecorder(page)
  let reads = 0
  await page.route("**/api/v1/runs/list-run", route => { reads += 1; return route.fulfill({ json: snapshot("succeeded") }) })
  await page.goto("/?mode=ai&runId=list-run")
  const retry = page.getByTestId("retry-public-search")
  await expect(retry).toBeVisible()
  await expect(retry).toBeDisabled()
  expect(reads).toBe(0)
  await page.screenshot({ path: testInfo.outputPath("restored-cooldown.png"), animations: "disabled" })

  // When: 到达冷却期限后，用户手动恢复原任务。
  await page.clock.runFor(2000)
  await expect(retry).toBeEnabled()
  const refreshed = page.waitForRequest(request => new URL(request.url()).pathname === "/api/v1/runs/list-run")
  await retry.click()
  await refreshed

  // Then: 只读取原 Run，不通过取消或创建来解锁。
  await expect(page.getByRole("region", { name: "AI 搜索结果", exact: true }).locator(".article-list article")).toHaveCount(2)
  expect(reads).toBe(1)
  expect(mutations.requests).toEqual([])
})

test("queries the snapshot after an uncertain cancellation without replaying it", async ({ page }, testInfo) => {
  // Given: 取消请求返回 503，订阅停止，结果仍需查询确认。
  await installRecoveryFixtures(page)
  let finished = false
  let cancellations = 0
  await page.route("**/api/v1/runs/list-run", route => route.fulfill({ json: snapshot(finished ? "succeeded" : "running") }))
  await page.route("**/api/v1/runs/list-run/events", route => route.fulfill({ headers: { "Content-Type": "text/event-stream" }, body: ": heartbeat\n\n" }))
  await page.route("**/api/v1/runs/list-run/cancel", route => {
    cancellations += 1
    return route.fulfill({ status: 503, json: { type: "about:blank", title: "unavailable", status: 503 } })
  })
  await page.goto("/?mode=ai&runId=list-run")
  await expect(page.getByTestId("cancel-public-search")).toBeEnabled()
  await page.getByTestId("cancel-public-search").click()
  const cancelled = page.waitForResponse(response => new URL(response.url()).pathname.endsWith("/list-run/cancel"))
  await page.getByRole("button", { name: "确认取消任务", exact: true }).click()
  await cancelled
  const query = page.getByTestId("retry-public-search")
  await expect(query).toHaveText("查询结果")
  await expect(query).toBeEnabled()
  await page.screenshot({ path: testInfo.outputPath("uncertain-cancellation.png"), animations: "disabled" })

  // When: 用户选择 GET 查询，而不是再次取消。
  finished = true
  const refreshed = page.waitForRequest(request => new URL(request.url()).pathname === "/api/v1/runs/list-run")
  await query.click()
  await refreshed

  // Then: 新快照收敛结果，取消请求始终只有一次。
  await expect(page.getByRole("region", { name: "AI 搜索结果", exact: true }).locator(".article-list article")).toHaveCount(2)
  await expect(page.getByRole("button", { name: "重试取消", exact: true })).toHaveCount(0)
  expect(cancellations).toBe(1)
})

test("keeps explicit restart available after a displayed run loses its identity", async ({ page }, testInfo) => {
  // Given: 正在展示的 Run 通过下一条通知触发身份失效。
  await installRecoveryFixtures(page)
  const mutations = await installMutationRecorder(page)
  const notified = Promise.withResolvers<void>()
  let expired = false
  await page.route("**/api/v1/runs/list-run", route => expired
    ? route.fulfill({ status: 401, json: { type: "about:blank", title: "expired", status: 401 } })
    : route.fulfill({ json: snapshot("running") }))
  await page.route("**/api/v1/runs/list-run/events", async route => {
    await notified.promise
    await route.fulfill({ headers: { "Content-Type": "text/event-stream" }, body: 'id: expiry\ndata: {"event_id":"expiry","run_id":"list-run","kind":"changed"}\n\n' })
  })
  try {
    await page.goto("/?mode=ai&runId=list-run")
    await expect(page.getByTestId("cancel-public-search")).toBeEnabled()
    const rejected = page.waitForResponse(response => new URL(response.url()).pathname === "/api/v1/runs/list-run" && response.status() === 401)

    // When: 权威读取返回 401，随后用户明确开始新的搜索。
    expired = true
    notified.resolve()
    await rejected
    const restart = page.getByRole("button", { name: "开始新的搜索", exact: true })
    await expect(restart).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath("identity-recovery.png"), animations: "disabled" })
    await restart.click()

    // Then: 旧 Run 地址被移除，新输入可用，但不会自动创建或重放旧任务。
    await expect(page).toHaveURL(url => url.searchParams.get("mode") === "ai" && !url.searchParams.has("runId"))
    await page.locator("#content-search-input").fill("新的文章主题")
    await expect(page.locator(".home-content-search").getByRole("button", { name: "AI 搜索", exact: true }).last()).toBeEnabled()
    expect(mutations.requests).toEqual([])
  } finally { notified.resolve() }
})

test("can abandon an expired unknown creation while restoring an older run", async ({ page }, testInfo) => {
  // Given: 有效旧 Run 与过期未知创建同时恢复，测试时间由浏览器时钟控制。
  await page.clock.install({ time: new Date("2026-10-17T00:00:00Z") })
  await installRecoveryFixtures(page)
  const mutations = await installMutationRecorder(page)
  await page.addInitScript(() => sessionStorage.setItem("scyg.ai.anonymous.list-identity.workspace", JSON.stringify({
    form: { query: "未知的新搜索", maxResults: 5 }, lastRun: "list-run",
    pending: { key: "12345678-1234-4234-8234-123456789012", input: { query: "未知的新搜索", max_results: 5 }, capability: "search", createdAt: Date.now() - 172800000 },
  })))
  await page.route("**/api/v1/runs/list-run", route => route.fulfill({ json: { ...snapshot("succeeded"), result: { conclusion: "", articles: [] } } }))
  await page.goto("/?mode=ai&runId=list-run")
  const abandon = page.getByTestId("abandon-public-search")
  await expect(abandon).toBeVisible()
  await page.screenshot({ path: testInfo.outputPath("expired-creation-recovery.png"), animations: "disabled" })

  // When: 用户明确确认只放弃本地未知操作。
  await abandon.click()
  await page.getByRole("button", { name: "确认放弃并开始新操作", exact: true }).click()

  // Then: 原结果恢复可见，不创建替代任务。
  await expect(page.getByRole("region", { name: "AI 搜索结果", exact: true })).toBeVisible()
  await expect(page.locator("#content-search-input")).toBeEnabled()
  await expect(page).toHaveURL(url => url.searchParams.get("runId") === "list-run")
  expect(mutations.requests).toEqual([])
})
