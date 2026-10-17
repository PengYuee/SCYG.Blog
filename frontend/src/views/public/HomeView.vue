<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue"
import { useRouter } from "vue-router"
import { SparklesIcon, ArrowPathIcon, XMarkIcon, MagnifyingGlassIcon } from "@heroicons/vue/24/outline"
import ArticleSection from "@/components/article/ArticleSection.vue"
import type { ObserverFactory } from "@/components/public/BlogHeader.vue"
import HeroSearch from "@/components/public/HeroSearch.vue"
import ProfileCard from "@/components/public/ProfileCard.vue"
import RecommendedArticles from "@/components/public/RecommendedArticles.vue"
import ArticleFilterCard from "@/components/public/ArticleFilterCard.vue"
import PublicSearchResult from "@/components/ai/PublicSearchResult.vue"
import AppModal from "@/components/shared/AppModal.vue"
import BlogLayout from "@/layouts/BlogLayout.vue"
import { useHomeArticles } from "@/services/home-articles"
import { usePublicAiSearch } from "@/services/public-ai-search"
import { serializeArticleListQuery, type ArticleListQuery } from "@/router/query"
import { selectRecommendations } from "@/utils/search"
import type { ArticleFeed } from "@/stores/article-feed"
import type { Taxonomy } from "@/stores/taxonomy"
import "@/assets/ai.css"

const props = defineProps<{ readonly articleFeed?: ArticleFeed; readonly taxonomy?: Taxonomy; readonly observerFactory?: ObserverFactory }>()
const router = useRouter()
const initialScene = router.options.history.state["homeScene"]
const contentVisible = ref(initialScene === "content" || initialScene !== "hero" && (Object.keys(router.currentRoute.value.query).length > 0 || ["#home-results", "#blog-content"].includes(router.currentRoute.value.hash)))
function rememberScene(): void {
  if (router.currentRoute.value.path === "/") router.options.history.replace(router.currentRoute.value.fullPath, { homeScene: contentVisible.value ? "content" : "hero" })
}
watch([contentVisible, () => router.currentRoute.value.fullPath], rememberScene, { flush: "post" })
onMounted(rememberScene)
const { feedState, taxonomyState, parsed, filters, categories, tags, loadFeed, retryFeed, retryTaxonomy } = useHomeArticles(props)
const ai = usePublicAiSearch()
const { query: question, maxResults, busy, canSubmit, pending, pendingExpired, error, waitSeconds, lastRun } = ai
const mode = ref<"normal" | "ai">("normal")
const keyword = ref("")
const runId = ref<string>()
const showSubmissionState = ref(false)
const confirmAbandon = ref(false)
const abandonTrigger = ref<HTMLButtonElement | null>(null)
const resultRegion = ref<HTMLElement | null>(null)
const recommendations = computed(() => selectRecommendations(feedState.value.items))
const activeFilters = computed(() => filters.value !== undefined && (filters.value.q !== "" || filters.value.articleTypeId !== undefined || filters.value.tagId !== undefined))
const title = computed(() => filters.value?.q ? `“${filters.value.q}”的搜索结果` : activeFilters.value ? "筛选文章" : "最新文章")
const layoutProps = computed(() => props.observerFactory === undefined ? {} : { observerFactory: props.observerFactory })
const searchProps = computed(() => ({ busy: busy.value, locked: busy.value || pending.value !== undefined, canSubmit: canSubmit.value, queryMaxChars: ai.limits.queryMaxChars, resultLimit: ai.limits.maxResults, retrying: pending.value !== undefined }))
const showRunResults = computed(() => runId.value !== undefined && !showSubmissionState.value && !ai.invalidated.value && !pendingExpired.value)
const aiDescription = computed(() => {
  if (busy.value) return "正在检索站内相关文章…"
  if (ai.invalidated.value) return "搜索会话已失效，请开始新的搜索。"
  if (waitSeconds.value) return `搜索繁忙，请在 ${waitSeconds.value} 秒后重试。`
  if (pending.value) return pendingExpired.value ? "上次搜索尚未确认，请处理后再开始新的搜索。" : "上次搜索尚未确认，可在右侧重试。"
  if (error.value) return "搜索未完成，请在右侧重试。"
  return "在右侧输入问题，AI 会帮你查找相关文章。"
})
let appliedQuery = ""
let routeRevision = 0
const pageNavigations = new Set<string>()
const removeNavigationGuard = router.beforeEach((to) => {
  if (!pageNavigations.delete(to.fullPath)) { routeRevision += 1; showSubmissionState.value = false }
})
onBeforeUnmount(removeNavigationGuard)
watch(parsed, (result) => {
  if (!result.success) return
  mode.value = result.data.mode
  if (result.data.q !== appliedQuery) { keyword.value = result.data.q; appliedQuery = result.data.q }
  runId.value = result.data.runId
}, { immediate: true })

