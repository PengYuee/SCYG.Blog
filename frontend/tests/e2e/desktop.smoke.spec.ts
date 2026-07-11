import { expect, test } from "@playwright/test"

test("opens the actual Vite page and renders the public blog", async ({ page }) => {
  // Given: Playwright 通过 webServer 启动真实 Vite 应用。
  // When: 桌面浏览器打开公共首页。
  await page.goto("/")

  // Then: 可见博客标题证明应用完成挂载与路由。
  await expect(page.getByRole("heading", { name: "妄揽明月" })).toBeVisible()
})
