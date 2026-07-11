import { expect, test } from "@playwright/test"
import { installReadFixtures } from "./fixtures/api-fixtures"
import { expectCjkUnclipped, expectDesktopGeometry } from "./helpers/layout"
import { evidencePath, expectHealthy, gotoReady, observePageHealth, settleVisualState } from "./helpers/page-health"

const navigationCeilingMs = 10_000
const requestCeiling = 30

/** 记录资源请求总数，作为比 Lighthouse 更稳定的本地性能预算。 */
test("三个 Edge 桌面视口满足布局、性能与无障碍契约", async ({ page }) => {
  // Given: 当前 project 提供精确视口，全部读取在导航前被固定。
  const health = observePageHealth(page)
  let requestCount = 0
  page.on("request", () => { requestCount += 1 })
  await installReadFixtures(page)

  // When: 首页进入确定性 ready 状态。
  const startedAt = Date.now()
  await gotoReady(page, "/", "[data-testid='article-grid']")
  const readyElapsedMs = Date.now() - startedAt

  // Then: T6 几何、横向边界、中文排版和宽松导航预算全部稳定。
  await expectDesktopGeometry(page)
  await expectCjkUnclipped(page.locator("h1, h2, button").filter({ visible: true }))
  expect(readyElapsedMs).toBeLessThan(navigationCeilingMs)
  expect(requestCount).toBeLessThanOrEqual(requestCeiling)

  // Then: 仅依赖原生角色、标签和焦点顺序验证键盘可达性。
  await expect(page.getByRole("navigation", { name: "主要导航" })).toBeVisible()
  await expect(page.getByRole("search", { name: "搜索文章" })).toBeVisible()
  await expect(page.getByLabel("搜索文章")).toBeVisible()
  await page.keyboard.press("Tab")
  await expect(page.getByRole("link", { name: "跳到主要内容" })).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(page.locator("#blog-content")).toBeFocused()
  expectHealthy(health)
})

test("保存 Hero、滚动导航、悬停与焦点的稳定视觉状态", async ({ page }, testInfo) => {
  // Given: 减少动态效果消除非确定性，读取 fixture 在导航前生效。
  const health = observePageHealth(page)
  await page.emulateMedia({ reducedMotion: "reduce" })
  await installReadFixtures(page)
  await gotoReady(page, "/", "[data-testid='article-grid']")
  await settleVisualState(page)

  // When/Then: 分别捕获 Hero 静止态、滚动导航、卡片悬停和搜索焦点。
  await page.screenshot({ path: evidencePath(testInfo, "hero-rest.png"), fullPage: false, animations: "disabled" })
  await page.locator("#blog-content").scrollIntoViewIfNeeded()
  await expect(page.locator("header[data-state]")).toHaveAttribute("data-state", "scrolled")
  await page.screenshot({ path: evidencePath(testInfo, "nav-scrolled.png"), fullPage: false, animations: "disabled" })
  const firstArticle = page.locator("[data-testid='article-grid'] a").first()
  await firstArticle.hover()
  await page.screenshot({ path: evidencePath(testInfo, "article-hover.png"), fullPage: false, animations: "disabled" })
  await page.getByLabel("搜索文章").focus()
  await expect(page.getByLabel("搜索文章")).toBeFocused()
  await page.screenshot({ path: evidencePath(testInfo, "search-focus.png"), fullPage: false, animations: "disabled" })
  expectHealthy(health)
})
