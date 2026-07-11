import vue from "@vitejs/plugin-vue"
import { defineConfig } from "vite"

/** Vite 构建配置：启用 Vue 单文件组件与 @ 源码别名。 */
export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      "@": "/src",
    },
  },
})