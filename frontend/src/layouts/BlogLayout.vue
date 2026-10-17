<script setup lang="ts">
import { ArrowRightEndOnRectangleIcon } from "@heroicons/vue/24/outline"
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue"
import BlogFooter from "@/components/public/BlogFooter.vue"
import BlogHeader from "@/components/public/BlogHeader.vue"
import type { ObserverFactory } from "@/components/public/BlogHeader.vue"
import HeroSearch from "@/components/public/HeroSearch.vue"
import HeroProfile from "@/components/public/HeroProfile.vue"

/** 首页完整 Hero 布局输入属性。 */
defineProps<{ readonly observerFactory?: ObserverFactory; readonly heroControlled?: boolean }>()
const contentVisible = defineModel<boolean>("contentVisible", { default: false })

/** 完整 Hero 的可观察边界，避免 1px 边界在浏览器精确边缘的相交差异。 */
const heroBoundary = ref<HTMLElement | null>(null)
/** 项目 Hero 图片解码失败时切换到稳定可访问回退。 */
const heroImageFailed = ref(false)
/** Hero 下方主要内容边界。 */
const contentBoundary = ref<HTMLElement | null>(null)
let readingPosition = 0
let wheelTail = false
let wheelIdle: ReturnType<typeof setTimeout> | undefined
/** 同一个手势的惯性尾部不能继续滚动刚出现的内容。 */
function consumeWheelTail(): void {
  wheelTail = true
  clearTimeout(wheelIdle)
  wheelIdle = setTimeout(() => { wheelTail = false }, 150)
}
async function enterContent(): Promise<void> {
  contentVisible.value = true
  await nextTick()
  window.scrollTo({ top: readingPosition, behavior: "instant" })
}
async function skipToContent(): Promise<void> {
  contentVisible.value = true
  await nextTick()
  window.scrollTo({ top: 0, behavior: "instant" })
  contentBoundary.value?.focus({ preventScroll: true })
}
function returnToHero(): void { contentVisible.value = false }
function handleWheel(event: WheelEvent): void {
  if (event.ctrlKey || Math.abs(event.deltaX) > Math.abs(event.deltaY) || event.deltaY === 0) return
  if (wheelTail) { event.preventDefault(); consumeWheelTail(); return }
  if (contentVisible.value || event.deltaY < 0) return
  const target = event.target
  if (target instanceof Element && target.closest('form,button,a,input,textarea,select,[contenteditable],[role="dialog"]')) return
  if (document.querySelector('[role="dialog"][aria-modal="true"]') !== null) return
  event.preventDefault()
  void enterContent()
}
watch(contentVisible, async (visible) => {
  if (!visible) readingPosition = window.scrollY
  consumeWheelTail()
  await nextTick()
  if (!visible) window.scrollTo({ top: 0, behavior: "instant" })
  if (visible) contentBoundary.value?.focus({ preventScroll: true })
  else heroBoundary.value?.focus({ preventScroll: true })
})
onMounted(() => window.addEventListener("wheel", handleWheel, { passive: false }))
onBeforeUnmount(() => {
  window.removeEventListener("wheel", handleWheel)
  clearTimeout(wheelIdle)
})
</script>

<template>
  <div data-layout="public" :data-scene="contentVisible ? 'content' : 'hero'" class="home-scenes min-h-[100dvh] bg-canvas text-text-primary">
    <a href="#blog-content" class="fixed left-4 top-4 z-[60] -translate-y-24 rounded-lg bg-surface px-4 py-3 font-semibold text-accent shadow-[var(--shadow-nav)] focus:translate-y-0" @click.prevent="skipToContent">跳到主要内容</a>
    <BlogHeader :boundary="heroBoundary" :hero-controlled="heroControlled ?? false" :observer-factory="observerFactory" :scene="contentVisible ? 'content' : 'hero'" :home-action="returnToHero" />
    <section v-show="!contentVisible" ref="heroBoundary" tabindex="-1" class="blog-hero public-hero relative isolate overflow-hidden" aria-labelledby="hero-profile-name">
      <img v-if="!heroImageFailed" data-testid="hero-image" :src="'/images/hero-starry.jpg'" alt="" aria-hidden="true" class="absolute inset-0 z-0 size-full object-cover opacity-0" @error="heroImageFailed = true" />
      <div v-else data-testid="hero-fallback" role="img" aria-label="星空背景暂时无法显示" class="absolute inset-0 z-0 bg-[var(--hero-background-color)]" />
      <div class="blog-shell hero-entrance relative z-10 flex h-full flex-col items-center justify-center gap-8 pt-[var(--layout-nav-height)]">
        <HeroProfile />
        <slot name="hero-search"><HeroSearch /></slot>
      </div>
      <button data-testid="hero-scroll-cue" type="button" class="scroll-cue absolute bottom-6 left-1/2 z-10 flex min-h-12 -translate-x-1/2 items-center justify-center gap-2 rounded-full border px-5 text-sm font-semibold" aria-label="进入内容区" @click="enterContent">
        <ArrowRightEndOnRectangleIcon class="size-5" aria-hidden="true" />进入内容区
      </button>
    </section>
    <div v-show="contentVisible" class="public-lower-page">
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
.home-scenes[data-scene="hero"] { height: 100dvh; overflow: hidden; }
/* 正文与页脚共同占满第二屏，内容不足时由正文填充页脚之外的剩余空间。 */
.public-lower-page {
  display: flex;
  min-height: 100dvh;
  flex-direction: column;
  animation: scene-enter var(--duration-fast) var(--ease-standard) both;
}
.public-content {
  flex: 1;
  padding-block: calc(var(--layout-nav-height) + var(--space-2)) var(--space-16);
}
.hero-entrance {
  gap: var(--space-6);
  padding-bottom: var(--space-24);
  animation: hero-enter var(--duration-slow) var(--ease-standard) both;
}
@media (max-height: 800px) {
  .hero-entrance { gap: var(--space-4); padding-bottom: calc(var(--space-24) + var(--space-8)); }
  .hero-entrance :deep([aria-labelledby="hero-profile-name"] > img) { width: var(--space-16); height: var(--space-16); }
  .hero-entrance :deep(#hero-profile-name) { margin-top: var(--space-3); font-size: var(--font-size-h1); }
  .hero-entrance :deep([aria-labelledby="hero-profile-name"] > p),
  .hero-entrance :deep([aria-labelledby="hero-profile-name"] > nav),
  .hero-entrance :deep([aria-labelledby="hero-profile-name"] > button) { margin-top: var(--space-2); }
  .hero-entrance :deep([aria-labelledby="hero-profile-name"] > figure) { margin-top: var(--space-3); }
  .hero-entrance :deep(blockquote) { font-size: var(--font-size-body); }
}
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
@keyframes scene-enter { from { opacity: 0; } to { opacity: 1; } }
@keyframes hero-enter {
  from { opacity: 0; transform: translateY(var(--space-4)); }
  to { opacity: 1; transform: translateY(0); }
}
@media (prefers-reduced-motion: reduce) {
  .public-content { padding-block: calc(var(--layout-nav-height) + var(--space-2)) var(--space-16); }
  .hero-entrance { animation: none; transform: none; }
  .public-lower-page { animation: none; }
}
</style>
