<script setup lang="ts">
import { ArrowPathIcon, FunnelIcon, MagnifyingGlassIcon } from "@heroicons/vue/24/outline"
import { computed, ref, shallowRef, watch } from "vue"
import { useRoute, useRouter } from "vue-router"
import ArticleSection from "@/components/article/ArticleSection.vue"
import TagCloud from "@/components/public/TagCloud.vue"
import BlogLayout from "@/layouts/BlogLayout.vue"
import { useApiServices } from "@/request/api-services"
import { parseArticleListQuery, serializeArticleListQuery, type ArticleListQuery } from "@/router/query"
import { createArticleFeed } from "@/stores/article-feed"
import { createTaxonomy } from "@/stores/taxonomy"
import { searchLoadedArticles } from "@/utils/search"

const route = useRoute()
const router = useRouter()
/** 读取应用挂载时创建的同一 API 服务容器。 */
const services = useApiServices()
/** T4 文章流保持显式九篇分页，不注册滚动监听。 */
const feed = createArticleFeed(services.article, 9)
/** T4 分类状态机为筛选和卡片提供同一字典。 */
const taxonomy = createTaxonomy({ listArticleTypes: () => services.articleType.list(), listTags: () => services.tag.list() })/** Vue 快照在状态机操作结束后显式同步。 */
const feedState = shallowRef(feed.state)
const taxonomyState = shallowRef(taxonomy.state)
/** 当前路由查询的类型化解析结果。 */
const queryResult = computed(() => parseArticleListQuery(route.query))
/** 搜索草稿仅在提交时写回 URL。 */
const draftQuery = ref("")

/** 可用于筛选和卡片展示的分类。 */
const categories = computed(() => taxonomyState.value.kind === "ready" ? taxonomyState.value.articleTypes : [])
/** 可用于筛选的标签。 */
const tags = computed(() => taxonomyState.value.kind === "ready" ? taxonomyState.value.tags : [])
/** 仅在合法标签存在时向 TagCloud 传递可选属性。 */
const activeTagProps = computed(() => queryResult.value.kind === "valid" && queryResult.value.value.tagId !== undefined
  ? { activeTagId: queryResult.value.value.tagId }
  : {})
/** 本地搜索仅覆盖已经显式加载的文章。 */
const visibleArticles = computed(() => {
  if (queryResult.value.kind === "invalid") return []
  const names = {
    categories: new Map(categories.value.map(({ id, name }) => [id, name])),
    tags: new Map(tags.value.map(({ id, name }) => [id, name])),
  }
  return searchLoadedArticles(feedState.value.items, queryResult.value.value.q, names).items
})

/** 同步状态机快照到 Vue 响应层。 */
function syncSnapshots(): void {
  feedState.value = feed.state
  taxonomyState.value = taxonomy.state
}

/** 根据规范路由查询重置并加载第一页。 */
async function loadFromRoute(): Promise<void> {
  const result = queryResult.value
  if (result.kind === "invalid") return
  draftQuery.value = result.value.q
  feed.resetFilters({
    ...(result.value.categoryId === undefined ? {} : { articleTypeId: result.value.categoryId }),
    ...(result.value.tagId === undefined ? {} : { tagId: result.value.tagId }),
  })
  const taxonomyLoad = taxonomy.state.kind === "idle" ? taxonomy.load() : Promise.resolve()
  const feedLoad = feed.loadNext()
  syncSnapshots()
  await Promise.all([feedLoad, taxonomyLoad])
  syncSnapshots()
}

/** 显式加载下一页，页面不使用无限滚动。 */
async function loadMore(): Promise<void> {
  const pending = feed.loadNext()
  syncSnapshots()
  await pending
  syncSnapshots()
}

/** 重试最后失败的分页请求。 */
async function retryFeed(): Promise<void> {
  const pending = feed.retry()
  syncSnapshots()
  await pending
  syncSnapshots()
}

/** 将表单草稿合并到当前合法查询并导航。 */
async function submitSearch(): Promise<void> {
  const result = queryResult.value
  if (result.kind === "invalid") return
  await router.push({ path: "/articles", query: serializeArticleListQuery({ ...result.value, q: draftQuery.value }) })
}

/** 更新分类查询；空值表示清除筛选。 */
async function updateCategory(event: Event): Promise<void> {
  if (!(event.currentTarget instanceof HTMLSelectElement) || queryResult.value.kind === "invalid") return
  const categoryId = event.currentTarget.value === "" ? undefined : Number(event.currentTarget.value)
  await navigateWithFilters({ ...queryResult.value.value, categoryId })
}

/** 更新标签查询；空值表示清除筛选。 */
async function updateTag(event: Event): Promise<void> {
  if (!(event.currentTarget instanceof HTMLSelectElement) || queryResult.value.kind === "invalid") return
  const tagId = event.currentTarget.value === "" ? undefined : Number(event.currentTarget.value)
  await navigateWithFilters({ ...queryResult.value.value, tagId })
}

