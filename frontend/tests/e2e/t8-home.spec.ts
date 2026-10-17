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

test("keeps the hero and discovery in exclusive mounted scenes", async ({ page }, testInfo) => {
  // Given: Edge 捕获当前首页全部运行时错误。
  const errors = captureErrors(page)

  // When: 打开 T8 Happy 首页并等待真实状态机完成。
  await page.goto("/t8-home.html", { waitUntil: "networkidle" })
  await expect(page.getByTestId("home-announcement")).toContainText("已加载 9 篇")
  await expect(page.locator("#blog-content")).toBeHidden()

  // Then: 首屏严格只有 Hero，内容从视口下方开始且无横向溢出。
  const fold = await page.evaluate(() => {
    const hero = document.querySelector<HTMLElement>(".public-hero")
    const announcement = document.querySelector<HTMLElement>("[data-testid='home-announcement']")
    const cards = [...document.querySelectorAll<HTMLElement>("#blog-content .blog-card")].filter((card) => card.getClientRects().length > 0)
    return {
      heroHeight: hero?.getBoundingClientRect().height ?? 0,
      contentHidden: announcement?.getClientRects().length === 0,
      visibleCards: cards.length,
      viewportHeight: window.innerHeight,
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    }
  })
  expect(Math.abs(fold.heroHeight - fold.viewportHeight)).toBeLessThanOrEqual(1)
  expect(fold.contentHidden).toBe(true)
  expect(fold.visibleCards).toBe(0)
  expect(fold.overflow).toBe(0)
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-first-fold.png`), fullPage: false })

  // When: 进入发现内容并检查完整且不重复的文章流。
  await page.getByRole("button", { name: "进入内容区", exact: true }).click()
  await expect(page.locator(".public-hero")).toBeHidden()
  expect(await page.evaluate(() => window.scrollY)).toBe(0)
  await expect(page.getByTestId("recommended-articles")).toContainText("给旧接口一层可靠的类型契约")
  const grid = page.getByTestId("article-grid")
  await expect(grid).toHaveCount(1)
  await expect(grid.locator("article")).toHaveCount(9)
  await expect(page.getByRole("heading", { name: "最新文章", exact: true })).toBeVisible()
  await page.getByRole("button", { name: "前端工程", exact: true }).click()
  await expect(page).toHaveURL((url) => url.searchParams.get("articleTypeId") === "1")
  await expect(page.getByRole("button", { name: "前端工程", exact: true })).toHaveAttribute("aria-pressed", "true")
  await expect(grid.locator("article")).toHaveCount(6)
  const composition = await page.evaluate(() => ({
    sidebarWidth: document.querySelector<HTMLElement>(".blog-sidebar")?.getBoundingClientRect().width ?? 0,
    overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    clippedText: [...document.querySelectorAll<HTMLElement>("h1,h2,h3,p,a,button")].filter((element) => element.getClientRects().length > 0 && element.scrollWidth > element.clientWidth + 1).length,
  }))
  expect(composition.sidebarWidth).toBe(220)
  expect(composition.overflow).toBe(0)
  expect(composition.clippedText).toBe(0)
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-composition.png`), fullPage: true })
  expect(errors).toEqual([])
})

test("routes hero search into the content scene through the isolated homepage router", async ({ page }) => {
  // Given: Happy 首页处于 Hero 首屏。
  const errors = captureErrors(page)
  await page.goto("/t8-home.html", { waitUntil: "networkidle" })

  // When: 提交 Hero 搜索。
  await page.locator("#hero-search-input").fill("  Vue 状态机  ")
  await page.locator("form[role='search']").first().press("Enter")

  // Then: 独立路由保留查询且原生滚动进入内容。
  await expect(page).toHaveURL((url) => url.pathname === "/t8-home.html/" && url.searchParams.get("q") === "Vue 状态机")
  await expect(page.locator("header[data-state]")).toHaveAttribute("data-state", "scrolled")
  await expect(page.locator(".public-hero")).toBeHidden()
  await expect(page.locator("#content-search-input")).toHaveValue("Vue 状态机")
  await expect(page.locator("header").getByRole("search")).toHaveCount(0)
  expect(errors).toEqual([])
})

test("keeps scoped failures and empty feed composed", async ({ page }, testInfo) => {
  // Given: 字典与文章流均失败。
  const errors = captureErrors(page)
  await page.goto("/t8-home.html#fixture-failure", { waitUntil: "networkidle" })
  await page.getByTestId("hero-scroll-cue").click()

  // Then: 两个失败域分别可重试，个人与搜索表面仍存在。
  await expect(page.getByTestId("taxonomy-error")).toContainText("分类服务暂时不可用")
  await expect(page.getByTestId("feed-error")).toContainText("文章服务暂时不可用")
  await expect(page.locator("#hero-search-input")).toBeHidden()
  await expect(page.locator("#content-search-input")).toBeVisible()
  await expect(page.locator("aside[aria-labelledby='profile-card-name']")).toBeVisible()
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-failure.png`), fullPage: true })

  // When: 切换到真实空文章流。
  await page.goto("/t8-home.html#fixture-empty", { waitUntil: "networkidle" })
  await page.getByTestId("hero-scroll-cue").click()

  // Then: 空状态与侧栏同时存在且无运行时错误。
  await expect(page.getByTestId("feed-empty")).toContainText("暂无已发布文章")
  await expect(page.getByText("TypeScript", { exact: true })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0)
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-empty.png`), fullPage: true })
  expect(errors).toEqual([])
})
