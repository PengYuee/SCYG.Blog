import type { RouteRecordRaw } from "vue-router"

/** 作者域始终注册，由全局认证守卫控制访问。 */
export const authorRoutes: readonly RouteRecordRaw[] = [
  {
    path: "/author",
    component: () => import("@/layouts/AuthorLayout.vue"),
    meta: { requiresAuth: true },
    children: [
      { path: "ai", name: "author-ai", component: () => import("@/views/author/AiWorkspaceView.vue"), meta: { title: "AI 工作台" } },
      { path: "ai/runs/:runId", name: "author-ai-run", component: () => import("@/views/author/AiRunPage.vue"), meta: { title: "AI 任务" } },
      {
        path: "articles/new",
        name: "author-article-new",
        component: () => import("@/views/author/ArticleEditorView.vue"),
        meta: { title: "新建文章" },
      },
      {
        path: "articles/:id/edit",
        name: "author-article-edit",
        component: () => import("@/views/author/ArticleEditorView.vue"),
        meta: { title: "编辑文章" },
      },
      {
        path: "taxonomy",
        name: "author-taxonomy",
        component: () => import("@/views/author/TaxonomyView.vue"),
        meta: { title: "分类与标签" },
      },
    ],
  },
]
