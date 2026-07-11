import { describe, expect, it } from "vitest"
import { createArticleFeed, type ArticleFeedApi } from "@/stores/article-feed"
import { createTaxonomy, type TaxonomyApi } from "@/stores/taxonomy"
import { buildSearchUrl, groupHomepageArticles, searchLoadedArticles, selectRecommendations } from "@/utils/search"
import type { PageResult } from "@/types/api"
import type { ArticleDetail } from "@/types/article"

/** 创建稳定文章测试数据。 */
function article(id: number, category = 1, visited = 0): ArticleDetail {
  return { id, title: `Title ${id}`, slug: `article-${id}`, digest: `Digest ${id}`, markdown: `Body ${id}`, articleTypeId: category, tagIds: [id + 10], status: 2, support: 0, comment: 0, visited, version: 1, createdAt: `2026-07-${String(id).padStart(2, "0")}T00:00:00Z`, updatedAt: null }
}

/** 创建只读分页结果。 */
function page(items: readonly ArticleDetail[], pageIndex: number, totalItems: number, totalPages: number, pageSize = 2): PageResult<ArticleDetail> {
  return { items, pageIndex, pageSize, totalItems, totalPages }
}

/** 可编排文章分页假实现。 */
class FakeFeedApi implements ArticleFeedApi {
  /** 已收到的页码。 */ readonly requestedPages: number[] = []
  /** 顺序响应；Error 表示适配器拒绝。 */ constructor(private readonly responses: readonly (PageResult<ArticleDetail> | Error | Promise<PageResult<ArticleDetail>>)[]) {}
  /** 返回当前调用对应响应。 */ async list(request: Parameters<ArticleFeedApi["list"]>[0]): Promise<PageResult<ArticleDetail>> {
    this.requestedPages.push(request.pageModel.pageIndex)
    const response = this.responses[this.requestedPages.length - 1]
    if (response === undefined) throw new Error("missing fake response")
    if (response instanceof Error) throw response
    return response
  }
}

describe("article feed state machine", () => {
  it("merges two pages once in stable first-seen order and detects the end", async () => {
    // Given: page two repeats one article from page one.
    const api = new FakeFeedApi([page([article(1), article(2)], 0, 3, 2), page([article(2), article(3)], 1, 3, 2)])
    const feed = createArticleFeed(api, 2)
    // When: both pages load.
    await feed.loadNext(); await feed.loadNext(); await feed.loadNext()
    // Then: IDs are deduplicated and terminal state blocks extra calls.
    expect(feed.state).toMatchObject({ kind: "ready", pageIndex: 1, endReached: true })
    expect(feed.state.items.map(({ id }) => id)).toEqual([1, 2, 3])
    expect(api.requestedPages).toEqual([0, 1])
  })

  it("ignores a concurrent load while the request lock is held", async () => {
    // Given: the first adapter call is pending.
    let release: ((value: PageResult<ArticleDetail>) => void) | undefined
    const pending = new Promise<PageResult<ArticleDetail>>((resolve) => { release = resolve })
    const api = new FakeFeedApi([pending])
    const feed = createArticleFeed(api, 2)
    // When: two loads overlap.
    const first = feed.loadNext(); const ignored = feed.loadNext()
    release?.(page([article(1)], 0, 1, 1)); await Promise.all([first, ignored])
    // Then: only one adapter request was made.
    expect(api.requestedPages).toEqual([0])
  })

  it("resets filters atomically to page zero and restores route-keyed scroll", async () => {
    // Given: one loaded page and remembered scroll offsets.
    const api = new FakeFeedApi([page([article(1)], 0, 2, 2), page([article(2, 1)], 0, 1, 1)])
    const feed = createArticleFeed(api, 2)
    await feed.loadNext(); feed.rememberScroll("/?category=1", 420); feed.rememberScroll("/?category=2", 80)
    // When: filters change and loading restarts.
    feed.resetFilters({ articleTypeId: 2, tagId: 9 }); await feed.loadNext()
    // Then: no stale rows survive and route restoration is deterministic.
    expect(api.requestedPages).toEqual([0, 0])
    expect(feed.state.items.map(({ id }) => id)).toEqual([2])
    expect(feed.restoreScroll("/?category=1")).toBe(420)
    expect(feed.restoreScroll("/?missing")).toBe(0)
  })

  it("discards a stale response after filters reset", async () => {
    // Given: an old-filter request remains pending while a new-filter page is available.
    let release: ((value: PageResult<ArticleDetail>) => void) | undefined
    const pending = new Promise<PageResult<ArticleDetail>>((resolve) => { release = resolve })
    const api = new FakeFeedApi([pending, page([article(9, 2)], 0, 1, 1)])
    const feed = createArticleFeed(api, 2)
    const staleLoad = feed.loadNext()
    // When: filters reset and the new page completes before the stale response.
    feed.resetFilters({ articleTypeId: 2 }); await feed.loadNext(); release?.(page([article(1, 1)], 0, 1, 1)); await staleLoad
    // Then: the old response cannot overwrite the new-filter state.
    expect(feed.state.items.map(({ id }) => id)).toEqual([9])
    expect(feed.state.filters).toEqual({ articleTypeId: 2 })
  })

  it("retries the last failed page after malformed-page rejection without duplicates", async () => {
    // Given: page one succeeds, page two fails once, then succeeds with a duplicate.
    const api = new FakeFeedApi([page([article(1), article(2)], 0, 3, 2), new Error("malformed page"), page([article(2), article(3)], 1, 3, 2)])
    const feed = createArticleFeed(api, 2)
    await feed.loadNext(); await feed.loadNext()
    // When: the failed intent is retried.
    expect(feed.state).toMatchObject({ kind: "error", failedPageIndex: 1 }); await feed.retry()
    // Then: page one intent repeats and rows remain unique.
    expect(api.requestedPages).toEqual([0, 1, 1])
    expect(feed.state.items.map(({ id }) => id)).toEqual([1, 2, 3])
  })
})

