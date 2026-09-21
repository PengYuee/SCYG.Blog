<script setup lang="ts">
import { ChevronDownIcon } from "@heroicons/vue/24/outline"
import { ref } from "vue"
import BlogFooter from "@/components/public/BlogFooter.vue"
import BlogHeader from "@/components/public/BlogHeader.vue"
import type { ObserverFactory } from "@/components/public/BlogHeader.vue"
import BlogNavigation from "@/components/public/BlogNavigation.vue"
import HeroSearch from "@/components/public/HeroSearch.vue"
import HeroProfile from "@/components/public/HeroProfile.vue"

/** 首页完整 Hero 布局输入属性。 */
defineProps<{ readonly observerFactory?: ObserverFactory }>()

/** 完整 Hero 的可观察边界，避免 1px 边界在浏览器精确边缘的相交差异。 */
const heroBoundary = ref<HTMLElement | null>(null)
/** 项目 Hero 图片解码失败时切换到稳定可访问回退。 */
const heroImageFailed = ref(false)
/** Hero 下方主要内容边界。 */
const contentBoundary = ref<HTMLElement | null>(null)
/** 将 Hero 提示滚动到主要内容且不写入 URL hash。 */
const scrollToContent = (): void => {
  const behavior: ScrollBehavior = window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth"
  contentBoundary.value?.scrollIntoView({ behavior, block: "start" })
}
</script>

<template>
  <div data-layout="public" class="min-h-[100dvh] bg-canvas text-text-primary">
    <a href="#blog-content" class="fixed left-4 top-4 z-[60] -translate-y-24 rounded-lg bg-surface px-4 py-3 font-semibold text-accent shadow-[var(--shadow-nav)] focus:translate-y-0">跳到主要内容</a>
    <BlogHeader :boundary="heroBoundary" hero-controlled :observer-factory="observerFactory" />
    <section ref="heroBoundary" class="blog-hero public-hero relative isolate overflow-hidden" aria-labelledby="hero-profile-name">
      <img v-if="!heroImageFailed" data-testid="hero-image" :src="'/images/hero-starry.jpg'" alt="" aria-hidden="true" class="absolute inset-0 z-0 size-full object-cover opacity-0" @error="heroImageFailed = true" />
      <div v-else data-testid="hero-fallback" role="img" aria-label="星空背景暂时无法显示" class="absolute inset-0 z-0 bg-[var(--hero-background-color)]" />
      <div class="blog-shell hero-entrance relative z-10 flex h-full flex-col items-center justify-center gap-8 pt-[var(--layout-nav-height)]">
        <HeroProfile />
        <HeroSearch :content-target="contentBoundary" tone="hero" input-id="hero-search-input" />
        <BlogNavigation tone="hero" />
      </div>
      <!-- 按钮避免刷新地址携带 Hero hash，仅内部图标表达向下方向。 -->
      <button data-testid="hero-scroll-cue" type="button" class="scroll-cue absolute bottom-6 left-1/2 z-10 flex size-11 -translate-x-1/2 items-center justify-center rounded-full border" aria-label="向下浏览文章" @click="scrollToContent">
        <ChevronDownIcon class="scroll-cue-icon size-6" aria-hidden="true" />
      </button>
    </section>
    <div class="public-lower-page">
      <main id="blog-content" ref="contentBoundary" class="public-content" tabindex="-1">
        <div class="blog-shell"><slot /></div>
      </main>
      <BlogFooter />
    </div>
  </div>
</template>

<style scoped>
.public-hero {
  height: 100dvh;
  color: var(--color-hero-text);
}
/* 正文与页脚共同占满第二屏，内容不足时由正文填充页脚之外的剩余空间。 */
.public-lower-page {
  display: flex;
  min-height: 100dvh;
  flex-direction: column;
}
.public-content {
  flex: 1;
  padding-block: calc(var(--layout-nav-height) + var(--space-2)) var(--space-16);
}
.hero-entrance { animation: hero-enter var(--duration-slow) var(--ease-standard) both; }
.scroll-cue {
  color: var(--color-hero-text-muted);
  border-color: var(--color-hero-border);
  background: var(--color-hero-surface);
  backdrop-filter: blur(var(--space-3));
}
.scroll-cue:hover {
  color: var(--color-hero-text);
  background: var(--color-hero-surface-hover);
}
.scroll-cue:active { background: var(--color-hero-surface-active); }
/* 滚动提示只动画内部图标，避免定位锚点产生位移。 */
.scroll-cue-icon { animation: scroll-cue var(--duration-scroll-cue) var(--ease-standard) infinite; }
@keyframes hero-enter {
  from { opacity: 0; transform: translateY(var(--space-4)); }
  to { opacity: 1; transform: translateY(0); }
}
@keyframes scroll-cue {
  0%, 100% { opacity: 0.55; transform: translateY(calc(var(--space-1) * -1)); }
  50% { opacity: 1; transform: translateY(var(--space-1)); }
}
@media (prefers-reduced-motion: reduce) {
  .public-content { padding-block: calc(var(--layout-nav-height) + var(--space-2)) var(--space-16); }
  .hero-entrance { animation: none; transform: none; }
  .scroll-cue-icon { animation: none; transform: none; }
}
</style>
