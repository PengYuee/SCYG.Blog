import { expect, test } from "@playwright/test"
import { installReadFixtures } from "./fixtures/api-fixtures"
import { expectHealthy, gotoReady, observePageHealth } from "./helpers/page-health"

const maliciousMarkdown = `# 安全标题

<script>window.__t13Xss = true</script>
<img src=x onerror="window.__t13Xss = true">
[危险链接](javascript:alert(1))

## 安全小节

保留的正文。`

test("后端 5xx 与离线错误呈现可恢复界面", async ({ page }) => {
  // Given: 列表接口在导航前固定为服务端错误。
  const health = observePageHealth(page)
  await installReadFixtures(page, { listStatus: 503 })
  // When: 打开文章列表。
  await gotoReady(page, "/articles", "[role='alert']")
  // Then: 用户获得明确重试操作，页面本身没有脚本异常。
  await expect(page.getByRole("heading", { name: "文章归档" })).toBeVisible()
  await expect(page.getByRole("button", { name: "重试" })).toBeVisible()
  expectHealthy(health)
})

test("详情离线失败保持导航并提供重新加载", async ({ page }) => {
  // Given: 详情请求模拟真实离线中断，其余字典正常。
  const health = observePageHealth(page)
  await installReadFixtures(page)
  await page.route(/\/Article\/GetArticle(?:\?|$)/, (route) => route.abort("internetdisconnected"))
  // When: 打开文章详情。
  await gotoReady(page, "/articles/101", "[data-state='error']")
  // Then: 错误局限在详情并提供可操作重试。
  await expect(page.getByRole("heading", { name: "文章暂时无法加载" })).toBeVisible()
  await expect(page.getByRole("button", { name: "重新加载" })).toBeVisible()
  expect(health.errors).toEqual([])
})

test("恶意 Markdown 被共享清理器移除且安全内容保留", async ({ page }) => {
  // Given: 详情 fixture 包含脚本、事件处理器和危险协议。
  const health = observePageHealth(page)
  await installReadFixtures(page, { detailMarkdown: maliciousMarkdown })
  // When: 打开详情并等待 Markdown 渲染。
  await gotoReady(page, "/articles/101", "[data-testid='markdown-layout']")
  // Then: 攻击载荷不可执行也不可留在 DOM，安全标题仍存在。
  await expect(page.getByRole("heading", { name: "安全标题" })).toBeVisible()
  await expect(page.getByRole("heading", { name: "安全小节" })).toBeVisible()
  await expect(page.locator("script, [onerror], a[href^='javascript:']")).toHaveCount(0)
  await expect.poll(async () => page.evaluate(() => "__t13Xss" in window)).toBe(false)
  expectHealthy(health)
})

test("Hero 图片失败使用项目自有可访问回退", async ({ page }) => {
  // Given: Hero 图片响应为不可解码内容。
  const health = observePageHealth(page)
  await installReadFixtures(page)
  await page.route("**/images/hero-starry.jpg", (route) => route.fulfill({ status: 200, contentType: "image/jpeg", body: "invalid-image" }))
  // When: 打开首页。
  await gotoReady(page, "/", "[data-testid='hero-fallback']")
  // Then: 回退图语义存在且 Hero 仍为精确视口高度。
  await expect(page.getByTestId("hero-fallback")).toHaveAttribute("aria-label", "星空背景暂时无法显示")
  const heroHeight = await page.locator(".public-hero").evaluate((element) => element.getBoundingClientRect().height)
  expect(Math.abs(heroHeight - (page.viewportSize()?.height ?? 0))).toBeLessThanOrEqual(1)
  expectHealthy(health)
})

test("不支持的认证不发请求也不伪造登录表单", async ({ page }) => {
  // Given: 监视认证相关网络调用。
  const health = observePageHealth(page)
  const authRequests: string[] = []
  page.on("request", (request) => { if (/auth|login|token|session/i.test(request.url())) authRequests.push(request.url()) })
  // When: 打开生产登录地址。
  await gotoReady(page, "/login", "[data-layout='public']")
  // Then: 明确 unsupported，且不存在表单、令牌或认证请求。
  await expect(page.getByRole("heading", { name: "登录功能暂不可用" })).toBeVisible()
  await expect(page.locator("form")).toHaveCount(0)
  expect(authRequests).toEqual([])
  const storedKeys = await page.evaluate(() => [...Object.keys(localStorage), ...Object.keys(sessionStorage)])
  expect(storedKeys.filter((key) => /auth|token|session/i.test(key))).toEqual([])
  expectHealthy(health)
})
