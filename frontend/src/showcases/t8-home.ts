import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import "@/assets/main.css"
import T8HomeShowcase from "@/showcases/T8HomeShowcase.vue"

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
showcaseApp.mount("#t8-home")