async function showSearchResults(): Promise<void> {
  contentVisible.value = true
  await nextTick()
  window.scrollTo({ top: 0, behavior: "instant" })
}
async function navigate(filtersValue: ArticleListQuery, selectedMode = mode.value): Promise<void> {
  const target = { path: "/", query: {
    ...serializeArticleListQuery(filtersValue),
    ...(selectedMode === "ai" ? { mode: "ai" } : {}),
    ...(runId.value === undefined ? {} : { runId: runId.value }),
  } }
  const destination = router.resolve(target).fullPath
  pageNavigations.add(destination)
  try {
    await router.push(target)
  } finally { pageNavigations.delete(destination) }
}
async function switchMode(value: "normal" | "ai"): Promise<void> {
  mode.value = value
  if (parsed.value.success) await navigate(parsed.value.data, value)
}
async function submit(): Promise<void> {
  if (!parsed.value.success) return
  if (mode.value === "normal") {
    await navigate({ ...parsed.value.data, q: keyword.value.trim() }, "normal")
    await showSearchResults()
    return
  }
  if (!canSubmit.value) return
  const submissionRevision = routeRevision
  showSubmissionState.value = true
  await showSearchResults()
  if (submissionRevision !== routeRevision) return
  const created = await ai.submit()
  if (created === undefined || submissionRevision !== routeRevision || !parsed.value.success) return
  runId.value = created
  // 模式切换保留任务；历史导航后的迟到结果只留在上次搜索，不覆盖用户地址。
  await navigate(parsed.value.data, mode.value)
  showSubmissionState.value = false
}
async function filterCategory(id?: number): Promise<void> {
  if (!parsed.value.success) return
  await navigate({ ...parsed.value.data, articleTypeId: id }, "normal")
}
async function filterTag(id?: number): Promise<void> {
  if (!parsed.value.success) return
  await navigate({ ...parsed.value.data, tagId: id }, "normal")
}
async function clearFilters(): Promise<void> {
  keyword.value = ""
  await navigate({ q: "" }, "normal")
}
async function tryAi(): Promise<void> {
  if (question.value.trim() === "") question.value = filters.value?.q ?? ""
  await switchMode("ai")
}
async function restartAi(): Promise<void> {
  ai.restart()
  runId.value = undefined
  if (parsed.value.success) await navigate(parsed.value.data, "ai")
}
async function restoreAbandonFocus(): Promise<void> {
  await nextTick()
  if (abandonTrigger.value !== null && !abandonTrigger.value.disabled) abandonTrigger.value.focus()
  else document.getElementById(contentVisible.value ? "content-search-input" : "hero-search-input")?.focus()
}
</script>

