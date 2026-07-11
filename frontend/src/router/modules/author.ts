import { RouterView, type RouteRecordRaw } from "vue-router"
import { fakeAuthorEnabled } from "@/services/author-runtime"
import { authorRouteAvailability, type AuthorRouteAvailability } from "@/router/guards"
import { parseAuthRuntimeConfig, type AuthState } from "@/stores/auth"

/** 生产作者能力使用 T5 的真实不可用结果，不伪造登录态。 */
const productionAuthorState: AuthState = { kind: "unsupported", reason: "backend auth unavailable" }
const productionAvailability = authorRouteAvailability(
  parseAuthRuntimeConfig({ mode: "production", fakeAuthEnabled: false }),
  productionAuthorState,
)

/** 从同一显式 Fake 谓词创建组件与元数据一致的作者路由。 */
export function createAuthorRoutes(fakeEnabled: boolean): readonly RouteRecordRaw[] {
  const availability: AuthorRouteAvailability = fakeEnabled ? { kind: "available" } : productionAvailability
  const authorLayout = fakeEnabled ? () => import("@/layouts/AuthorLayout.vue") : RouterView
  const articleEditor = fakeEnabled ? () => import("@/views/author/ArticleEditorView.vue") : () => import("@/views/public/PublicNotFoundView.vue")
  const taxonomy = fakeEnabled ? () => import("@/views/author/TaxonomyView.vue") : () => import("@/views/public/PublicNotFoundView.vue")
  return [
  {
    path: "/author",
    component: authorLayout,
    meta: { availability },
    children: [
      {
        path: "articles/new",
        name: "author-article-new",
        component: articleEditor,
        props: fakeEnabled ? {} : { mode: "author-unavailable" },
        meta: { title: fakeEnabled ? "新建文章" : "写作功能暂不可用", availability },
      },
      {
        path: "articles/:id/edit",
        name: "author-article-edit",
        component: articleEditor,
        props: fakeEnabled ? {} : { mode: "author-unavailable" },
        meta: { title: fakeEnabled ? "编辑文章" : "写作功能暂不可用", availability },
      },
      {
        path: "taxonomy",
        name: "author-taxonomy",
        component: taxonomy,
        props: fakeEnabled ? {} : { mode: "author-unavailable" },
        meta: { title: fakeEnabled ? "分类与标签" : "分类管理暂不可用", availability },
      },
    ],
  },
  ]
}

/** 作者域预留新建、编辑和分类入口；显式 Fake 谓词是唯一切换边界。 */
export const authorRoutes = createAuthorRoutes(fakeAuthorEnabled)
