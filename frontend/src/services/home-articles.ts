import { computed, onMounted, shallowRef, watch, type ShallowRef } from "vue"
import { useRoute } from "vue-router"
import { useApiServices } from "@/request/api-services"
import { parseHomepageQuery } from "@/router/query"
import { createArticleFeed, type ArticleFeed, type ArticleFeedApi, type ArticleFeedState } from "@/stores/article-feed"
import { createTaxonomy, type Taxonomy } from "@/stores/taxonomy"
import { useScrollRestoration } from "@/services/scroll-restoration"

/** 应用 API 实例拥有首页文章缓存，详情返回时保留已加载页，不跨应用或测试共享。 */
const homeFeeds = new WeakMap<ArticleFeedApi, { readonly feed: ArticleFeed; readonly state: ShallowRef<ArticleFeedState> }>()

export function useHomeArticles(props: { readonly articleFeed?: ArticleFeed | undefined; readonly taxonomy?: Taxonomy | undefined }) {
  const route = useRoute()
  const restoration = useScrollRestoration()
  const services = useApiServices()
  let cached = props.articleFeed === undefined ? homeFeeds.get(services.article) : undefined
  if (cached === undefined) {
    const feed = props.articleFeed ?? createArticleFeed(services.article, 9)
    cached = { feed, state: shallowRef(feed.state) }
    if (props.articleFeed === undefined) homeFeeds.set(services.article, cached)
  }
  const { feed: articleFeed, state: feedState } = cached
  const taxonomy = props.taxonomy ?? createTaxonomy({
    listArticleTypes: () => services.publicTaxonomy.listArticleTypes(),
    listTags: () => services.publicTaxonomy.listTags(),
  })
  const taxonomyState = shallowRef(taxonomy.state)
  const parsed = computed(() => parseHomepageQuery(route.query))
  const filters = computed(() => parsed.value.success ? {
    q: parsed.value.data.q,
    ...(parsed.value.data.articleTypeId === undefined ? {} : { articleTypeId: parsed.value.data.articleTypeId }),
    ...(parsed.value.data.tagId === undefined ? {} : { tagId: parsed.value.data.tagId }),
  } : undefined)
  const categories = computed(() => taxonomyState.value.kind === "ready" ? taxonomyState.value.articleTypes : [])
  const tags = computed(() => taxonomyState.value.kind === "ready" ? taxonomyState.value.tags : [])
  let initialReady = false
  function markReady(): void {
    if (!initialReady) { initialReady = true; restoration.markReady() }
  }
  async function loadFeed(): Promise<void> {
    const request = articleFeed.loadNext()
    feedState.value = articleFeed.state
    await request
    feedState.value = articleFeed.state
    markReady()
  }
  async function retryFeed(): Promise<void> {
    const request = articleFeed.retry()
    feedState.value = articleFeed.state
    await request
    feedState.value = articleFeed.state
  }
  async function loadTaxonomy(): Promise<void> {
    const request = taxonomy.state.kind === "error" ? taxonomy.retry() : taxonomy.load()
    taxonomyState.value = taxonomy.state
    await request
    taxonomyState.value = taxonomy.state
  }
  watch(() => JSON.stringify(filters.value), async () => {
    if (filters.value === undefined) { markReady(); return }
    if (JSON.stringify(articleFeed.state.filters) !== JSON.stringify(filters.value)) articleFeed.resetFilters(filters.value)
    if (articleFeed.state.kind === "idle" || articleFeed.state.kind === "error") await loadFeed()
    else { feedState.value = articleFeed.state; markReady() }
  }, { immediate: true })
  onMounted(loadTaxonomy)
  return { feedState, taxonomyState, parsed, filters, categories, tags, loadFeed, retryFeed, retryTaxonomy: loadTaxonomy }
}
