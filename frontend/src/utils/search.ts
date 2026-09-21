import type { ArticleDetail } from "@/types/article"

/** 可序列化搜索筛选状态。 */
export type SearchFilters = {
  /** 后端文章搜索词。 */ readonly q: string
  /** 可选分类筛选。 */ readonly articleTypeId?: number
  /** 可选标签筛选。 */ readonly tagId?: number
}

/** 首页分类顺序输入。 */
export type CategoryOrder = {
  /** 分类标识。 */ readonly id: number
  /** 分类名称。 */ readonly name: string
}

/** 首页分类文章组。 */
export type HomepageGroup = {
  /** 分类标识。 */ readonly categoryId: number
  /** 分类名称。 */ readonly categoryName: string
  /** 最多六篇、保持加载顺序的文章。 */ readonly articles: readonly ArticleDetail[]
}

/** 已加载文章的本地搜索结果，明确不代表服务端全量搜索。 */
export type LoadedSearchResult = {
  /** 稳定结果标记，供 UI 和验收证据识别搜索范围。 */
  readonly kind: "loaded_results"
  /** 规范化后的搜索词。 */
  readonly query: string
  /** 按当前文章流顺序筛选出的结果。 */
  readonly items: readonly ArticleDetail[]
}

/** 仅在当前已加载文章的标题、摘要、分类和标签文本中搜索。 */
export function searchLoadedArticles(
  articles: readonly ArticleDetail[],
  rawQuery: string,
  names: { readonly categories: ReadonlyMap<number, string>; readonly tags: ReadonlyMap<number, string> },
): LoadedSearchResult {
  const query = rawQuery.trim().toLocaleLowerCase()
  const items = query.length === 0
    ? [...articles]
    : articles.filter((article) => {
      const category = names.categories.get(article.articleTypeId) ?? ""
      const tags = article.tagIds.map((tagId) => names.tags.get(tagId) ?? "")
      return [article.title, article.digest, category, ...tags].some((value) => value.toLocaleLowerCase().includes(query))
    })
  return { kind: "loaded_results", query, items }
}

/** 按访问量降序、文章标识升序选择最多三篇推荐。 */
export function selectRecommendations(articles: readonly ArticleDetail[]): readonly ArticleDetail[] {
  return [...articles].sort((left, right) => right.visited - left.visited || left.id - right.id).slice(0, 3)
}

/** 按传入分类顺序构建最多六篇的稳定首页文章组。 */
export function groupHomepageArticles(articles: readonly ArticleDetail[], categories: readonly CategoryOrder[]): readonly HomepageGroup[] {
  return categories.map((category) => ({
    categoryId: category.id,
    categoryName: category.name,
    articles: articles.filter((article) => article.articleTypeId === category.id).slice(0, 6),
  }))
}

/** 以固定参数顺序构建可分享、可恢复的规范搜索 URL。 */
export function buildSearchUrl(path: string, filters: SearchFilters): string {
  const parameters = new URLSearchParams()
  const query = filters.q.trim().toLocaleLowerCase()
  if (query.length > 0) parameters.set("q", query)
  if (filters.articleTypeId !== undefined) parameters.set("articleTypeId", String(filters.articleTypeId))
  if (filters.tagId !== undefined) parameters.set("tagId", String(filters.tagId))
  const serialized = parameters.toString()
  return serialized.length === 0 ? path : `${path}?${serialized}`
}
