<script setup lang="ts">
import { MagnifyingGlassIcon } from "@heroicons/vue/24/outline"
import HeroSearch from "@/components/public/HeroSearch.vue"
</script>

<template>
  <section class="blog-card p-6" aria-labelledby="article-search-title">
    <div class="mb-4 flex items-center gap-3">
      <span class="inline-flex size-11 items-center justify-center rounded-full bg-accent-soft text-accent"><MagnifyingGlassIcon class="size-5" aria-hidden="true" /></span>
      <h2 id="article-search-title" class="font-[family-name:var(--font-family-display)] text-xl font-semibold text-text-primary">文章搜索</h2>
    </div>
    <HeroSearch tone="surface" input-id="article-search-input" />
  </section>
</template>