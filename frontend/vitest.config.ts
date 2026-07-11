import vue from "@vitejs/plugin-vue"
import { fileURLToPath, URL } from "node:url"
import { defineConfig, defineProject } from "vitest/config"

/** 共享源码别名，确保测试解析方式与 Vite 应用一致。 */
const sourceAlias = { "@": fileURLToPath(new URL("./src", import.meta.url)) }

/** Vitest 工作区：unit 与 component 使用互斥目录，避免重复收集。 */
export default defineConfig({
  test: {
    coverage: {
      provider: "v8",
      reporter: ["text", "html"],
      reportsDirectory: "coverage",
    },
    projects: [
      defineProject({
        test: {
          name: "unit",
          environment: "node",
          include: ["tests/unit/**/*.test.ts"],
        },
        resolve: { alias: sourceAlias },
      }),
      defineProject({
        plugins: [vue()],
        test: {
          name: "component",
          environment: "jsdom",
          include: ["tests/component/**/*.test.ts"],
          setupFiles: ["./tests/setup.ts"],
        },
        resolve: { alias: sourceAlias },
      }),
    ],
  },
})
