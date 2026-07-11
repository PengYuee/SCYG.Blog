import { defineConfig } from "@playwright/test"

/** T6 Microsoft Edge 桌面视口。 */
const desktopViewports = [
  { name: "edge-1280x720", width: 1280, height: 720 },
  { name: "edge-1440x900", width: 1440, height: 900 },
  { name: "edge-1920x1080", width: 1920, height: 1080 },
] as const

/** T6 独立 Edge 配置；Vite 由外层有界 Start-Process 生命周期管理。 */
export default defineConfig({
  testDir: ".",
  testMatch: "t6-primitives.spec.ts",
  reporter: [["list"], ["json", { outputFile: process.env["T6_RESULT_JSON"] ?? "t6-edge-results.json" }]],
  timeout: 30_000,
  use: {
    baseURL: process.env["T6_BASE_URL"] ?? "http://127.0.0.1:4173",
    channel: "msedge",
    trace: "retain-on-failure",
  },
  projects: desktopViewports.map(({ name, width, height }) => ({ name, use: { viewport: { width, height } } })),
})