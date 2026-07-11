import { expect, test } from "@playwright/test"

test("opens the actual Vite page and renders the admin dashboard", async ({ page }) => {
  // Given: Playwright starts the real Vite application through webServer.
  // When: the desktop browser opens the production route.
  await page.goto("/admin/dashboard")

  // Then: a binary visible heading proves the application mounted and routed.
  await expect(page.getByRole("heading", { name: /仪表盘$/ })).toBeVisible()
})