<template>
  <BlogLayout v-model:content-visible="contentVisible" v-bind="layoutProps" class="home-page">
    <template #hero-search>
      <HeroSearch v-model:mode="mode" v-model:keyword="keyword" v-model:question="question" v-model:max-results="maxResults" v-bind="searchProps" @update:mode="switchMode" @submit="submit" />
    </template>
    <div ref="resultRegion" class="home-content-grid" id="home-results">
      <aside class="home-author-rail" aria-label="作者与推荐文章">
        <ProfileCard />
        <div data-testid="recommended-articles"><RecommendedArticles :articles="recommendations" /></div>
      </aside>

      <div class="home-result-column min-w-0">
        <section v-if="!parsed.success" class="blog-card p-8" role="alert" data-testid="query-error">
          <h2 class="text-xl font-semibold">搜索参数无效</h2><p class="mt-3 text-text-secondary">请检查搜索方式、分类、标签或任务标识。</p>
          <RouterLink to="/" class="home-text-action mt-4">返回首页</RouterLink>
        </section>
        <div v-show="parsed.success && mode === 'normal'" class="space-y-6">
          <ArticleSection :title="title" :articles="feedState.items" :categories="categories" list>
            <template #description>
              <div class="search-result-summary">
                <p data-testid="home-announcement" class="search-result-note" role="status">已加载 {{ feedState.items.length }} 篇文章</p>
                <button v-if="activeFilters" type="button" class="search-result-action" @click="clearFilters"><XMarkIcon aria-hidden="true" />清除条件</button>
              </div>
            </template>
            <template #empty>
              <div v-if="feedState.kind === 'loading' || feedState.kind === 'idle'" data-testid="feed-loading" class="blog-card grid min-h-48 place-content-center p-8 text-text-secondary" role="status">正在加载文章…</div>
              <section v-else-if="feedState.kind === 'empty'" data-testid="feed-empty" class="blog-card px-8 py-12 text-center">
                <MagnifyingGlassIcon class="mx-auto mb-4 size-8 text-text-tertiary" aria-hidden="true" />
                <h3 class="font-display text-xl font-semibold">{{ activeFilters ? "没有找到匹配的文章" : "暂无已发布文章" }}</h3>
                <p class="mt-3 text-sm text-text-secondary">换个关键词，或让 AI 帮你从站内文章中寻找线索。</p>
                <button type="button" class="home-text-action mx-auto mt-4" @click="tryAi"><SparklesIcon class="size-5" aria-hidden="true" />用 AI 搜索这个问题</button>
              </section>
            </template>
          </ArticleSection>
          <section v-if="feedState.kind === 'error'" data-testid="feed-error" class="blog-card p-6" role="alert">
            <h2 class="font-semibold">{{ feedState.items.length ? "加载更多失败" : "文章加载失败" }}</h2>
            <p class="mt-2 text-sm text-text-secondary">{{ feedState.message }}</p>
            <button data-testid="feed-retry" type="button" class="home-text-action mt-3" @click="retryFeed"><ArrowPathIcon class="size-4" aria-hidden="true" />重试文章</button>
          </section>
          <div class="text-center">
            <button v-if="feedState.kind === 'ready' && !feedState.endReached" data-testid="load-more" type="button" class="home-load-more" @click="loadFeed">加载更多</button>
            <p v-else-if="feedState.kind === 'loading' && feedState.items.length" class="text-sm text-text-secondary" role="status">正在加载更多文章…</p>
            <p v-else-if="feedState.kind === 'ready' && feedState.endReached" class="text-sm text-text-tertiary">已展示全部文章</p>
          </div>
        </div>

        <section v-show="parsed.success && mode === 'ai'" aria-label="公共 AI 搜索">
          <PublicSearchResult v-if="runId" v-show="showRunResults" :key="runId" :run-id="runId" />
          <ArticleSection v-if="!showRunResults" title="最新文章" :articles="[]" :categories="categories" list>
            <template #description>
              <div class="search-result-summary" :aria-busy="busy">
                <p class="search-result-note" role="status" data-testid="ai-submission-status"><ArrowPathIcon v-if="busy" class="search-result-spinner" aria-hidden="true" />{{ aiDescription }}</p>
                <button v-if="pendingExpired" ref="abandonTrigger" data-testid="abandon-public-search" type="button" class="search-result-action" @click="confirmAbandon = true">处理上次搜索</button>
                <button v-else-if="ai.invalidated.value" type="button" class="search-result-action" @click="restartAi">开始新的搜索</button>
                <button v-else-if="!busy && !pending && !error && lastRun" type="button" class="search-result-action" @click="showSubmissionState = false; runId = lastRun; parsed.success && navigate(parsed.data, 'ai')">查看上次结果</button>
              </div>
            </template>
            <template #empty />
          </ArticleSection>
        </section>
      </div>

      <aside class="home-filter-rail" aria-label="搜索与筛选">
        <section class="blog-card home-search-card" aria-labelledby="content-search-title">
          <h2 id="content-search-title" class="font-display text-xl font-semibold">站内搜索</h2>
          <HeroSearch v-model:mode="mode" v-model:keyword="keyword" v-model:question="question" v-model:max-results="maxResults" v-bind="searchProps" surface sidebar class="home-content-search" input-id="content-search-input" @update:mode="switchMode" @submit="submit" />
        </section>
        <div v-show="mode === 'normal'" class="home-taxonomy-cards">
          <div v-if="taxonomyState.kind === 'loading' || taxonomyState.kind === 'idle'" data-testid="taxonomy-loading" class="blog-card p-5 text-sm text-text-secondary" role="status">正在加载分类与标签…</div>
          <section v-else-if="taxonomyState.kind === 'error'" data-testid="taxonomy-error" class="blog-card p-5" role="alert">
            <h2 class="font-semibold">分类与标签加载失败</h2><p class="mt-2 text-sm text-text-secondary">{{ taxonomyState.message }}</p>
            <button data-testid="taxonomy-retry" type="button" class="home-text-action mt-3" @click="retryTaxonomy">重试字典</button>
          </section>
          <template v-else>
            <ArticleFilterCard kind="category" :items="categories" :selected-id="filters?.articleTypeId" @change="filterCategory" />
            <ArticleFilterCard kind="tag" :items="tags" :selected-id="filters?.tagId" @change="filterTag" />
          </template>
        </div>
      </aside>
    </div>
    <AppModal :open="confirmAbandon" title="放弃未确认的搜索？" description="旧搜索可能已执行并产生费用，放弃不会取消远端任务；再次提交可能重复执行并再次产生费用。" @close="confirmAbandon = false" @closed="restoreAbandonFocus">
      <template #footer>
        <button type="button" class="min-h-12 rounded-lg bg-surface-muted px-5 text-text-primary hover:bg-surface-hover" @click="confirmAbandon = false">保留原操作</button>
        <button type="button" class="min-h-12 rounded-lg bg-error px-5 font-semibold text-canvas hover:opacity-90" @click="ai.abandonExpiredCreate(); confirmAbandon = false">确认放弃并开始新操作</button>
      </template>
    </AppModal>
  </BlogLayout>
