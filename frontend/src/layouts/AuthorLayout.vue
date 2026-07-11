<script setup lang="ts">
import { RouterLink, RouterView } from "vue-router"
</script>

<template>
  <div data-layout="author" class="min-h-[100dvh] bg-canvas text-text-primary">
    <a href="#author-main" class="sr-only focus:not-sr-only">跳到写作区域</a>
    <header class="border-b border-border-subtle bg-surface">
      <nav class="mx-auto flex h-[var(--layout-nav-height)] max-w-[var(--layout-container)] items-center justify-between px-6" aria-label="作者导航">
        <RouterLink to="/" class="font-display text-xl font-semibold">SCYG 写作台</RouterLink>
        <div class="flex gap-2 text-sm"><RouterLink class="rounded-lg px-4 py-3 hover:bg-surface-hover" to="/author/articles/new">新文章</RouterLink><RouterLink class="rounded-lg px-4 py-3 hover:bg-surface-hover" to="/author/taxonomy">分类与标签</RouterLink></div>
      </nav>
    </header>
    <main id="author-main" class="mx-auto max-w-[var(--layout-container)] px-6 py-8"><RouterView /></main>
  </div>
</template>
