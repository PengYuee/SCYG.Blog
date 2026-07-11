<script setup lang="ts">
import { WrenchScrewdriverIcon } from "@heroicons/vue/24/outline"
import { RouterLink } from "vue-router"
</script>

<template>
  <main data-layout="admin" class="flex min-h-[100dvh] items-center justify-center bg-canvas p-8 text-text-primary">
    <section class="blog-card max-w-2xl p-12 text-center" aria-labelledby="admin-unavailable-title">
      <span class="mx-auto inline-flex size-16 items-center justify-center rounded-full bg-accent-soft text-accent">
        <WrenchScrewdriverIcon class="size-8" aria-hidden="true" />
      </span>
      <p class="mt-6 text-xs font-semibold uppercase tracking-[0.18em] text-accent">ADMIN BOUNDARY</p>
      <h1 id="admin-unavailable-title" class="mt-2 text-balance font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h1)] font-bold leading-[var(--line-height-h1)]">管理后台暂不可用</h1>
      <p class="mx-auto mt-4 max-w-xl text-pretty text-text-secondary">当前版本只保留管理域路由边界，不提供仪表盘、套餐或订阅功能。</p>
      <RouterLink to="/" class="mt-8 inline-flex min-h-11 items-center rounded-[var(--radius-control)] bg-accent px-6 font-semibold text-[color:var(--color-hero-text)] hover:bg-accent-hover active:scale-[0.98]">返回博客首页</RouterLink>
    </section>
  </main>
</template>
