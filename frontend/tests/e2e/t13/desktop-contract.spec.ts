import { expect, test } from "@playwright/test"
import { installReadFixtures } from "./fixtures/api-fixtures"
import { expectCjkUnclipped, expectDesktopGeometry } from "./helpers/layout"
import { evidencePath, expectHealthy, gotoReady, observePageHealth, settleVisualState, waitForHeroBackground } from "./helpers/page-health"

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
  await gotoReady(page, "/", page.locator("#hero-profile-name"))
  const readyElapsedMs = Date.now() - startedAt

  // Then: T6 几何、横向边界、中文排版和宽松导航预算全部稳定。
  await expectDesktopGeometry(page)
  await expectCjkUnclipped(page.locator("h1, h2, button, input, select").filter({ visible: true }))
  expect(readyElapsedMs).toBeLessThan(navigationCeilingMs)
  expect(requestCount).toBeLessThanOrEqual(requestCeiling)

  // Then: 仅依赖原生角色、标签和焦点顺序验证键盘可达性。
  await page.getByRole("banner").getByRole("button", { name: "主页", exact: true }).click()
  await expect(page.locator(".public-hero")).toBeVisible()
  await expect(page.getByRole("banner").getByRole("navigation", { name: "主要导航" })).toBeVisible()
  // 两个搜索 landmark 共享合法名称，使用各自唯一输入反向限定语义归属。
  const heroSearch = page.locator("#hero-search-input")
  const articleSearch = page.locator("#content-search-input")
  await expect(page.locator("header[data-state]")).toHaveAttribute("data-state", "transparent")
  await expect(page.getByRole("banner").getByRole("navigation").getByRole("button", { name: "主页", exact: true })).toBeVisible()
  await expect(page.getByRole("banner").getByRole("search")).toHaveCount(0)
  await expect(page.getByRole("search", { name: "站内搜索" }).filter({ has: heroSearch })).toBeVisible()
  await expect(heroSearch).toBeVisible()
  await expect(articleSearch).toHaveCount(1)
  await expect(articleSearch).toBeHidden()
  await page.getByRole("link", { name: "跳到主要内容" }).focus()
  await expect(page.getByRole("link", { name: "跳到主要内容" })).toBeFocused()
  await page.keyboard.press("Enter")
  const content = page.locator("#blog-content")
  await expect(content).toBeFocused()
  await expect(page).toHaveURL((url) => url.pathname === "/" && url.hash === "")
  await expect(page.locator(".public-hero")).toBeHidden()
  await expect(content).toBeVisible()
  // 固定导航下的主内容锚点必须完整露出。
  const anchorGeometry = await page.evaluate(() => {
    const header = document.querySelector<HTMLElement>("header[data-state]")
    const target = document.querySelector<HTMLElement>("#blog-content")
    if (header === null || target === null) return null
    return { headerBottom: header.getBoundingClientRect().bottom, targetTop: target.firstElementChild?.getBoundingClientRect().top ?? 0 }
  })
  expect(anchorGeometry).not.toBeNull()
  expect(anchorGeometry?.targetTop).toBeGreaterThanOrEqual(anchorGeometry?.headerBottom ?? Number.POSITIVE_INFINITY)
  await expect(page.getByRole("search", { name: "站内搜索" }).filter({ has: articleSearch })).toBeVisible()
  expectHealthy(health)
})

test("保存 Hero、滚动导航、悬停与焦点的稳定视觉状态", async ({ page }, testInfo) => {
  // Given: 减少动态效果消除非确定性，读取 fixture 在导航前生效。
  const health = observePageHealth(page)
  await page.emulateMedia({ reducedMotion: "reduce" })
  await installReadFixtures(page)
  await gotoReady(page, "/", page.locator("#hero-profile-name"))
  await settleVisualState(page)

  // When/Then: 分别捕获 Hero 静止态、滚动导航、卡片悬停和搜索焦点。
  await page.screenshot({ path: evidencePath(testInfo, "hero-rest.png"), fullPage: false, animations: "disabled" })
  await page.getByTestId("hero-scroll-cue").click()
  await expect(page.locator(".public-hero")).toBeHidden()
  await expect(page.locator("header[data-state]")).toHaveAttribute("data-state", "scrolled")
  await page.screenshot({ path: evidencePath(testInfo, "nav-scrolled.png"), fullPage: false, animations: "disabled" })
  // 最新文章区域包含唯一网格；其中首个链接是明确的悬停目标。
  const firstArticle = page.getByRole("heading", { name: "最新文章", level: 2 }).locator("xpath=ancestor::section").getByTestId("article-grid").getByRole("link").first()
  await firstArticle.hover()
  await page.screenshot({ path: evidencePath(testInfo, "article-hover.png"), fullPage: false, animations: "disabled" })
  // 下方文章搜索框拥有独立 ID，避免与 Hero 搜索框的同名标签冲突。
  const articleSearch = page.locator("#content-search-input")
  await articleSearch.focus()
  await expect(articleSearch).toBeFocused()
  await page.screenshot({ path: evidencePath(testInfo, "search-focus.png"), fullPage: false, animations: "disabled" })
  expectHealthy(health)
})

