import { START_LOCATION, type RouterScrollBehavior } from "vue-router"
import type { ScrollRestorationService } from "@/services/scroll-restoration"

const READY_ROUTE_NAMES = new Set(["home", "article-list", "article-detail"])

/** 创建保持 history、刷新、hash 与普通 SPA 优先级的滚动策略。 */
export function createBlogScrollBehavior(restoration: Pick<ScrollRestorationService, "arm" | "cancel" | "wait">): RouterScrollBehavior {
  return async (to, from, savedPosition) => {
    if (from === START_LOCATION && typeof to.name === "string" && READY_ROUTE_NAMES.has(to.name) && restoration.arm(to.fullPath)) return restoration.wait()
    if (from !== START_LOCATION) restoration.cancel()
    if (savedPosition !== null) return savedPosition
    if (to.hash.length > 0) {
      const target = document.getElementById(to.hash.slice(1))
      // Vue Router 使用坐标滚动，不会像原生锚点一样自动应用 scroll-margin。
      return target === null ? { el: to.hash } : { el: to.hash, top: Number.parseFloat(getComputedStyle(target).scrollMarginTop) || 0 }
    }
    if (to.name === "home" && from.name === "home") return false
    return { top: 0 }
  }
}
