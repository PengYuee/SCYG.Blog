import { createPinia } from "pinia"
import { createApp } from "vue"
import App from "@/App.vue"
import type { RuntimeConfig } from "@/config/runtime"
import { runtimeConfigKey } from "@/config/runtime-provider"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import { configureAuthTokenProvider, configureUnauthorizedHandler, http } from "@/request/http"
import { router } from "@/router"
import { createAuthNavigationGuard } from "@/router/guards"
import { authSessionKey, createAuthSessionController } from "@/services/auth-session"
import { scrollRestoration, scrollRestorationKey } from "@/services/scroll-restoration"
import { useAuthStore } from "@/stores/auth"

/** 创建、配置并挂载 Vue 应用。 */
export function mountApplication(config: RuntimeConfig): void {
  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)

  const apiServices = createApiServices(http, config.serverUrl)
  const authSession = createAuthSessionController(apiServices.auth, useAuthStore(pinia), window.localStorage)
  authSession.restore()
  configureAuthTokenProvider(authSession.accessToken)
  configureUnauthorizedHandler(authSession.handleUnauthorized)
  router.beforeEach(createAuthNavigationGuard(authSession))

  scrollRestoration.install()
  app.provide(scrollRestorationKey, scrollRestoration)
  app.provide(runtimeConfigKey, config)
  app.provide(apiServicesKey, apiServices)
  app.provide(authSessionKey, authSession)
  app.use(router)
  app.mount("#app")
}
