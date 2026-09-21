import { START_LOCATION, type RouterScrollBehavior } from "vue-router"
import type { ScrollRestorationService } from "@/services/scroll-restoration"

const READY_ROUTE_NAMES = new Set(["home", "article-list", "article-detail"])

/** 创建保持 history、刷新、hash 与普通 SPA 优先级的滚动策略。 */
export function createBlogScrollBehavior(restoration: Pick<ScrollRestorationService, "arm" | "cancel" | "wait">): RouterScrollBehavior {
  return async (to, from, savedPosition) => {
    if (from === START_LOCATION && typeof to.name === "string" && READY_ROUTE_NAMES.has(to.name) && restoration.arm(to.fullPath)) return restoration.wait()
    if (from !== START_LOCATION) restoration.cancel()
    if (savedPosition !== null) return savedPosition
    if (to.hash.length > 0) return { el: to.hash }
    return { top: 0 }
  }
}