/** 将已解析筛选序列化到唯一 URL。 */
async function navigateWithFilters(filters: ArticleListQuery): Promise<void> {
  await router.push({ path: "/articles", query: serializeArticleListQuery(filters) })
}

watch(() => route.fullPath, loadFromRoute, { immediate: true })
</script>

<template>
  <BlogLayout>
    <div class="blog-content-grid items-start">
      <div class="min-w-0">
        <header class="mb-8">
          <p class="text-xs font-semibold uppercase tracking-[0.18em] text-accent">DISCOVER</p>
          <h1 class="mt-2 text-balance font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h1)] font-bold leading-[var(--line-height-h1)]">文章归档</h1>
          <p class="mt-3 text-pretty text-text-secondary">按关键词、分类和标签浏览已发布内容。搜索范围会随显式加载的页面扩展。</p>
        </header>

        <div v-if="queryResult.kind === 'invalid'" class="blog-card border-error bg-error-soft p-6 text-error" role="alert" data-testid="query-error">
          <p class="font-semibold">筛选参数无效</p><p class="mt-1 text-sm">{{ queryResult.message }}</p>
        </div>
        <template v-else>
          <form class="blog-card mb-8 grid grid-cols-[minmax(0,1fr)_auto_auto_auto] items-end gap-4 p-5" role="search" aria-label="筛选文章" @submit.prevent="submitSearch">
            <label class="grid gap-2 text-sm font-semibold"><span>关键词</span><span class="flex min-h-11 items-center rounded-[var(--radius-control)] border border-border bg-surface px-3"><MagnifyingGlassIcon class="mr-2 size-5 text-text-tertiary" aria-hidden="true" /><input v-model="draftQuery" class="min-w-0 flex-1 bg-transparent outline-none" type="search" placeholder="标题、摘要、分类或标签" /></span></label>
            <label class="grid gap-2 text-sm font-semibold"><span>分类</span><select class="min-h-11 rounded-[var(--radius-control)] border border-border bg-surface px-3" :value="queryResult.value.categoryId ?? ''" @change="updateCategory"><option value="">全部分类</option><option v-for="category in categories" :key="category.id" :value="category.id">{{ category.name }}</option></select></label>
            <label class="grid gap-2 text-sm font-semibold"><span>标签</span><select class="min-h-11 rounded-[var(--radius-control)] border border-border bg-surface px-3" :value="queryResult.value.tagId ?? ''" @change="updateTag"><option value="">全部标签</option><option v-for="tag in tags" :key="tag.id" :value="tag.id">{{ tag.name }}</option></select></label>
            <button type="submit" class="inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-control)] bg-accent px-5 font-semibold text-[color:var(--color-hero-text)] hover:bg-accent-hover active:scale-[0.98]"><FunnelIcon class="size-5" aria-hidden="true" />应用</button>
          </form>

          <div v-if="feedState.kind === 'loading' && feedState.items.length === 0" class="blog-card flex min-h-48 items-center justify-center p-8 text-text-secondary" role="status">正在加载文章…</div>
          <div v-else-if="feedState.kind === 'error' && feedState.items.length === 0" class="blog-card border-error bg-error-soft p-6 text-error" role="alert"><p class="font-semibold">文章加载失败</p><p class="mt-1 text-sm">{{ feedState.message }}</p><button type="button" class="mt-4 inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-control)] border border-error px-4 font-semibold" @click="retryFeed"><ArrowPathIcon class="size-5" aria-hidden="true" />重试</button></div>
          <ArticleSection v-else title="全部文章" :articles="visibleArticles" :categories="categories" />
          <div v-if="feedState.kind === 'error' && feedState.items.length > 0" data-testid="load-more-error" class="mt-6 rounded-[var(--radius-card)] border border-error bg-error-soft p-5 text-error" role="alert">
            <p class="font-semibold">加载更多失败</p>
            <p class="mt-1 text-sm">{{ feedState.message }}</p>
            <button type="button" data-testid="load-more-retry" class="mt-4 inline-flex min-h-11 items-center gap-2 rounded-[var(--radius-control)] border border-error px-4 font-semibold" @click="retryFeed"><ArrowPathIcon class="size-5" aria-hidden="true" />重试加载更多</button>
          </div>

          <div v-if="feedState.kind === 'ready' && !feedState.endReached" class="mt-8 text-center"><button type="button" data-testid="load-more" class="inline-flex min-h-11 items-center rounded-[var(--radius-control)] border border-accent bg-surface px-6 font-semibold text-accent hover:bg-surface-hover active:scale-[0.98]" @click="loadMore">加载更多</button></div>
          <p v-else-if="feedState.kind === 'loading' && feedState.items.length > 0" class="mt-8 text-center text-sm text-text-secondary" role="status">正在加载更多文章…</p>
        </template>
      </div>
      <aside class="grid gap-6" aria-label="文章辅助导航"><TagCloud :tags="tags" v-bind="activeTagProps" /></aside>
    </div>
  </BlogLayout>
</template>