for (const reducedMotion of ["no-preference", "reduce"] as const) {
  test(`Hero 与键盘锚点保持各自终态并恢复阅读位置 (${reducedMotion})`, async ({ page }) => {
    const health = observePageHealth(page)
    await page.emulateMedia({ reducedMotion })
    await installReadFixtures(page)
    const content = page.locator("#blog-content")
    await gotoReady(page, "/", page.locator("#hero-profile-name"))

    // Hero 下滑仍精确移除整屏 Hero，不写入 hash；正文内边距避让导航。
    await page.getByRole("button", { name: "进入内容区", exact: true }).click()
    await expect(page.locator(".public-hero")).toBeHidden()
    await expect(content).toBeVisible()
    expect(await page.evaluate(() => window.scrollY)).toBe(0)
    await expect(page).toHaveURL((url) => url.pathname === "/" && url.hash === "")
    expect(await content.evaluate((target) => {
      const header = document.querySelector<HTMLElement>("header[data-state]")
      const body = target.firstElementChild
      return header !== null && body !== null && body.getBoundingClientRect().top >= header.getBoundingClientRect().bottom
    })).toBe(true)
    const readingPosition = await page.evaluate(() => window.scrollY)

    // 刷新恢复当前阅读位置，普通 SPA 导航回顶部，后退恢复阅读位置。
    await page.reload()
    await expect(content).toBeVisible()
    await expect(page.locator(".public-hero")).toBeHidden()
    await waitForHeroBackground(page)
    expect(await page.evaluate(() => window.scrollY)).toBe(readingPosition)
    // 详情挂载会加载预览依赖；先订阅这些请求的完成，再进入并返回，避免用导航中断制造网络错误。
    const previewAssets = ["echarts.min.js", "mermaid.min.js", "highlight.min.js", "katex.min.js", "atom-one-light.min.css", "katex.min.css"]
    await Promise.all([
      Promise.all(previewAssets.map((asset) => page.waitForResponse((response) => {
        const url = new URL(response.url())
        return url.hostname === "unpkg.com" && url.pathname.endsWith(`/${asset}`)
      }, { timeout: 20_000 }).then((response) => response.finished()))),
      page.getByRole("heading", { name: "最新文章", level: 2 }).locator("xpath=ancestor::section").getByRole("link", { name: /在 Edge 中验证中文博客的完整阅读体验/ }).click(),
    ])
    await expect(page).toHaveURL((url) => url.pathname === "/articles/101" && url.hash === "")
    expect(await page.evaluate(() => window.scrollY)).toBe(0)
    await page.goBack()
    await expect(page).toHaveURL((url) => url.pathname === "/" && url.hash === "")
    await waitForHeroBackground(page)
    await expect(content).toBeVisible()
    await expect(page.locator(".public-hero")).toBeHidden()
    expect(await page.evaluate(() => window.history.state.homeScene)).toBe("content")
    expect(await page.evaluate(() => window.scrollY)).toBe(readingPosition)

    // 归档旧地址落到首页结果，键盘锚点仍不能被头部遮挡。
    await gotoReady(page, "/articles", page.getByRole("heading", { name: "最新文章", level: 2 }))
    await expect(page).toHaveURL((url) => url.pathname === "/" && url.hash === "#home-results")
    await page.getByRole("link", { name: "跳到主要内容" }).focus()
    await expect(page.getByRole("link", { name: "跳到主要内容" })).toBeFocused()
    await page.keyboard.press("Enter")
    await expect(content).toBeFocused()
    await expect(page).toHaveURL((url) => url.pathname === "/")
    await expect(page.locator(".public-hero")).toBeHidden()
    expect(await content.evaluate((target) => {
      const header = document.querySelector<HTMLElement>("header[data-state]")
      return header !== null && (target.firstElementChild?.getBoundingClientRect().top ?? 0) >= header.getBoundingClientRect().bottom
    })).toBe(true)

    // 带 hash 的初始路由也必须消费目标偏移，而不是按绝对坐标遮挡主内容。
    await gotoReady(page, "/#blog-content", page.getByRole("heading", { name: "最新文章", level: 2 }))
    expect(await content.evaluate((target) => {
      const header = document.querySelector<HTMLElement>("header[data-state]")
      return header !== null && (target.firstElementChild?.getBoundingClientRect().top ?? 0) >= header.getBoundingClientRect().bottom
    })).toBe(true)
    expectHealthy(health)
  })
}
