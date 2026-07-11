<script setup lang="ts">
import { ArrowRightIcon, EyeIcon } from "@heroicons/vue/20/solid"
import { RouterLink } from "vue-router"
import type { ArticleSummary } from "@/types/article"

/** 推荐文章输入属性。 */
defineProps<{ readonly articles: readonly ArticleSummary[] }>()
</script>

<template>
  <section class="blog-card p-6" aria-labelledby="recommended-title">
    <h2 id="recommended-title" class="font-[family-name:var(--font-family-display)] text-xl font-semibold text-text-primary">推荐文章</h2>
    <ol v-if="articles.length > 0" class="mt-4 divide-y divide-border-subtle">
      <li v-for="(article, index) in articles" :key="article.id" class="py-3 first:pt-0 last:pb-0">
        <RouterLink :to="`/articles/${article.id}`" class="group flex min-h-11 items-center gap-3 rounded-lg text-text-primary hover:text-accent">
          <span class="font-[family-name:var(--font-family-display)] text-xl text-text-tertiary">{{ String(index + 1).padStart(2, "0") }}</span>
          <span class="min-w-0 flex-1 text-sm font-semibold">{{ article.title }}</span>
          <span class="inline-flex items-center gap-1 text-xs text-text-tertiary"><EyeIcon class="size-4" aria-hidden="true" />{{ article.visited }}</span>
          <ArrowRightIcon class="size-4 text-text-tertiary group-hover:text-accent" aria-hidden="true" />
        </RouterLink>
      </li>
    </ol>
    <p v-else class="mt-4 text-sm text-text-secondary">暂无推荐文章</p>
  </section>
</template>