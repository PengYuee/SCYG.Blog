<script setup lang="ts">
import { ExclamationTriangleIcon, InformationCircleIcon } from "@heroicons/vue/24/outline"
import { computed, onMounted, ref } from "vue"
import ArticleSection from "@/components/article/ArticleSection.vue"
import ArticleSearchCard from "@/components/public/ArticleSearchCard.vue"
import type { ObserverFactory } from "@/components/public/BlogHeader.vue"
import ProfileCard from "@/components/public/ProfileCard.vue"
import RecommendedArticles from "@/components/public/RecommendedArticles.vue"
import TagCloud from "@/components/public/TagCloud.vue"
import BlogLayout from "@/layouts/BlogLayout.vue"
import { parseArticleList } from "@/request/api/article"
import { parseArticleTypes } from "@/request/api/article-type"
import { parseTags } from "@/request/api/tag"
import { http } from "@/request/http"
import { createArticleFeed, type ArticleFeed } from "@/stores/article-feed"
import { createTaxonomy, type Taxonomy } from "@/stores/taxonomy"
import type { ArticleDetail } from "@/types/article"
import type { ArticleType, Tag } from "@/types/taxonomy"
import { buildSearchUrl, groupHomepageArticles, selectRecommendations } from "@/utils/search"

/** 首页组合层输入，依赖由路由集成边界注入。 */
type Props = {
  /** 可替换的 T4 文章流状态机。 */ readonly articleFeed?: ArticleFeed
  /** 可替换的 T4 分类与标签状态机。 */ readonly taxonomy?: Taxonomy
  /** 测试可替换的 Hero 边界观察器。 */ readonly observerFactory?: ObserverFactory
}

const props = defineProps<Props>()
/** 未注入时使用现有只读 API 边界创建生产文章流。 */
const articleFeed = props.articleFeed ?? createArticleFeed({
  /** 获取并解析一页真实文章。 */
  async list(request) {
    const response = await http.get("/Article/GetArticleList", { params: request })
    return parseArticleList(response.data)
  },
}, 20)
/** 未注入时使用现有只读 API 边界创建生产字典。 */
const taxonomy = props.taxonomy ?? createTaxonomy({
  /** 获取并解析有序分类。 */
  async listArticleTypes() {
    const response = await http.get("/ArticleType/GetArticleTypeDic")
    return parseArticleTypes(response.data, window.location.origin)
  },
  /** 获取并解析标签。 */
  async listTags() {
    const response = await http.get("/Tag/GetTagDic")
    return parseTags(response.data)
  },
})
/** 驱动非 UI 状态机快照进入 Vue 响应式渲染周期。 */
const renderRevision = ref(0)
/** 仅在调用方提供观察器时向布局传递该可选属性。 */
const layoutProps = computed<{ readonly observerFactory?: ObserverFactory }>(() => props.observerFactory === undefined ? {} : { observerFactory: props.observerFactory })
/** 当前文章流快照。 */
const feedState = computed(() => { void renderRevision.value; return articleFeed.state })
/** 当前字典快照。 */
const taxonomyState = computed(() => { void renderRevision.value; return taxonomy.state })
/** 所有状态都保留的已加载文章。 */
const loadedArticles = computed<readonly ArticleDetail[]>(() => feedState.value.items)
/** 可展示分类，仅 ready 状态拥有非空或部分字典。 */
const categories = computed<readonly ArticleType[]>(() => taxonomyState.value.kind === "ready" ? taxonomyState.value.articleTypes : [])
/** 可展示标签，仅 ready 状态拥有非空或部分字典。 */
const tags = computed<readonly Tag[]>(() => taxonomyState.value.kind === "ready" ? taxonomyState.value.tags : [])
/** 最新区域保持后端加载顺序并限制首批六篇。 */
const latestArticles = computed(() => loadedArticles.value.slice(0, 6))
/** 推荐严格复用 T4 的访问量降序与 ID 稳定次序。 */
const recommendations = computed(() => selectRecommendations(loadedArticles.value))
/** 分类区域严格复用 T4 分组与每组六篇上限。 */
const categoryGroups = computed(() => groupHomepageArticles(loadedArticles.value, categories.value))

/** 启动文章流请求，并在开始和完成时同步 UI 快照。 */
const loadFeed = async (): Promise<void> => {
  const request = articleFeed.loadNext()
  renderRevision.value += 1
  await request
  renderRevision.value += 1
}

/** 启动字典请求，并在开始和完成时同步 UI 快照。 */
const loadTaxonomy = async (): Promise<void> => {
  const request = taxonomy.load()
  renderRevision.value += 1
  await request
  renderRevision.value += 1
}

/** 仅重试文章流最后失败的分页意图。 */
const retryFeed = async (): Promise<void> => {
  const request = articleFeed.retry()
  renderRevision.value += 1
  await request
  renderRevision.value += 1
}

