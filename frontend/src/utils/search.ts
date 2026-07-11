import type { ArticleDetail } from "@/types/article"

/** 可序列化搜索筛选状态。 */
export type SearchFilters = {
  /** 本地搜索词。 */ readonly q: string
  /** 可选分类筛选。 */ readonly articleTypeId?: number
  /** 可选标签筛选。 */ readonly tagId?: number
}

/** 明确声明范围仅限已加载文章的搜索结果。 */
export type LoadedSearchResult = {
  /** 真实性标记，禁止暗示远程完整结果。 */ readonly kind: "loaded_results"
  /** 规范化后的搜索词。 */ readonly query: string
  /** 保持已加载顺序的匹配文章。 */ readonly items: readonly ArticleDetail[]
}

/** 分类与标签名称索引。 */
export type SearchNames = {
  /** 分类标识到展示名。 */ readonly categories: ReadonlyMap<number, string>
  /** 标签标识到展示名。 */ readonly tags: ReadonlyMap<number, string>
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

/** 在标题、摘要、分类名和标签名中确定性搜索已加载文章。 */
export function searchLoadedArticles(articles: readonly ArticleDetail[], query: string, names: SearchNames): LoadedSearchResult {
  const normalizedQuery = query.trim().toLocaleLowerCase()
  if (normalizedQuery.length === 0) return { kind: "loaded_results", query: normalizedQuery, items: articles }
  const items = articles.filter((article) => {
    const categoryName = names.categories.get(article.articleTypeId) ?? ""
    const tagNames = article.tagIds.map((tagId) => names.tags.get(tagId) ?? "")
    return [article.title, article.digest, categoryName, ...tagNames]
      .some((value) => value.toLocaleLowerCase().includes(normalizedQuery))
  })
  return { kind: "loaded_results", query: normalizedQuery, items }
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
