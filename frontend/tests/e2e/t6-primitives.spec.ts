import { expect, test } from "@playwright/test"
import { fileURLToPath } from "node:url"
import path from "node:path"

/** T6 Edge 截图证据目录。 */
const evidenceDirectory = path.resolve(fileURLToPath(new URL("../../../.omo/evidence/zfy-blog-vue3-desktop-redesign/t6-edge", import.meta.url)))

test("verifies desktop public primitives through Microsoft Edge", async ({ page }, testInfo) => {
  // Given: 捕获所有控制台和页面错误。
  const errors: string[] = []
  page.on("console", (message) => { if (message.type() === "error") errors.push(`console:${message.text()}`) })
  page.on("pageerror", (error) => errors.push(`page:${error.message}`))

  // When: Edge 打开当前 T6 独立展示面。
  await page.goto("/t6-showcase.html", { waitUntil: "networkidle" })
  const hero = page.locator(".public-hero")
  const content = page.locator("#blog-content")
  const header = page.locator("header[data-state]")
  await expect(header).toHaveAttribute("data-state", "transparent")

  // Then: Hero、内容边界、300px 侧栏、三列和横向边界均符合桌面契约。
  const geometry = await page.evaluate(() => {
    const heroElement = document.querySelector<HTMLElement>(".public-hero")
    const contentElement = document.querySelector<HTMLElement>("#blog-content")
    const sidebarElement = document.querySelector<HTMLElement>(".blog-sidebar")
    const gridElement = document.querySelector<HTMLElement>("[data-testid='article-grid']")
    if (heroElement === null || contentElement === null || sidebarElement === null || gridElement === null) return null
    return {
      heroHeight: heroElement.getBoundingClientRect().height,
      contentTop: contentElement.getBoundingClientRect().top + window.scrollY,
      sidebarWidth: sidebarElement.getBoundingClientRect().width,
      cardColumns: getComputedStyle(gridElement).gridTemplateColumns.split(" ").length,
      viewportHeight: window.innerHeight,
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    }
  })
  expect(geometry).not.toBeNull()
  expect(Math.abs((geometry?.heroHeight ?? 0) - (geometry?.viewportHeight ?? 1))).toBeLessThanOrEqual(1)
  expect(Math.abs((geometry?.contentTop ?? 0) - (geometry?.heroHeight ?? 1))).toBeLessThanOrEqual(1)
  expect(geometry?.sidebarWidth).toBe(300)
  expect(geometry?.cardColumns).toBe(3)
  expect(geometry?.overflow).toBe(0)

  // When: 显式换一句并检查全部站外链接策略。
  const quote = page.getByTestId("hero-quote")
  const originalQuote = await quote.textContent()
  await page.getByTestId("quote-refresh").click()
  await expect(quote).not.toHaveText(originalQuote ?? "")
  const externalLinks = page.locator('a[target="_blank"]')
  expect(await externalLinks.count()).toBe(8)
  for (const link of await externalLinks.all()) await expect(link).toHaveAttribute("rel", "noopener noreferrer")

  // When: 用户启用减少动态效果。
  await page.emulateMedia({ reducedMotion: "reduce" })
  await page.evaluate(() => {
    const nativeScrollIntoView = Element.prototype.scrollIntoView
    Element.prototype.scrollIntoView = function scrollIntoView(options): void {
      document.documentElement.dataset["scrollBehavior"] = typeof options === "object" ? options.behavior ?? "auto" : "auto"
      nativeScrollIntoView.call(this, options)
    }
  })
  const motion = await page.locator(".hero-entrance").evaluate((element) => ({ animationName: getComputedStyle(element).animationName, transform: getComputedStyle(element).transform }))
  expect(motion.animationName).toBe("none")
  expect(motion.transform).toBe("none")

  // When: 首页提交搜索。
  await page.locator("#hero-search-input").fill("  Vue 3  ")
  await page.locator("form[role='search']").first().press("Enter")
  await expect(page).toHaveURL(/\/t6-showcase\.html\/articles\?q=Vue\+3$/)
  await expect.poll(async () => page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
  await expect(header).toHaveAttribute("data-state", "scrolled")
  await expect(page.locator("html")).toHaveAttribute("data-scroll-behavior", "auto")
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-settled.png`), fullPage: true })
  expect(errors).toEqual([])
})

test("shows an accessible fallback when the project hero asset is missing", async ({ page }, testInfo) => {
  // Given: 项目 Hero 图片解码失败，且捕获全部控制台与页面错误。
  const errors: string[] = []
  page.on("console", (message) => { if (message.type() === "error") errors.push(`console:${message.text()}`) })
  page.on("pageerror", (error) => errors.push(`page:${error.message}`))
  await page.route("**/images/hero-starry.jpg", (route) => route.fulfill({ status: 200, contentType: "image/jpeg", body: "invalid-image" }))

  // When: Edge 打开 T6 展示面。
  await page.goto("/t6-showcase.html", { waitUntil: "networkidle" })

  // Then: 可访问的静态回退出现且布局高度不变。
  const fallback = page.getByTestId("hero-fallback")
  await expect(fallback).toHaveAttribute("role", "img")
  await expect(fallback).toHaveAttribute("aria-label", "星空背景暂时无法显示")
  const heroHeight = await page.locator(".public-hero").evaluate((element) => element.getBoundingClientRect().height)
  expect(Math.abs(heroHeight - (page.viewportSize()?.height ?? 0))).toBeLessThanOrEqual(1)
  await page.screenshot({ path: path.join(evidenceDirectory, `${testInfo.project.name}-missing-asset.png`), fullPage: false })
  expect(errors).toEqual([])
})