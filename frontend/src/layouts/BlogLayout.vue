<script setup lang="ts">
import { ref } from "vue"
import BlogFooter from "@/components/public/BlogFooter.vue"
import BlogHeader from "@/components/public/BlogHeader.vue"
import type { ObserverFactory } from "@/components/public/BlogHeader.vue"
import HeroProfile from "@/components/public/HeroProfile.vue"
import HeroSearch from "@/components/public/HeroSearch.vue"
import WaveDivider from "@/components/public/WaveDivider.vue"

/** 公共布局输入属性。 */
defineProps<{ readonly observerFactory?: ObserverFactory }>()

/** Hero 底部的可观察边界。 */
const heroBoundary = ref<HTMLElement | null>(null)
/** Hero 下方主要内容边界。 */
const contentBoundary = ref<HTMLElement | null>(null)
/** 项目图片加载失败状态。 */
const heroImageFailed = ref(false)
</script>

<template>
  <div data-layout="public" class="min-h-[100dvh] bg-canvas text-text-primary">
    <a href="#blog-content" class="fixed left-4 top-4 z-[60] -translate-y-24 rounded-lg bg-surface px-4 py-3 font-semibold text-accent shadow-[var(--shadow-nav)] focus:translate-y-0">跳到主要内容</a>
    <BlogHeader :boundary="heroBoundary" :observer-factory="observerFactory" />
    <section class="public-hero relative isolate overflow-hidden" aria-labelledby="hero-profile-name">
      <img v-show="!heroImageFailed" :src="'/images/hero-starry.jpg'" width="1920" height="1080" class="absolute inset-0 -z-20 size-full object-cover" alt="" fetchpriority="high" data-testid="hero-image" @error="heroImageFailed = true" />
      <div class="absolute inset-0 -z-10 bg-overlay" aria-hidden="true" />
      <div v-if="heroImageFailed" data-testid="hero-fallback" class="absolute inset-0 -z-20 bg-text-primary" role="img" aria-label="星空背景暂时无法显示" />
      <div class="blog-shell hero-entrance flex h-full flex-col items-center justify-center gap-8 pb-[var(--layout-wave-height)] pt-[var(--layout-nav-height)]">
        <HeroProfile />
        <HeroSearch :content-target="contentBoundary" />
      </div>
      <div ref="heroBoundary" class="absolute inset-x-0 bottom-[var(--layout-wave-height)] h-px" aria-hidden="true" />
      <WaveDivider />
    </section>
    <main id="blog-content" ref="contentBoundary" class="public-content" tabindex="-1">
      <div class="blog-shell"><slot /></div>
    </main>
    <BlogFooter />
  </div>
</template>

<style scoped>
.public-hero {
  height: var(--layout-hero-height);
  height: 100dvh;
  color: var(--color-hero-text);
}
.public-content { padding-block: calc(var(--layout-nav-height) + var(--space-2)) var(--space-16); }
.hero-entrance { animation: hero-enter var(--duration-slow) var(--ease-standard) both; }
@keyframes hero-enter {
  from { opacity: 0; transform: translateY(var(--space-4)); }
  to { opacity: 1; transform: translateY(0); }
}
@media (prefers-reduced-motion: reduce) {
  .public-content { padding-block: calc(var(--layout-nav-height) + var(--space-2)) var(--space-16); }
.hero-entrance { animation: none; transform: none; }
}
</style>