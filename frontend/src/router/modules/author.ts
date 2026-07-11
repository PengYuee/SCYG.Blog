import { RouterView, type RouteRecordRaw } from "vue-router"
import { authorRouteAvailability } from "@/router/guards"
import { parseAuthRuntimeConfig, type AuthState } from "@/stores/auth"

/** 生产作者能力使用 T5 的真实不可用结果，不伪造登录态。 */
const productionAuthorState: AuthState = { kind: "unsupported", reason: "backend auth unavailable" }
const authorAvailability = authorRouteAvailability(
  parseAuthRuntimeConfig({ mode: "production", fakeAuthEnabled: false }),
  productionAuthorState,
)

/** 作者域预留新建、编辑和分类入口；T11 仅替换本模块的懒加载视图。 */
export const authorRoutes: readonly RouteRecordRaw[] = [
  {
    path: "/author",
    component: RouterView,
    meta: { availability: authorAvailability },
    children: [
      {
        path: "articles/new",
        name: "author-article-new",
        component: () => import("@/views/public/PublicNotFoundView.vue"),
        props: { mode: "author-unavailable" },
        meta: { title: "写作功能暂不可用", availability: authorAvailability },
      },
      {
        path: "articles/:id/edit",
        name: "author-article-edit",
        component: () => import("@/views/public/PublicNotFoundView.vue"),
        props: { mode: "author-unavailable" },
        meta: { title: "写作功能暂不可用", availability: authorAvailability },
      },
      {
        path: "taxonomy",
        name: "author-taxonomy",
        component: () => import("@/views/public/PublicNotFoundView.vue"),
        props: { mode: "author-unavailable" },
        meta: { title: "分类管理暂不可用", availability: authorAvailability },
      },
    ],
  },
]
