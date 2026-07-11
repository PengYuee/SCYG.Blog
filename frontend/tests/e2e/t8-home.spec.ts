import { expect, test, type Page } from "@playwright/test"
import { fileURLToPath } from "node:url"
import path from "node:path"

/** T8 Edge 截图证据目录。 */
const evidenceDirectory = path.resolve(fileURLToPath(new URL("../../../.omo/evidence/zfy-blog-vue3-desktop-redesign/t8-edge", import.meta.url)))

/** 捕获页面级错误并返回可断言集合。 */
const captureErrors = (page: Page): string[] => {
  const errors: string[] = []
  page.on("console", (message) => { if (message.type() === "error") errors.push(`console:${message.text()}`) })
  page.on("pageerror", (error) => errors.push(`page:${error.message}`))
  return errors
}

test("keeps the first desktop fold hero-only and composes discovery below it", async ({ page }, testInfo) => {
  // Given: Edge 捕获当前首页全部运行时错误。
  const errors = captureErrors(page)

  // When: 打开 T8 Happy 首页并等待真实状态机完成。
  await page.goto("/t8-home.html", { waitUntil: "networkidle" })
  await expect(page.getByTestId("home-announcement")).toContainText("已加载 9 篇")

  // Then: 首屏严格只有 Hero，内容从视口下方开始且无横向溢出。
  const fold = await page.evaluate(() => {
    const hero = document.querySelector<HTMLElement>(".public-hero")
    const announcement = document.querySelector<HTMLElement>("[data-testid='home-announcement']")
    const cards = [...document.querySelectorAll<HTMLElement>("#blog-content .blog-card")]
    return {
      heroHeight: hero?.getBoundingClientRect().height ?? 0,
      announcementTop: announcement?.getBoundingClientRect().top ?? 0,
      firstCardTop: Math.min(...cards.map((card) => card.getBoundingClientRect().top)),
      viewportHeight: window.innerHeight,
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    }
  })
  expect(Math.abs(fold.heroHeight - fold.viewportHeight)).toBeLessThanOrEqual(1)
  expect(fold.announcementTop).toBeGreaterThanOrEqual(fold.viewportHeight)
  expect(fold.firstCardTop).toBeGreaterThanOrEqual(fold.viewportHeight)
  expect(fold.overflow).toBe(0)
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-first-fold.png`), fullPage: false })

  // When: 进入发现内容并检查真实排序、分组和确定性 MORE 查询。
  await page.locator("#blog-content").scrollIntoViewIfNeeded()
  await expect(page.getByTestId("recommended-articles")).toContainText("给旧接口一层可靠的类型契约")
  const categorySections = page.getByTestId("category-section")
  await expect(categorySections).toHaveCount(3)
  await expect(categorySections.nth(0).locator("h2")).toHaveText("前端工程")
  await expect(categorySections.nth(0).locator("article")).toHaveCount(6)
  await expect(categorySections.nth(0).getByRole("link", { name: "更多文章" })).toHaveAttribute("href", "/t8-home.html/articles?articleTypeId=1")
  const composition = await page.evaluate(() => ({
    sidebarWidth: document.querySelector<HTMLElement>(".blog-sidebar")?.getBoundingClientRect().width ?? 0,
    overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    clippedText: [...document.querySelectorAll<HTMLElement>("h1,h2,h3,p,a,button")].filter((element) => element.scrollWidth > element.clientWidth + 1).length,
  }))
  expect(composition.sidebarWidth).toBe(300)
  expect(composition.overflow).toBe(0)
  expect(composition.clippedText).toBe(0)
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-composition.png`), fullPage: true })
  expect(errors).toEqual([])
})

test("routes hero search and scrolls through the isolated homepage router", async ({ page }) => {
  // Given: Happy 首页处于 Hero 首屏。
  const errors = captureErrors(page)
  await page.goto("/t8-home.html", { waitUntil: "networkidle" })

  // When: 提交 Hero 搜索。
  await page.locator("#hero-search-input").fill("  Vue 状态机  ")
  await page.locator("form[role='search']").first().press("Enter")

  // Then: 独立路由保留查询且原生滚动进入内容。
  await expect(page).toHaveURL(/\/t8-home\.html\/articles\?q=Vue\+%E7%8A%B6%E6%80%81%E6%9C%BA$/)
  await expect.poll(async () => page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
  expect(errors).toEqual([])
})

test("keeps scoped failures and empty feed composed", async ({ page }, testInfo) => {
  // Given: 字典与文章流均失败。
  const errors = captureErrors(page)
  await page.goto("/t8-home.html?mode=failure", { waitUntil: "networkidle" })

  // Then: 两个失败域分别可重试，个人与搜索表面仍存在。
  await expect(page.getByTestId("taxonomy-error")).toContainText("分类服务暂时不可用")
  await expect(page.getByTestId("feed-error")).toContainText("文章服务暂时不可用")
  await expect(page.getByRole("heading", { name: "文章搜索" })).toBeVisible()
  await expect(page.getByText("妄揽明月", { exact: true }).first()).toBeVisible()
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-failure.png`), fullPage: true })

  // When: 切换到真实空文章流。
  await page.goto("/t8-home.html?mode=empty", { waitUntil: "networkidle" })

  // Then: 空状态与侧栏同时存在且无运行时错误。
  await expect(page.getByTestId("feed-empty")).toContainText("暂无已发布文章")
  await expect(page.getByText("TypeScript", { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0)
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-empty.png`), fullPage: true })
  expect(errors).toEqual([])
})
