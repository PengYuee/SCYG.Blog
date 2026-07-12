import { createPinia } from "pinia"
import { createApp } from "vue"
import App from "@/App.vue"
import type { RuntimeConfig } from "@/config/runtime"
import { runtimeConfigKey } from "@/config/runtime-provider"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import { http } from "@/request/http"
import { router } from "@/router"

/** 创建、配置并挂载 Vue 应用。 */
export function mountApplication(config: RuntimeConfig): void {
  const app = createApp(App)
  app.use(createPinia())
  app.use(router)
  const apiServices = createApiServices(http, config.serverUrl)
  app.provide(runtimeConfigKey, config)
  app.provide(apiServicesKey, apiServices)
  app.mount("#app")
}
