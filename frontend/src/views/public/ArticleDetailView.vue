<script setup lang="ts">
import { CalendarDaysIcon, EyeIcon, TagIcon } from "@heroicons/vue/24/outline"
import { computed, ref, watch } from "vue"
import { useRoute } from "vue-router"
import MarkdownRenderer from "@/components/article/MarkdownRenderer.vue"
import PublicLayout from "@/layouts/PublicLayout.vue"
import { useApiServices } from "@/request/api-services"
import { HttpRequestError } from "@/request/http-error"
import { createTaxonomy, type Taxonomy, type TaxonomyState } from "@/stores/taxonomy"
import type { ArticleDetail } from "@/types/article"
import { useScrollRestoration } from "@/services/scroll-restoration"

/** 文章详情加载依赖。 */
export interface ArticleDetailLoader {
  /** 按标识读取一篇文章。 */
  detail(id: number): Promise<ArticleDetail>
}

/** 文章详情加载状态。 */
type DetailState =
  | { readonly kind: "idle" }
  | { readonly kind: "loading" }
  | { readonly kind: "ready"; readonly article: ArticleDetail }
  | { readonly kind: "not-found" }
  | { readonly kind: "error" }

/** 文章详情页可注入依赖。 */
const props = defineProps<{
  /** 测试或预览显式指定的文章标识。 */ readonly articleId?: number
  /** 可替换的文章详情加载器。 */ readonly articleLoader?: ArticleDetailLoader
  /** 可替换的分类状态机。 */ readonly taxonomy?: Taxonomy
}>()
/** 应用级刷新滚动恢复服务。 */
const scrollRestoration = useScrollRestoration()

/** 文章元数据使用的稳定中文日期格式器。 */
const articleDateFormatter = new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "long", day: "numeric" })
/** 格式化已由 T3 解析的文章日期。 */
const formatArticleDate = (value: string): string => articleDateFormatter.format(new Date(value))

/** 根据显式依赖完整性决定是否读取应用服务容器。 */
const [articleLoader, taxonomy] = (() => {
  if (props.articleLoader !== undefined && props.taxonomy !== undefined) return [props.articleLoader, props.taxonomy] as const
  const services = useApiServices()
  return [
    props.articleLoader ?? services.article,
    props.taxonomy ?? createTaxonomy({ listArticleTypes: () => services.publicTaxonomy.listArticleTypes(), listTags: () => services.publicTaxonomy.listTags() }),
  ] as const
})()
/** 当前路由提供 T9 详情参数，但不修改生产路由。 */
const route = useRoute()
/** 当前文章请求状态。 */
const state = ref<DetailState>({ kind: "idle" })
/** taxonomy 独立快照；它缺席或迟到都不阻塞正文。 */
const taxonomyState = ref<TaxonomyState>(taxonomy.state)
/** 分类图片加载失败后切换到项目自有回退图。 */
const categoryImageFailed = ref(false)
/** 请求世代阻止路由快速切换时旧响应覆盖新文章。 */
let requestGeneration = 0
/** 首次文章请求终态只通知一次，不让重试或 taxonomy 重复触发。 */
let initialReadyMarked = false


/** 解析显式测试标识或 T9 路由参数。 */
const requestedArticleId = computed<number | null>(() => {
  if (props.articleId !== undefined) return props.articleId > 0 ? props.articleId : null
  const rawId = route.params["id"]
  if (typeof rawId !== "string" || !/^\d+$/.test(rawId)) return null
  const id = Number(rawId)
  return Number.isSafeInteger(id) && id > 0 ? id : null
})

/** ready 状态中的文章。 */
const article = computed(() => state.value.kind === "ready" ? state.value.article : null)
/** Hero 优先展示已加载文章标题，其他状态使用稳定页面标题。 */
const layoutTitle = computed(() => article.value?.title ?? "文章详情")
/** taxonomy ready 时解析文章分类。 */
const category = computed(() => {
  const currentArticle = article.value
  if (currentArticle === null || taxonomyState.value.kind !== "ready") return undefined
  return taxonomyState.value.articleTypes.find((item) => item.id === currentArticle.articleTypeId)
})
/** taxonomy ready 时按文章顺序解析标签。 */
const tags = computed(() => {
  const currentArticle = article.value
  if (currentArticle === null || taxonomyState.value.kind !== "ready") return []
  const tagIds = new Set(currentArticle.tagIds)
  return taxonomyState.value.tags.filter((tag) => tagIds.has(tag.id))
})
/** 分类图片或项目自有稳定回退。 */
const categoryImage = computed(() => categoryImageFailed.value ? "/images/hero-starry.jpg" : category.value?.imageUrl ?? "/images/hero-starry.jpg")

/** 独立加载 taxonomy 并同步最终状态，不参与正文成败。 */
const loadTaxonomy = async (): Promise<void> => {
  await taxonomy.load()
  taxonomyState.value = taxonomy.state
}

