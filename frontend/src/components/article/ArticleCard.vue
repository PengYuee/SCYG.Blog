<script setup lang="ts">
import { ChatBubbleLeftIcon, EyeIcon, HandThumbUpIcon, PhotoIcon } from "@heroicons/vue/20/solid"
import { ref } from "vue"
import { RouterLink } from "vue-router"
import type { ArticleSummary } from "@/types/article"
import type { ArticleType } from "@/types/taxonomy"

/** 文章卡片输入属性。 */
type Props = {
  /** 文章摘要。 */ readonly article: ArticleSummary
  /** 已解析的可选分类。 */ readonly category?: ArticleType | undefined
}

defineProps<Props>()
/** 分类图片是否加载失败。 */
const imageFailed = ref(false)
</script>

<template>
  <article class="blog-card group flex min-w-0 flex-col overflow-hidden">
    <div class="aspect-[16/10] overflow-hidden bg-surface-muted">
      <img v-if="category?.imageUrl && !imageFailed" :src="category.imageUrl" width="640" height="400" class="size-full object-cover transition duration-[var(--duration-standard)] group-hover:scale-[1.03]" :alt="`${category.name}分类配图`" loading="lazy" @error="imageFailed = true" />
      <div v-else class="flex size-full items-center justify-center text-text-tertiary" role="img" :aria-label="`${category?.name ?? '文章'}暂无分类配图`"><PhotoIcon class="size-10" aria-hidden="true" /></div>
    </div>
    <div class="flex flex-1 flex-col p-5">
      <p class="text-xs font-semibold uppercase tracking-wider text-accent">{{ category?.name ?? "未分类" }}</p>
      <h3 class="mt-2 text-pretty font-[family-name:var(--font-family-display)] text-xl font-semibold leading-[var(--line-height-h3)] text-text-primary">
        <RouterLink :to="`/articles/${article.id}`" class="rounded-sm group-hover:text-accent group-focus-within:text-accent">{{ article.title }}</RouterLink>
      </h3>
      <p class="mt-3 line-clamp-3 text-pretty text-sm leading-[var(--line-height-small)] text-text-secondary">{{ article.digest }}</p>
      <div class="mt-auto flex items-center justify-between gap-3 border-t border-border-subtle pt-4 text-xs text-text-tertiary">
        <time :datetime="article.createdAt">{{ article.createdAt.slice(0, 10) }}</time>
        <span class="flex items-center gap-3">
          <span class="inline-flex items-center gap-1" :aria-label="`${article.visited} 次阅读`"><EyeIcon class="size-4" aria-hidden="true" />{{ article.visited }}</span>
          <span class="inline-flex items-center gap-1" :aria-label="`${article.support} 个赞`"><HandThumbUpIcon class="size-4" aria-hidden="true" />{{ article.support }}</span>
          <span class="inline-flex items-center gap-1" :aria-label="`${article.comment} 条评论`"><ChatBubbleLeftIcon class="size-4" aria-hidden="true" />{{ article.comment }}</span>
        </span>
      </div>
    </div>
  </article>
</template>

<style scoped>
@media (prefers-reduced-motion: reduce) {
  img { transform: none !important; }
}
</style>