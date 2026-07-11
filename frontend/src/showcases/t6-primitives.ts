import { createPinia } from "pinia"
import { createApp } from "vue"
import { createRouter, createWebHistory } from "vue-router"
import "@/assets/main.css"
import T6PrimitivesShowcase from "@/showcases/T6PrimitivesShowcase.vue"

/** T6 独立展示路由，不修改 T9 拥有的应用路由。 */
const showcaseRouter = createRouter({
  history: createWebHistory("/t6-showcase.html"),
  routes: [
    { path: "/", component: T6PrimitivesShowcase },
    { path: "/articles", component: T6PrimitivesShowcase },
    { path: "/articles/:id", component: T6PrimitivesShowcase },
  ],
})

/** T6 独立展示应用。 */
const showcaseApp = createApp(T6PrimitivesShowcase)
showcaseApp.use(createPinia())
showcaseApp.use(showcaseRouter)
showcaseApp.mount("#t6-showcase")