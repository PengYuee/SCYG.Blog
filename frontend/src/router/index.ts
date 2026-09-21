import { createRouter, createWebHistory } from "vue-router"
import { adminRoutes } from "@/router/modules/admin"
import { authorRoutes } from "@/router/modules/author"
import { publicRoutes } from "@/router/modules/public"
import { createBlogScrollBehavior } from "@/router/scroll-behavior"
import { scrollRestoration } from "@/services/scroll-restoration"

/** 生产路由按 admin、author、public 顺序组合，避免公共 catch-all 吞掉保留域。 */
export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [...adminRoutes, ...authorRoutes, ...publicRoutes],
  /** 返回历史位置优先，初始刷新交给一次性服务，随后保留锚点与普通 SPA 回顶。 */
  scrollBehavior: createBlogScrollBehavior(scrollRestoration),
})

/** 每次成功导航后以路由元数据同步唯一文档标题。 */
router.afterEach((to) => {
  const title = typeof to.meta["title"] === "string" ? to.meta["title"] : "SCYG Blog"
  document.title = `${title} · SCYG Blog`
})
