import { expect, test } from "@playwright/test"
import { installReadFixtures } from "../t13/fixtures/api-fixtures"

const backendApiRoot = "http://localhost:5000/api/"
const viteOrigin = "http://127.0.0.1:4173"
const businessPath = /\/(?:api\/)?(?:Article|ArticleType|Tag)\//

test("uses config.json for every public business request", async ({ page }) => {
  // Given: 生产预览返回真实 config.json，后端读取由确定性 fixture 接管。
  const businessRequests: string[] = []
  page.on("request", (request) => {
    if (businessPath.test(new URL(request.url()).pathname)) businessRequests.push(request.url())
  })
  await installReadFixtures(page)

  // When: 用户进入公共首页并等待文章内容。
  await page.goto("/")
  await expect(page.getByText("在 Edge 中验证中文博客的完整阅读体验").first()).toBeVisible()

  // Then: 每一条业务请求都发往运行时后端，绝不使用 Vite 来源。
  expect(businessRequests.length).toBeGreaterThan(0)
  expect(businessRequests.every((url) => url.startsWith(backendApiRoot))).toBe(true)
  expect(businessRequests.some((url) => url.startsWith(viteOrigin))).toBe(false)
})

test("blocks application import and business requests when config fails", async ({ page }) => {
  // Given: config.json 在应用启动边界返回失败。
  const businessRequests: string[] = []
  page.on("request", (request) => {
    if (businessPath.test(new URL(request.url()).pathname)) businessRequests.push(request.url())
  })
  await page.route("**/config.json", async (route) => route.fulfill({ status: 500, body: "invalid" }))

  // When: 用户打开生产预览。
  await page.goto("/")

  // Then: 稳定中文告警可见，且没有任何业务请求。
  await expect(page.getByRole("alert")).toHaveText("运行时配置加载失败，请检查 config.json 后重试。")
  expect(businessRequests).toEqual([])
})