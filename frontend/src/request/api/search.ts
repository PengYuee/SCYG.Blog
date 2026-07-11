import type { UnsupportedResult } from "@/types/api"

/** 未来远程搜索请求。 */
export type SearchRequest = { readonly query: string; readonly pageIndex: number; readonly pageSize: number; readonly articleTypeId?: number; readonly tagId?: number }

/** 远程搜索 API 契约。 */
export interface SearchApi { /** 搜索文章；未启用时返回 unsupported。 */ search(request: SearchRequest): Promise<UnsupportedResult> }

/** 生产远程搜索尚未启用，调用方应使用已加载文章回退。 */
export const unsupportedSearchApi: SearchApi = {
  /** 不发起网络请求。 */ async search() { return { kind: "unsupported", feature: "search" } },
}
