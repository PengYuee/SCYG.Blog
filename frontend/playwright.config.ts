import { defineConfig } from "@playwright/test"

/** 固定桌面视口集合，后续 E2E 任务必须复用且不得引入移动端项目。 */
const desktopViewports = [
  { name: "1280x720", width: 1280, height: 720 },
  { name: "1440x900", width: 1440, height: 900 },
  { name: "1920x1080", width: 1920, height: 1080 },
] as const

/** CI 使用 Playwright Chromium；本地显式选择已安装的 Microsoft Edge channel。 */
const browserName = process.env["CI"] ? "chromium" : "edge"
const browserUse = process.env["CI"] ? {} : { channel: "msedge" as const }

/** Playwright 桌面配置：仅收集 tests/e2e，和 Vitest globs 完全隔离。 */
export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: "**/*.spec.ts",
  forbidOnly: Boolean(process.env["CI"]),
  retries: process.env["CI"] ? 2 : 0,
  reporter: [["list"], ["html", { open: "never", outputFolder: "playwright-report" }]],
  use: {
    baseURL: "http://127.0.0.1:4173",
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
    ...browserUse,
  },
  projects: desktopViewports.map(({ name, width, height }) => ({
    name: `${browserName}-${name}`,
    use: { viewport: { width, height } },
  })),
  webServer: {
    command: "pnpm dev --host 127.0.0.1 --port 4173 --strictPort",
    url: "http://127.0.0.1:4173/admin/dashboard",
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
