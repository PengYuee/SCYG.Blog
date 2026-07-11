<script setup lang="ts">
import HomeView from "@/views/public/HomeView.vue"
import { createArticleFeed, type ArticleFeedApi } from "@/stores/article-feed"
import { createTaxonomy, type TaxonomyApi } from "@/stores/taxonomy"
import type { ArticleDetail } from "@/types/article"
import type { PageResult } from "@/types/api"

/** T8 浏览器验收场景。 */
type ShowcaseMode = "happy" | "empty" | "failure"
/** 从独立展示 URL 选择有界验收场景。 */
const mode: ShowcaseMode = new URLSearchParams(window.location.search).get("mode") === "failure"
  ? "failure"
  : new URLSearchParams(window.location.search).get("mode") === "empty" ? "empty" : "happy"

/** 构建具备真实领域字段的浏览器文章 fixture。 */
const article = (id: number, articleTypeId: number, visited: number): ArticleDetail => ({
  id,
  title: ["从 Vue 响应式边界理解状态机", "给旧接口一层可靠的类型契约", "桌面阅读体验的节奏与留白", "用确定性测试守住异步交互", "分类字典如何服务文章发现", "让错误恢复不再清空页面", "构建稳定的文章分组顺序", "从真实数据生成推荐内容", "在 Edge 中验证中文排版"][id - 1] ?? `文章 ${id}`,
  slug: `home-fixture-${id}`,
  digest: "这是一段来自领域 fixture 的文章摘要，用于验证真实标题、元数据、分组和中文换行。",
  markdown: "正文",
  articleTypeId,
  tagIds: [id % 2 === 0 ? 2 : 1],
  status: 1,
  support: id * 2,
  comment: id,
  visited,
  version: 1,
  createdAt: `2026-07-${String(id).padStart(2, "0")}T00:00:00Z`,
  updatedAt: null,
})

/** Happy 场景的真实只读领域 fixture。 */
const articles: readonly ArticleDetail[] = [
  article(1, 1, 48), article(2, 2, 86), article(3, 3, 32), article(4, 1, 74), article(5, 2, 51),
  article(6, 1, 28), article(7, 1, 19), article(8, 1, 14), article(9, 1, 9),
]
/** T4 文章流依赖，按场景返回成功、空或失败。 */
const articleApi: ArticleFeedApi = {
  async list(): Promise<PageResult<ArticleDetail>> {
    if (mode === "failure") throw new Error("文章服务暂时不可用")
    const items = mode === "empty" ? [] : articles
    return { items, pageIndex: 0, pageSize: 20, totalItems: items.length, totalPages: items.length === 0 ? 0 : 1 }
  },
}
/** T4 字典依赖，保留菜单定义顺序。 */
const taxonomyApi: TaxonomyApi = {
  async listArticleTypes() {
    if (mode === "failure") throw new Error("分类服务暂时不可用")
    return [
      { id: 1, name: "前端工程", imageUrl: null, menu: 1 },
      { id: 2, name: "类型与接口", imageUrl: null, menu: 2 },
      { id: 3, name: "设计随笔", imageUrl: null, menu: 3 },
    ]
  },
  async listTags() {
    return [{ id: 1, name: "Vue 3" }, { id: 2, name: "TypeScript" }]
  },
}
/** 展示面使用的真实 T4 状态机。 */
const articleFeed = createArticleFeed(articleApi, 20)
/** 展示面使用的真实 T4 字典状态机。 */
const taxonomy = createTaxonomy(taxonomyApi)
</script>

<template>
  <HomeView :article-feed="articleFeed" :taxonomy="taxonomy" />
</template>