/** 仅重试分类与标签字典，不触发文章流。 */
const retryTaxonomy = async (): Promise<void> => {
  const request = taxonomy.retry()
  renderRevision.value += 1
  await request
  renderRevision.value += 1
}

/** 为分类更多链接生成固定参数顺序的筛选 URL。 */
const categoryMoreTo = (categoryId: number): string => buildSearchUrl("/articles", { q: "", articleTypeId: categoryId })

/** 首次挂载并行启动两个相互独立的数据域。 */
onMounted(async () => {
  await Promise.all([loadTaxonomy(), loadFeed()])
})
</script>

<template>
  <BlogLayout v-bind="layoutProps">
    <section data-testid="home-announcement" class="blog-card mb-8 flex items-center gap-4 px-6 py-5" aria-label="首页数据说明">
      <span class="inline-flex size-11 shrink-0 items-center justify-center rounded-full bg-accent-soft text-accent"><InformationCircleIcon class="size-6" aria-hidden="true" /></span>
      <div>
        <p class="font-semibold text-text-primary">文章发现</p>
        <p class="text-sm text-text-secondary">当前展示已加载 {{ loadedArticles.length }} 篇文章，推荐与分组均来自真实文章数据。</p>
      </div>
    </section>

    <div class="home-content-grid">
      <aside class="blog-sidebar space-y-6" aria-label="作者与文章发现工具">
        <ProfileCard />
        <ArticleSearchCard />
        <div data-testid="recommended-articles"><RecommendedArticles :articles="recommendations" /></div>

        <div v-if="taxonomyState.kind === 'loading' || taxonomyState.kind === 'idle'" data-testid="taxonomy-loading" class="blog-card p-6 text-sm text-text-secondary" role="status">正在加载分类与标签…</div>
        <section v-else-if="taxonomyState.kind === 'error'" data-testid="taxonomy-error" class="blog-card border-error bg-error-soft p-6" aria-labelledby="taxonomy-error-title">
          <ExclamationTriangleIcon class="size-6 text-error" aria-hidden="true" />
          <h2 id="taxonomy-error-title" class="mt-3 font-semibold text-text-primary">分类与标签加载失败</h2>
          <p class="mt-2 text-sm text-text-secondary">{{ taxonomyState.message }}</p>
          <button data-testid="taxonomy-retry" type="button" class="mt-4 min-h-11 rounded-lg bg-accent px-4 text-sm font-semibold text-[color:var(--color-hero-text)] hover:bg-accent-hover" @click="retryTaxonomy">重试字典</button>
        </section>
        <TagCloud v-else :tags="tags" />
      </aside>

      <div class="min-w-0 space-y-12" aria-live="polite">
        <div v-if="feedState.kind === 'loading' || feedState.kind === 'idle'" data-testid="feed-loading" class="blog-card flex min-h-48 items-center justify-center p-8 text-sm text-text-secondary" role="status">正在加载文章…</div>
        <section v-else-if="feedState.kind === 'error'" data-testid="feed-error" class="blog-card border-error bg-error-soft p-6" aria-labelledby="feed-error-title">
          <ExclamationTriangleIcon class="size-6 text-error" aria-hidden="true" />
          <h2 id="feed-error-title" class="mt-3 font-semibold text-text-primary">文章加载失败</h2>
          <p class="mt-2 text-sm text-text-secondary">{{ feedState.message }}</p>
          <button data-testid="feed-retry" type="button" class="mt-4 min-h-11 rounded-lg bg-accent px-4 text-sm font-semibold text-[color:var(--color-hero-text)] hover:bg-accent-hover" @click="retryFeed">重试文章</button>
        </section>
        <section v-else-if="feedState.kind === 'empty'" data-testid="feed-empty" class="blog-card flex min-h-48 flex-col items-center justify-center p-8 text-center">
          <h2 class="font-[family-name:var(--font-family-display)] text-[length:var(--font-size-h2)] font-bold text-text-primary">暂无已发布文章</h2>
          <p class="mt-2 text-sm text-text-secondary">分类、搜索与作者信息仍可继续浏览。</p>
        </section>

        <template v-if="loadedArticles.length > 0">
          <ArticleSection title="最新文章" :articles="latestArticles" :categories="categories" more-to="/articles" />
          <div v-for="group in categoryGroups" :key="group.categoryId" data-testid="category-section">
            <ArticleSection :title="group.categoryName" :articles="group.articles" :categories="categories" :more-to="categoryMoreTo(group.categoryId)" />
          </div>
        </template>
      </div>
    </div>
  </BlogLayout>
</template>

<style scoped>
.home-content-grid {
  display: grid;
  grid-template-columns: var(--layout-sidebar) minmax(0, 1fr);
  gap: var(--layout-column-gap);
}
</style>
