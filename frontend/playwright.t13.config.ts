import { defineConfig } from "@playwright/test"

/** T13 要求的三个精确桌面视口。 */
const desktopViewports = [
  { name: "1280x720", width: 1280, height: 720 },
  { name: "1440x900", width: 1440, height: 900 },
  { name: "1920x1080", width: 1920, height: 1080 },
] as const

/** 本地固定 Microsoft Edge，CI 可使用同尺寸 Chromium。 */
const browserName = process.env["CI"] ? "chromium" : "edge"
const browserUse = process.env["CI"] ? {} : { channel: "msedge" as const }

/** T13 仅验证预构建生产产物，证据统一写入忽略目录。 */
export default defineConfig({
  testDir: "./tests/e2e/t13",
  testMatch: "**/*.spec.ts",
  forbidOnly: Boolean(process.env["CI"]),
  retries: process.env["CI"] ? 2 : 0,
  reporter: [["list"], ["html", { open: "never", outputFolder: "../.omo/evidence/zfy-blog-vue3-desktop-redesign/t13/report" }]],
  outputDir: "../.omo/evidence/zfy-blog-vue3-desktop-redesign/t13/test-results",
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
    command: "pnpm preview --host 127.0.0.1 --port 4173 --strictPort",
    url: "http://127.0.0.1:4173/",
    reuseExistingServer: false,
    timeout: 60_000,
  },
})
