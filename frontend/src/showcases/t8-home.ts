import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import { createBrowserScrollRestoration, scrollRestorationKey } from "@/services/scroll-restoration"
import "@/assets/main.css"
import T8HomeShowcase from "@/showcases/T8HomeShowcase.vue"
import { apiServicesKey, createApiServices } from "@/request/api-services"
import { http } from "@/request/http"

/** T8 独立展示路由，不修改 T9 拥有的生产路由。 */
const showcaseRouter = createRouter({
  history: createWebHistory("/t8-home.html"),
  routes: [
    { path: "/", component: T8HomeShowcase },
    { path: "/articles", component: T8HomeShowcase },
    { path: "/articles/:id", component: T8HomeShowcase },
  ],
})
/** T8 独立展示应用。 */
const showcaseApp = createApp(T8HomeShowcase)
showcaseApp.use(showcaseRouter)
showcaseApp.provide(apiServicesKey, createApiServices(http, window.location.origin))
showcaseApp.provide(scrollRestorationKey, createBrowserScrollRestoration())
showcaseApp.mount("#t8-home")
