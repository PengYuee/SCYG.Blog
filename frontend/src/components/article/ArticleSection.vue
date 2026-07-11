<script setup lang="ts">
import { ArrowRightIcon } from "@heroicons/vue/20/solid"
import { RouterLink } from "vue-router"
import ArticleCard from "@/components/article/ArticleCard.vue"
import type { ArticleSummary } from "@/types/article"
import type { ArticleType } from "@/types/taxonomy"

/** 文章分区输入属性。 */
type Props = {
  /** 分区标题。 */ readonly title: string
  /** 分区文章。 */ readonly articles: readonly ArticleSummary[]
  /** 用于卡片展示的分类字典。 */ readonly categories: readonly ArticleType[]
  /** 可选更多文章目标。 */ readonly moreTo?: string
}

const props = defineProps<Props>()

/**
 * 从已解析字典中查找文章分类。
 * @param article 当前文章。
 * @returns 匹配分类或 undefined。
 */
const categoryFor = (article: ArticleSummary): ArticleType | undefined => props.categories.find((category) => category.id === article.articleTypeId)
</script>

<template>
  <section :aria-labelledby="`article-section-${title}`">
    <div class="mb-6 flex items-end justify-between gap-6">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.18em] text-accent">ARTICLES</p>
        <h2 :id="`article-section-${title}`" class="mt-1 text-balance font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h2)] font-bold leading-[var(--line-height-h2)] text-text-primary">{{ title }}</h2>
      </div>
      <RouterLink v-if="moreTo" :to="moreTo" class="inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm font-semibold text-text-secondary hover:bg-surface-hover hover:text-accent">更多文章<ArrowRightIcon class="size-4" aria-hidden="true" /></RouterLink>
    </div>
    <div v-if="articles.length > 0" data-testid="article-grid" class="blog-card-grid">
      <ArticleCard v-for="article in articles" :key="article.id" :article="article" :category="categoryFor(article)" />
    </div>
    <div v-else class="flex min-h-48 items-center justify-center rounded-[var(--radius-card)] border border-border-subtle bg-surface p-8 text-center text-sm text-text-secondary shadow-[var(--shadow-card)]" role="status">暂无文章</div>
  </section>
</template>