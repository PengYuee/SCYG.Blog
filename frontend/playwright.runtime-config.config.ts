import { defineConfig } from "@playwright/test"

/** 运行时配置契约只使用已构建产物与本机 Microsoft Edge。 */
export default defineConfig({
  testDir: "./tests/e2e/runtime-config",
  testMatch: "**/*.spec.ts",
  reporter: "list",
  use: {
    baseURL: "http://127.0.0.1:4173",
    channel: "msedge",
    viewport: { width: 1440, height: 900 },
  },
})