/** 加载当前文章并将失败限制在详情页重试范围。 */
const loadArticle = async (): Promise<void> => {
  const articleId = requestedArticleId.value
  const generation = ++requestGeneration
  categoryImageFailed.value = false
  if (articleId === null) {
    state.value = { kind: "not-found" }
    if (!initialReadyMarked) {
      initialReadyMarked = true
      scrollRestoration.markReady()
    }
    return
  }
  state.value = { kind: "loading" }
  try {
    const loadedArticle = await articleLoader.detail(articleId)
    if (generation === requestGeneration) state.value = { kind: "ready", article: loadedArticle }
  } catch (error) {
    if (generation !== requestGeneration) return
    state.value = error instanceof HttpRequestError && error.status === 404 ? { kind: "not-found" } : { kind: "error" }
  }
  if (generation === requestGeneration && !initialReadyMarked) {
    initialReadyMarked = true
    scrollRestoration.markReady()
  }
}

void loadTaxonomy()
watch(requestedArticleId, () => { void loadArticle() }, { immediate: true })
</script>

<template>
  <PublicLayout :title="layoutTitle" content-id="article-content" skip-label="跳到文章正文" :data-state="state.kind">
    <section v-if="state.kind === 'idle'" class="blog-card p-8 text-center" role="status">准备加载文章</section>
    <section v-else-if="state.kind === 'loading'" class="blog-card flex min-h-64 items-center justify-center p-8 text-center" role="status" aria-busy="true">正在加载文章…</section>
    <section v-else-if="state.kind === 'not-found'" class="blog-card flex min-h-64 flex-col items-center justify-center gap-4 p-8 text-center">
      <h2 class="font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h2)] font-bold leading-[var(--line-height-h2)]">文章未找到</h2>
      <p class="text-text-secondary">这篇文章可能已被移除，或链接不正确。</p>
    </section>
    <section v-else-if="state.kind === 'error'" class="blog-card flex min-h-64 flex-col items-center justify-center gap-4 p-8 text-center" role="alert">
      <h1 class="font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h2)] font-bold leading-[var(--line-height-h2)]">文章暂时无法加载</h1>
      <p class="text-text-secondary">请检查网络后重试，当前页面不会丢失导航上下文。</p>
      <button type="button" class="min-h-11 rounded-[var(--radius-control)] bg-accent px-6 font-semibold text-canvas hover:bg-accent-hover active:scale-[0.98]" @click="loadArticle">重新加载</button>
    </section>
    <article v-else-if="state.kind === 'ready'" class="min-w-0">
      <header class="blog-card overflow-hidden">
        <div class="relative aspect-[5/2] overflow-hidden bg-surface-muted">
          <img :src="categoryImage" width="1220" height="488" class="size-full object-cover" :alt="category ? `${category.name}分类配图` : '文章默认星空配图'" data-testid="category-image" @error="categoryImageFailed = true" />
          <div class="absolute inset-0 bg-overlay" aria-hidden="true" />
          <div class="absolute inset-x-0 bottom-0 p-8 text-hero-text">
            <p class="text-sm font-semibold tracking-[0.18em] text-hero-text-muted">{{ category?.name ?? "未分类" }}</p>
            <h2 class="mt-2 max-w-4xl text-balance font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h2)] font-bold leading-[var(--line-height-h2)]">{{ state.article.title }}</h2>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-x-6 gap-y-3 border-b border-border-subtle bg-surface px-8 py-5 text-sm text-text-secondary">
          <span class="inline-flex items-center gap-2"><CalendarDaysIcon class="size-5 text-accent" aria-hidden="true" /><time :datetime="state.article.createdAt">{{ formatArticleDate(state.article.createdAt) }}</time></span>
          <span class="inline-flex items-center gap-2"><EyeIcon class="size-5 text-accent" aria-hidden="true" />{{ state.article.visited }} 次阅读</span>
          <span v-if="state.article.updatedAt">更新于 {{ formatArticleDate(state.article.updatedAt) }}</span>
        </div>
        <div class="flex min-h-16 flex-wrap items-center gap-2 px-8 py-3" aria-label="文章标签">
          <TagIcon class="size-5 text-accent" aria-hidden="true" />
          <span v-for="tag in tags" :key="tag.id" class="rounded-full bg-accent-soft px-3 py-1 text-sm font-medium text-accent">{{ tag.name }}</span>
          <span v-if="tags.length === 0" class="text-sm text-text-tertiary">暂无标签</span>
        </div>
      </header>
      <section class="mt-8 rounded-[var(--radius-card)] border border-border-subtle bg-surface p-8 shadow-[var(--shadow-card)]" aria-label="文章正文">
        <MarkdownRenderer :markdown="state.article.markdown" />
      </section>
    </article>
  </PublicLayout>
</template>
