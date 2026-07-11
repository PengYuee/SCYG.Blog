import type { RouteLocationGeneric, RouteRecordRaw } from "vue-router"
import { parseLegacyArticleId } from "@/router/query"

/** 旧文章地址仅在标识合法时映射详情，非法值留在原路由呈现类型化失败。 */
function resolveLegacyArticle(to: RouteLocationGeneric) {
  const articleId = parseLegacyArticleId(to.params["id"])
  return articleId === null ? true : { path: `/articles/${articleId}` }
}

/** 旧写作地址按 id 分流；非法 id 留在原地呈现类型化失败。 */
function resolveLegacyWrite(to: RouteLocationGeneric) {
  if (to.query["id"] === undefined) return { path: "/author/articles/new" }
  const articleId = parseLegacyArticleId(to.query["id"])
  return articleId === null ? true : { path: `/author/articles/${articleId}/edit` }
}

/** 公共域路由；catch-all 必须由总路由放在 admin 与 author 之后。 */
export const publicRoutes: readonly RouteRecordRaw[] = [
  { path: "/", name: "home", component: () => import("@/views/public/HomeView.vue"), meta: { title: "首页" } },
  { path: "/articles", name: "article-list", component: () => import("@/views/public/ArticleListView.vue"), meta: { title: "文章" } },
  { path: "/articles/:id", name: "article-detail", component: () => import("@/views/public/ArticleDetailView.vue"), meta: { title: "文章详情" } },
  {
    path: "/login",
    name: "login-unavailable",
    component: () => import("@/views/public/PublicNotFoundView.vue"),
    props: { mode: "login-unavailable" },
    meta: { title: "登录暂不可用" },
  },
  { path: "/main", redirect: { path: "/" } },
  {
    path: "/article/:id",
    name: "legacy-article-invalid",
    beforeEnter: resolveLegacyArticle,
    component: () => import("@/views/public/PublicNotFoundView.vue"),
    props: { mode: "invalid-legacy-id" },
    meta: { title: "无效文章标识", errorCode: "INVALID_ARTICLE_ID" },
  },
  {
    path: "/writeBlog",
    name: "legacy-write-invalid",
    beforeEnter: resolveLegacyWrite,
    component: () => import("@/views/public/PublicNotFoundView.vue"),
    props: { mode: "invalid-legacy-id" },
    meta: { title: "无效文章标识", errorCode: "INVALID_ARTICLE_ID" },
  },
  {
    path: "/:pathMatch(.*)*",
    name: "public-not-found",
    component: () => import("@/views/public/PublicNotFoundView.vue"),
    meta: { title: "页面未找到" },
  },
]
