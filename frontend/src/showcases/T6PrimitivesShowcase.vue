<script setup lang="ts">
import ArticleSection from "@/components/article/ArticleSection.vue"
import ArticleSearchCard from "@/components/public/ArticleSearchCard.vue"
import ProfileCard from "@/components/public/ProfileCard.vue"
import RecommendedArticles from "@/components/public/RecommendedArticles.vue"
import TagCloud from "@/components/public/TagCloud.vue"
import BlogLayout from "@/layouts/BlogLayout.vue"
import type { ArticleSummary } from "@/types/article"
import type { ArticleType, Tag } from "@/types/taxonomy"

/** Showcase 分类 fixture，只承载原语状态。 */
const categories: readonly ArticleType[] = [{ id: 1, name: "前端实践", imageUrl: null, menu: 1 }]
/** Showcase 标签 fixture，只承载原语状态。 */
const tags: readonly Tag[] = [{ id: 1, name: "Vue 3" }, { id: 2, name: "TypeScript" }, { id: 3, name: "工程化" }]
/** Showcase 文章 fixture，不表达产品级精确统计。 */
const articles: readonly ArticleSummary[] = [1, 2, 3].map((id) => ({
  id,
  title: `桌面文章卡片原语 ${id}`,
  slug: `showcase-${id}`,
  digest: "用于验证中文标题、摘要、元数据、焦点和三列桌面网格的组件展示内容。",
  markdown: "",
  articleTypeId: 1,
  tagIds: [id],
  status: 1,
  support: 0,
  comment: 0,
  visited: 0,
  version: 1,
  createdAt: "2026-07-11T00:00:00Z",
  updatedAt: null,
}))
</script>

<template>
  <BlogLayout>
    <div class="blog-content-grid" data-testid="showcase-content-grid">
      <div class="space-y-12">
        <ArticleSection title="公共文章原语" :articles="articles" :categories="categories" more-to="/articles" />
        <ArticleSection title="空状态" :articles="[]" :categories="categories" />
      </div>
      <div class="blog-sidebar space-y-6">
        <ProfileCard />
        <ArticleSearchCard />
        <RecommendedArticles :articles="articles" />
        <TagCloud :tags="tags" />
      </div>
    </div>
  </BlogLayout>
</template>