</template>

<style scoped>
.home-page[data-scene="content"] { --layout-container: 1500px; --layout-sidebar: 220px; --layout-filter-sidebar: 280px; --layout-column-gap: 24px; }
.home-content-grid { display: grid; grid-template-columns: var(--layout-sidebar) minmax(0, 1fr) var(--layout-filter-sidebar); align-items: start; gap: var(--layout-column-gap); padding-block-start: var(--space-6); padding-inline-end: max(0px, calc(var(--space-20) - max(var(--layout-gutter), calc((100vw - var(--layout-container)) / 2)))); scroll-margin-top: calc(var(--layout-nav-height) + var(--space-6)); }
.home-author-rail, .home-filter-rail, .home-taxonomy-cards { display: grid; min-width: 0; align-content: start; gap: var(--space-6); }
.home-search-card { padding: var(--space-5); }
.home-content-search { margin-top: var(--space-4); }
.home-author-rail :deep(.blog-sidebar) { width: 100%; }
.home-author-rail :deep(nav[aria-label="个人链接"]) { padding: var(--space-3); gap: var(--space-1); }
.home-author-rail :deep(nav[aria-label="个人链接"] a) { padding-inline: var(--space-2); }
.home-author-rail :deep(#recommended-title) { font-size: var(--font-size-h3); }
.home-author-rail :deep(ol a) { gap: var(--space-2); }
.home-author-rail :deep(ol a > span:nth-child(2)) { overflow-wrap: anywhere; }
.home-author-rail :deep(ol a > span:nth-child(3)), .home-author-rail :deep(ol a > svg) { display: none; }
.home-author-rail :deep(.blog-card:hover), .home-author-rail :deep(.blog-card:focus-within),
.home-filter-rail :deep(.blog-card:hover), .home-filter-rail :deep(.blog-card:focus-within) { transform: none; border-color: var(--color-border-subtle); box-shadow: var(--shadow-card); }
.home-result-column { overflow-wrap: anywhere; }
.home-text-action { display: flex; align-items: center; justify-content: center; gap: var(--space-2); width: fit-content; min-height: var(--space-12); color: var(--color-accent); font-size: var(--font-size-small); font-weight: 600; }
.home-text-action:hover { color: var(--color-accent-hover); }
.home-load-more { min-height: var(--space-12); padding-inline: var(--space-8); border: var(--border-width) solid var(--color-border); border-radius: var(--radius-control); background: var(--color-surface); color: var(--color-text-primary); }
.home-load-more:hover { background: var(--color-surface-hover); }
</style>
