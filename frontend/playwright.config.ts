import { defineConfig } from "@playwright/test"

/** 固定桌面视口集合，后续 E2E 任务必须复用且不得引入移动端项目。 */
const desktopViewports = [
  { name: "1280x720", width: 1280, height: 720 },
  { name: "1440x900", width: 1440, height: 900 },
  { name: "1920x1080", width: 1920, height: 1080 },
] as const

/** Playwright 始终使用本机 Microsoft Edge，不依赖 CI 环境变量选择浏览器。 */
const browserName = "edge"
const browserUse = { channel: "msedge" as const }

/** Playwright 开发验收配置：作者图片上传等场景使用显式 Fake 作者运行时；T13 生产验收由独立配置负责。 */
export default defineConfig({
  testDir: "./tests/e2e",
  testMatch: "**/*.spec.ts",
  testIgnore: "**/t13/**/*.spec.ts",
  forbidOnly: false,
  retries: 0,
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
    command: "pnpm dev --mode development --host 127.0.0.1 --port 4173 --strictPort",
    url: "http://127.0.0.1:4173/",
    reuseExistingServer: false,
    timeout: 60_000,
    env: { VITE_FAKE_AUTHOR: "true" },
  },
})
