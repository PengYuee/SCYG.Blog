import { createPinia } from "pinia"
import { createApp } from "vue"
import App from "@/App.vue"
import type { RuntimeConfig } from "@/config/runtime"
import { runtimeConfigKey } from "@/config/runtime-provider"
import { router } from "@/router"

/** 创建、配置并挂载 Vue 应用。 */
export function mountApplication(config: RuntimeConfig): void {
  const app = createApp(App)
  app.use(createPinia())
  app.use(router)
  app.provide(runtimeConfigKey, config)
  app.mount("#app")
}