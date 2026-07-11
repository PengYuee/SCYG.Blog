<script setup lang="ts">
import { HomeIcon, NewspaperIcon } from "@heroicons/vue/24/outline"
import { onUnmounted, ref, watch } from "vue"
import { RouterLink, useRoute } from "vue-router"
import { AUTHOR_NAME } from "@/content/profile"

/** 观察器生命周期所需的最小接口。 */
type ObserverHandle = Pick<IntersectionObserver, "observe" | "disconnect">
/** 可注入的边界观察器工厂。 */
export type ObserverFactory = (callback: IntersectionObserverCallback) => ObserverHandle
/** 头部视觉状态。 */
type HeaderState = "rest" | "transparent" | "scrolled"
/** 公共头部输入属性。 */
type Props = {
  /** Hero 底部观察边界。 */ readonly boundary: Element | null
  /** 测试可替换的观察器工厂。 */ readonly observerFactory?: ObserverFactory | undefined
}

const props = defineProps<Props>()
/** 当前头部状态；rest 是边界尚未报告时的稳定初始态。 */
const headerState = ref<HeaderState>("rest")
/** 当前路由用于标注导航选中态。 */
const route = useRoute()
/** 当前活动观察器，由组件生命周期独占并释放。 */
let activeObserver: ObserverHandle | null = null

/** 默认使用浏览器原生 IntersectionObserver。 */
const createObserver: ObserverFactory = (callback) => new IntersectionObserver(callback, { threshold: 0 })

watch(
  () => props.boundary,
  (boundary) => {
    activeObserver?.disconnect()
    activeObserver = null
    if (boundary === null) return

    const factory = props.observerFactory ?? createObserver
    activeObserver = factory((entries) => {
      const entry = entries[0]
      if (entry !== undefined) headerState.value = entry.isIntersecting ? "transparent" : "scrolled"
    })
    activeObserver.observe(boundary)
  },
  { immediate: true },
)

/** 卸载时释放原生观察器，不保留页面级监听。 */
onUnmounted(() => activeObserver?.disconnect())
</script>

<template>
  <header class="fixed inset-x-0 top-0 z-50 h-[var(--layout-nav-height)] transition" :class="headerState === 'scrolled' || headerState === 'rest' ? 'border-b border-border-subtle bg-surface/95 text-text-primary shadow-[var(--shadow-nav)] backdrop-blur-md' : 'bg-transparent text-[color:var(--color-hero-text)]'" :data-state="headerState">
    <div class="blog-shell flex h-full items-center justify-between">
      <RouterLink to="/" class="rounded-lg font-[family-name:var(--font-family-display)] text-xl font-bold">{{ AUTHOR_NAME }}</RouterLink>
      <nav class="flex h-full items-center gap-2" aria-label="主要导航">
        <RouterLink to="/" class="relative inline-flex min-h-11 items-center gap-2 nav-link rounded-lg px-4 text-sm font-semibold" :class="route.path === '/' ? 'after:absolute after:inset-x-4 after:bottom-2 after:h-0.5 after:bg-accent' : ''" :aria-current="route.path === '/' ? 'page' : undefined"><HomeIcon class="size-5" aria-hidden="true" />首页</RouterLink>
        <RouterLink to="/articles" class="relative inline-flex min-h-11 items-center gap-2 nav-link rounded-lg px-4 text-sm font-semibold" :class="route.path.startsWith('/articles') ? 'after:absolute after:inset-x-4 after:bottom-2 after:h-0.5 after:bg-accent' : ''" :aria-current="route.path.startsWith('/articles') ? 'page' : undefined"><NewspaperIcon class="size-5" aria-hidden="true" />文章</RouterLink>
      </nav>
    </div>
  </header>
</template>

<style scoped>
.nav-link:hover { background: var(--color-surface-hover); }
header[data-state="transparent"] .nav-link:hover {
  background: color-mix(in srgb, var(--color-hero-text) 10%, transparent);
}
</style>