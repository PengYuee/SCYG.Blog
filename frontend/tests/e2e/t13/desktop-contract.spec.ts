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
  const latestGrid = page.getByRole("heading", { name: "最新文章", level: 2 }).locator("xpath=ancestor::section").getByTestId("article-grid")
  await gotoReady(page, "/", latestGrid)
  const readyElapsedMs = Date.now() - startedAt

  // Then: T6 几何、横向边界、中文排版和宽松导航预算全部稳定。
  await expectDesktopGeometry(page)
  await expectCjkUnclipped(page.locator("h1, h2, button, input, select").filter({ visible: true }))
  expect(readyElapsedMs).toBeLessThan(navigationCeilingMs)
  expect(requestCount).toBeLessThanOrEqual(requestCeiling)

  // Then: 仅依赖原生角色、标签和焦点顺序验证键盘可达性。
  await expect(page.getByRole("navigation", { name: "主要导航" })).toBeVisible()
  // 两个搜索 landmark 共享合法名称，使用各自唯一输入反向限定语义归属。
  const heroSearch = page.locator("#hero-search-input")
  const articleSearch = page.locator("#article-search-input")
  await expect(page.getByRole("search", { name: "搜索文章" }).filter({ has: heroSearch })).toBeVisible()
  await expect(page.getByRole("search", { name: "搜索文章" }).filter({ has: articleSearch })).toBeVisible()
  await expect(heroSearch).toBeVisible()
  await expect(articleSearch).toBeVisible()
  await page.keyboard.press("Tab")
  await expect(page.getByRole("link", { name: "跳到主要内容" })).toBeFocused()
  await page.keyboard.press("Enter")
  const content = page.locator("#blog-content")
  await expect(content).toBeFocused()
  // 固定导航下的主内容锚点必须完整露出。
  const anchorGeometry = await page.evaluate(() => {
    const header = document.querySelector<HTMLElement>("header[data-state]")
    const target = document.querySelector<HTMLElement>("#blog-content")
    if (header === null || target === null) return null
    return { headerBottom: header.getBoundingClientRect().bottom, targetTop: target.getBoundingClientRect().top }
  })
  expect(anchorGeometry).not.toBeNull()
  expect(anchorGeometry?.targetTop).toBeGreaterThanOrEqual(anchorGeometry?.headerBottom ?? Number.POSITIVE_INFINITY)
  expectHealthy(health)
})

test("保存 Hero、滚动导航、悬停与焦点的稳定视觉状态", async ({ page }, testInfo) => {
  // Given: 减少动态效果消除非确定性，读取 fixture 在导航前生效。
  const health = observePageHealth(page)
  await page.emulateMedia({ reducedMotion: "reduce" })
  await installReadFixtures(page)
  const latestGrid = page.getByRole("heading", { name: "最新文章", level: 2 }).locator("xpath=ancestor::section").getByTestId("article-grid")
  await gotoReady(page, "/", latestGrid)
  await settleVisualState(page)

  // When/Then: 分别捕获 Hero 静止态、滚动导航、卡片悬停和搜索焦点。
  await page.screenshot({ path: evidencePath(testInfo, "hero-rest.png"), fullPage: false, animations: "disabled" })
  await page.locator("#blog-content").scrollIntoViewIfNeeded()
  await expect(page.locator("header[data-state]")).toHaveAttribute("data-state", "scrolled")
  await page.screenshot({ path: evidencePath(testInfo, "nav-scrolled.png"), fullPage: false, animations: "disabled" })
  // 最新文章区域包含唯一网格；其中首个链接是明确的悬停目标。
  const firstArticle = page.getByRole("heading", { name: "最新文章", level: 2 }).locator("xpath=ancestor::section").getByTestId("article-grid").getByRole("link").first()
  await firstArticle.hover()
  await page.screenshot({ path: evidencePath(testInfo, "article-hover.png"), fullPage: false, animations: "disabled" })
  // 下方文章搜索框拥有独立 ID，避免与 Hero 搜索框的同名标签冲突。
  const articleSearch = page.locator("#article-search-input")
  await articleSearch.focus()
  await expect(articleSearch).toBeFocused()
  await page.screenshot({ path: evidencePath(testInfo, "search-focus.png"), fullPage: false, animations: "disabled" })
  expectHealthy(health)
})
