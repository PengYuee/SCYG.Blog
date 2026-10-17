<script setup lang="ts">
import { onUnmounted, ref, watch } from "vue"
import { useRouter } from "vue-router"
import BlogNavigation from "@/components/public/BlogNavigation.vue"
import { AUTHOR_NAME } from "@/content/profile"

/** 观察器生命周期所需的最小接口。 */
type ObserverHandle = Pick<IntersectionObserver, "observe" | "disconnect">
/** 可注入的边界观察器工厂。 */
export type ObserverFactory = (callback: IntersectionObserverCallback) => ObserverHandle
/** 固定头部在完整 Hero、短 Hero 与正文表面间的视觉状态。 */
type HeaderState = "hidden" | "transparent" | "scrolled"
/** 公共头部输入属性。 */
type Props = {
  /** 当前 Hero 的观察边界。 */ readonly boundary: Element | null
  /** 是否由完整 Hero 离开视口的时机控制固定头部。 */ readonly heroControlled?: boolean
  /** 是否在公共短 Hero 上使用始终可见的覆盖头部。 */ readonly publicOverlay?: boolean
  /** 测试可替换的观察器工厂。 */ readonly observerFactory?: ObserverFactory | undefined
  /** 首页两个互斥场景直接决定头部表面。 */ readonly scene?: "hero" | "content"
  readonly homeAction?: (() => void) | undefined
}

const props = withDefaults(defineProps<Props>(), { heroControlled: false, publicOverlay: false })
const router = useRouter()
/** 首页首帧隐藏，公共短 Hero 与带边界场景首帧透明，其他场景直接显示正文表面。 */
const headerState = ref<HeaderState>(props.heroControlled ? "hidden" : props.publicOverlay || props.boundary !== null ? "transparent" : "scrolled")
/** 当前活动观察器，由组件生命周期独占并释放。 */
let activeObserver: ObserverHandle | null = null
/** 当前由固定头部观察的 Hero。 */
let activeBoundary: Element | null = null

/** 以原生视口为边界观察完整 Hero，不预留固定头部高度。 */
const createObserver: ObserverFactory = (callback) => new IntersectionObserver(callback, { root: null, threshold: 0 })
/** 根据 Hero 底边返回固定头部状态。 */
const resolveHeaderState = (boundaryBottom: number): HeaderState => {
  const navigationHeight = Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--layout-nav-height")) || 0
  if (boundaryBottom <= navigationHeight) return "scrolled"
  return props.heroControlled ? "hidden" : "transparent"
}
/** 根据 Hero 底边同步固定头部状态，覆盖平滑滚动时观察器可能遗漏的临界通知。 */
const syncHeaderState = (): void => {
  if (activeBoundary !== null) headerState.value = resolveHeaderState(activeBoundary.getBoundingClientRect().bottom)
}
async function returnHome(event: MouseEvent): Promise<void> {
  if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  if (router.currentRoute.value.fullPath === "/") { window.scrollTo({ top: 0 }); return }
  const failure = await router.push("/")
  if (failure === undefined) window.scrollTo({ top: 0 })
}

watch(
  () => [props.boundary, props.heroControlled, props.publicOverlay, props.scene] as const,
  ([boundary, heroControlled, publicOverlay, scene]) => {
    activeObserver?.disconnect()
    activeObserver = null
    activeBoundary = null
    window.removeEventListener("scroll", syncHeaderState)
    if (scene !== undefined) {
      headerState.value = scene === "hero" ? "transparent" : "scrolled"
      return
    }
    if (boundary === null && !heroControlled && !publicOverlay) {
      headerState.value = "scrolled"
      return
    }

    headerState.value = heroControlled ? "hidden" : "transparent"
    if (boundary === null) return
    activeBoundary = boundary
    window.addEventListener("scroll", syncHeaderState, { passive: true })
    const factory = props.observerFactory ?? createObserver
    activeObserver = factory((entries) => {
      const entry = entries[0]
      /** Edge 可能把顶边零面积交界报告为相交，因此仅以 Hero 底边是否越过视口顶边为准。 */
      if (entry !== undefined) headerState.value = resolveHeaderState(entry.boundingClientRect.bottom)
    })
    activeObserver.observe(boundary)
  },
  { immediate: true },
)

/** 卸载时释放原生观察器，不保留页面级监听。 */
onUnmounted(() => {
  activeObserver?.disconnect()
  window.removeEventListener("scroll", syncHeaderState)
})
</script>

<template>
  <header
    v-show="headerState !== 'hidden'"
    class="fixed inset-x-0 top-0 z-50 h-[var(--layout-nav-height)]"
    :class="headerState === 'transparent' ? 'bg-transparent text-hero-text' : 'border-b border-border-subtle bg-surface text-text-primary shadow-[var(--shadow-nav)]'"
    :data-state="headerState"
    :aria-hidden="headerState === 'hidden' ? 'true' : undefined"
  >
    <div class="blog-shell flex h-full items-center justify-between gap-6">
      <a :href="router.resolve('/').href" class="shrink-0 rounded-[var(--radius-control)] font-[family-name:var(--font-family-display)] text-xl font-bold" @click="returnHome">{{ AUTHOR_NAME }}</a>
      <div v-if="headerState === 'scrolled' && $slots['search']" class="header-search min-w-0 flex-1">
        <slot name="search" />
      </div>
      <BlogNavigation :tone="headerState === 'transparent' ? 'overlay' : 'surface'" :home-action="homeAction" :home-current="scene === 'hero'" />
    </div>
  </header>
</template>

<style scoped>
.header-search { max-width: calc(var(--layout-reading-measure) - var(--space-24)); }
</style>