describe("taxonomy state machine", () => {
  it("represents ready empty and recoverable error outcomes", async () => {
    // Given: typed category and tag adapters that fail once.
    let fails = true
    const api: TaxonomyApi = { async listArticleTypes() { if (fails) throw new Error("offline"); return [{ id: 2, name: "Tech", imageUrl: null, menu: 1 }] }, async listTags() { return [] } }
    const taxonomy = createTaxonomy(api)
    // When: initial load fails and retry succeeds.
    await taxonomy.load(); expect(taxonomy.state.kind).toBe("error"); fails = false; await taxonomy.retry()
    // Then: mixed dictionaries produce ready, while both empty produce empty in a fresh machine.
    expect(taxonomy.state).toMatchObject({ kind: "ready", tags: [] })
    const emptyApi: TaxonomyApi = { async listArticleTypes() { return [] }, async listTags() { return [] } }
    const empty = createTaxonomy(emptyApi); await empty.load(); expect(empty.state.kind).toBe("empty")
  })
})

describe("loaded-feed search and deterministic discovery", () => {
  const loaded = [article(3, 2, 8), article(1, 1, 8), article(2, 1, 12), { ...article(4, 2, 1), title: "Vue state", digest: "Pinia feed", tagIds: [99] }]

  it("searches only loaded title digest category and tag text with a truthful marker", () => {
    // Given: local taxonomy names and loaded articles.
    const names = { categories: new Map([[1, "Backend"], [2, "Frontend"]]), tags: new Map([[99, "Vue"]]) }
    // When: a normalized local query is applied.
    const result = searchLoadedArticles(loaded, "  VUE ", names)
    // Then: result explicitly states its limited loaded-feed scope.
    expect(result).toEqual({ kind: "loaded_results", query: "vue", items: [loaded[3]] })
  })

  it("keeps recommendation and category-group ordering stable", () => {
    // Given / When: recommendations and homepage groups are derived twice.
    const recommendations = selectRecommendations(loaded)
    const groups = groupHomepageArticles(loaded, [{ id: 2, name: "Frontend" }, { id: 1, name: "Backend" }])
    // Then: visits descend with ID tie-break, categories follow provided order and groups cap at six.
    expect(recommendations.map(({ id }) => id)).toEqual([2, 1, 3])
    expect(selectRecommendations(loaded)).toEqual(recommendations)
    expect(groups.map((group) => ({ categoryId: group.categoryId, ids: group.articles.map(({ id }) => id) }))).toEqual([{ categoryId: 2, ids: [3, 4] }, { categoryId: 1, ids: [1, 2] }])
    const capped = groupHomepageArticles(Array.from({ length: 7 }, (_, index) => article(index + 10, 1)), [{ id: 1, name: "Backend" }])
    expect(capped[0]?.articles).toHaveLength(6)
  })

  it("serializes equivalent search filters to one canonical URL", () => {
    // Given: unordered optional filter state.
    const first = { q: " vue ", tagId: 9, articleTypeId: 2 }
    const second = { articleTypeId: 2, q: "vue", tagId: 9 }
    // When / Then: both yield the same fixed parameter order without toggle state.
    expect(buildSearchUrl("/search", first)).toBe("/search?q=vue&articleTypeId=2&tagId=9")
    expect(buildSearchUrl("/search", second)).toBe("/search?q=vue&articleTypeId=2&tagId=9")
  })
})
