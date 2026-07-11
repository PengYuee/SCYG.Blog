<script setup lang="ts">
import { MagnifyingGlassIcon } from "@heroicons/vue/24/outline"
import { ref } from "vue"
import { useRoute, useRouter } from "vue-router"

/** Hero 搜索输入属性。 */
type Props = {
  /** 首页 Hero 下方的原生内容边界。 */ readonly contentTarget?: HTMLElement | null
  /** 控件所在表面。 */ readonly tone?: "hero" | "surface"
  /** 页面内唯一输入框标识。 */ readonly inputId?: string
}

const props = withDefaults(defineProps<Props>(), { contentTarget: null, tone: "hero", inputId: "hero-search-input" })
/** 搜索框的受控关键词。 */
const query = ref("")
/** 当前 Vue Router 实例。 */
const router = useRouter()
/** 提交搜索前的当前路由。 */
const route = useRoute()

/**
 * 提交清理后的搜索词，并在首页使用原生滚动进入内容。
 * @returns 导航完成后的 Promise。
 */
const submitSearch = async (): Promise<void> => {
  const normalizedQuery = query.value.trim()
  if (normalizedQuery.length === 0) return

  const startedOnHome = route.path === "/"
  await router.push({ path: "/articles", query: { q: normalizedQuery } })
  if (startedOnHome) {
    const reducedMotion = typeof window.matchMedia === "function" && window.matchMedia("(prefers-reduced-motion: reduce)").matches
    props.contentTarget?.scrollIntoView({ behavior: reducedMotion ? "auto" : "smooth", block: "start" })
  }
}
</script>

<template>
  <form class="mx-auto w-full max-w-2xl" role="search" aria-label="搜索文章" @submit.prevent="submitSearch">
    <label class="sr-only" :for="inputId">搜索文章</label>
    <div class="flex min-h-12 items-center rounded-full border p-1 shadow-[var(--shadow-card)]" :class="tone === 'hero' ? 'hero-search-surface' : 'border-border bg-surface'">
      <MagnifyingGlassIcon class="ml-4 size-5 shrink-0" :class="tone === 'hero' ? 'text-[color:var(--color-hero-text-muted)]' : 'text-text-tertiary'" aria-hidden="true" />
      <input :id="inputId" v-model="query" type="search" class="min-w-0 flex-1 bg-transparent px-3 py-2 outline-none" :class="tone === 'hero' ? 'hero-search-input' : 'text-text-primary placeholder:text-text-tertiary'" placeholder="搜索文章" autocomplete="off" />
      <button type="submit" class="min-h-11 rounded-full bg-accent px-6 text-sm font-semibold text-[color:var(--color-hero-text)] hover:bg-accent-hover active:scale-[0.98]">搜索</button>
    </div>
  </form>
</template>

<style scoped>
.hero-search-surface {
  border-color: color-mix(in srgb, var(--color-hero-text) 40%, transparent);
  background: color-mix(in srgb, var(--color-hero-text) 15%, transparent);
  backdrop-filter: blur(var(--space-3));
}
.hero-search-input { color: var(--color-hero-text); }
.hero-search-input::placeholder { color: var(--color-hero-text-muted); }
</style>