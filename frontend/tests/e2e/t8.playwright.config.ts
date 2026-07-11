import { defineConfig } from "@playwright/test"

/** T8 Microsoft Edge 桌面验收视口。 */
const desktopViewports = [
  { name: "edge-1280x720", width: 1280, height: 720 },
  { name: "edge-1440x900", width: 1440, height: 900 },
  { name: "edge-1920x1080", width: 1920, height: 1080 },
] as const

/** T8 独立 Edge 配置；预览服务由外层有界 Start-Process 管理。 */
export default defineConfig({
  testDir: ".",
  testMatch: "t8-home.spec.ts",
  reporter: [["list"], ["json", { outputFile: process.env["T8_RESULT_JSON"] ?? "t8-edge-results.json" }]],
  timeout: 30_000,
  use: {
    baseURL: process.env["T8_BASE_URL"] ?? "http://127.0.0.1:4178",
    channel: "msedge",
    trace: "retain-on-failure",
  },
  projects: desktopViewports.map(({ name, width, height }) => ({ name, use: { viewport: { width, height } } })),
})
