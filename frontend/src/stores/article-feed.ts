import type { PageResult } from "@/types/api"
import type { ArticleDetail, ArticleListRequest } from "@/types/article"

/** 文章分页依赖契约，允许生产适配器与测试假实现互换。 */
export interface ArticleFeedApi {
  /** 获取指定筛选条件的一页文章。 */
  list(request: ArticleListRequest): Promise<PageResult<ArticleDetail>>
}

/** 文章流筛选条件。 */
export type ArticleFeedFilters = {
  /** 分类筛选标识。 */ readonly articleTypeId?: number
  /** 标签筛选标识。 */ readonly tagId?: number
}

/** 所有文章流状态共享的稳定数据。 */
type FeedSnapshot = {
  /** 按首次出现顺序保存的去重文章。 */ readonly items: readonly ArticleDetail[]
  /** 当前筛选条件。 */ readonly filters: ArticleFeedFilters
}

/** 文章流状态；loading 本身即请求锁，error 保留失败意图。 */
export type ArticleFeedState =
  | (FeedSnapshot & { readonly kind: "idle" })
  | (FeedSnapshot & { readonly kind: "loading"; readonly requestedPageIndex: number })
  | (FeedSnapshot & { readonly kind: "ready"; readonly pageIndex: number; readonly endReached: boolean })
  | (FeedSnapshot & { readonly kind: "empty"; readonly pageIndex: number; readonly endReached: true })
  | (FeedSnapshot & { readonly kind: "error"; readonly pageIndex: number; readonly failedPageIndex: number; readonly message: string })

/** 文章流状态与操作。 */
export interface ArticleFeed {
  /** 当前判别联合状态。 */ readonly state: ArticleFeedState
  /** 加载派生出的下一页；请求中或已结束时忽略。 */ loadNext(): Promise<void>
  /** 重试最后失败的页意图。 */ retry(): Promise<void>
  /** 原子替换筛选条件并清空分页状态。 */ resetFilters(filters: ArticleFeedFilters): void
  /** 按规范路由键记录滚动位置。 */ rememberScroll(routeKey: string, top: number): void
  /** 恢复路由滚动位置，未记录时返回顶部。 */ restoreScroll(routeKey: string): number
}

/** 创建 UI 无关、依赖可注入的文章流状态机。 */
export function createArticleFeed(api: ArticleFeedApi, pageSize: number): ArticleFeed {
  /** 内部状态由状态机转换有意维护可变引用。 */
  let state: ArticleFeedState = { kind: "idle", items: [], filters: {} }
  /** 请求世代在筛选重置时递增，使旧响应无法覆盖新筛选状态。 */
  let requestGeneration = 0
  /** 路由滚动表由显式导航生命周期读写，不监听 window。 */
  const scrollPositions = new Map<string, number>()

  /** 执行指定页请求并完成唯一合法的状态转换。 */
  const loadPage = async (pageIndex: number): Promise<void> => {
    if (state.kind === "loading") return
    const generation = requestGeneration
    const snapshot = state
    state = { kind: "loading", items: snapshot.items, filters: snapshot.filters, requestedPageIndex: pageIndex }
    try {
      const result = await api.list({ ...snapshot.filters, pageModel: { pageIndex, pageSize } })
      if (generation !== requestGeneration) return
      const seenIds = new Set(snapshot.items.map((item) => item.id))
      const uniquePageItems = result.items.filter((item) => {
        if (seenIds.has(item.id)) return false
        seenIds.add(item.id)
        return true
      })
      const items = [...snapshot.items, ...uniquePageItems]
      const endReached = items.length >= result.totalItems || pageIndex + 1 >= result.totalPages || result.items.length < pageSize
      // 首次空页独立建模，非空分页统一进入 ready 并携带终止信息。
      state = items.length === 0
        ? { kind: "empty", items, filters: snapshot.filters, pageIndex, endReached: true }
        : { kind: "ready", items, filters: snapshot.filters, pageIndex, endReached }
    } catch (error) {
      if (generation !== requestGeneration) return
      state = {
        kind: "error",
        items: snapshot.items,
        filters: snapshot.filters,
        pageIndex: pageIndex - 1,
        failedPageIndex: pageIndex,
        message: error instanceof Error ? error.message : "Article page request failed",
      }
    }
  }

  return {
    /** 读取最新状态而不暴露可写引用。 */
    get state() { return state },
    /** 从当前已完成页派生下一页。 */
    async loadNext() {
      if (state.kind === "loading" || state.kind === "empty" || (state.kind === "ready" && state.endReached)) return
      const pageIndex = state.kind === "ready" ? state.pageIndex + 1 : state.kind === "error" ? state.failedPageIndex : 0
      await loadPage(pageIndex)
    },
    /** 仅 error 状态拥有可重试意图。 */
    async retry() { if (state.kind === "error") await loadPage(state.failedPageIndex) },
    /** 清除旧分页并使所有在途响应失效。 */
    resetFilters(filters) { requestGeneration += 1; state = { kind: "idle", items: [], filters } },
    /** 保存非负滚动偏移。 */
    rememberScroll(routeKey, top) { scrollPositions.set(routeKey, Math.max(0, top)) },
    /** 返回确定的路由滚动偏移。 */
    restoreScroll(routeKey) { return scrollPositions.get(routeKey) ?? 0 },
  }
}
