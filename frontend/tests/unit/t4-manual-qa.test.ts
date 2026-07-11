import { writeFile } from "node:fs/promises"
import { describe, expect, it } from "vitest"
import { createArticleFeed, type ArticleFeedApi } from "@/stores/article-feed"
import { buildSearchUrl, searchLoadedArticles } from "@/utils/search"
import type { PageResult } from "@/types/api"
import type { ArticleDetail, ArticleListRequest } from "@/types/article"

/** 单项 T4 QA 结果。 */
type QaEntry = { readonly expected: string; readonly actual: string; readonly passed: boolean }
/** T4 QA 文章。 */
const qaArticle: ArticleDetail = { id: 1, title: "Vue feed", slug: "vue-feed", digest: "Pinia", markdown: "Body", articleTypeId: 2, tagIds: [9], status: 2, support: 0, comment: 0, visited: 4, version: 1, createdAt: "2026-07-11T00:00:00Z", updatedAt: null }
const happyPath = process.env["T4_QA_HAPPY"]
const failurePath = process.env["T4_QA_FAILURE"]

/** 按队列返回分页或错误的 QA 适配器。 */
class QaFeedApi implements ArticleFeedApi {
  /** 请求页序列。 */ readonly pages: number[] = []
  /** 创建 QA 响应队列。 */ constructor(private readonly responses: readonly (PageResult<ArticleDetail> | Error)[]) {}
  /** 获取队列中的下一响应。 */ async list(request: ArticleListRequest): Promise<PageResult<ArticleDetail>> {
    this.pages.push(request.pageModel.pageIndex)
    const response = this.responses[this.pages.length - 1]
    if (response === undefined) throw new Error("QA response missing")
    if (response instanceof Error) throw response
    return response
  }
}

/** 构造 QA 分页。 */
function qaPage(items: readonly ArticleDetail[], pageIndex: number, totalItems: number, totalPages: number): PageResult<ArticleDetail> {
  return { items, pageIndex, pageSize: 1, totalItems, totalPages }
}

/** 创建字符串对比结果。 */
function entry(actual: string, expected: string): QaEntry { return { expected, actual, passed: actual === expected } }

/** 写入具名 QA JSON 并断言聚合结果。 */
async function emit(path: string, scenario: string, entries: Readonly<Record<string, QaEntry>>): Promise<void> {
  const passed = Object.values(entries).every((item) => item.passed)
  await writeFile(path, `${JSON.stringify({ scenario, result: passed ? "PASS" : "FAIL", entries }, null, 2)}\n`, "utf8")
  expect(passed).toBe(true)
}

describe.skipIf(happyPath === undefined || failurePath === undefined)("T4 named state-transition QA", () => {
  it("emits happy and recoverable-failure JSON with expected actual and passed", async () => {
    // Given: deterministic success and malformed-then-recovery adapters.
    const successApi = new QaFeedApi([qaPage([qaArticle], 0, 1, 1)])
    const success = createArticleFeed(successApi, 1)
    const recoveryApi = new QaFeedApi([new Error("malformed page"), qaPage([qaArticle], 0, 1, 1)])
    const recovery = createArticleFeed(recoveryApi, 1)
    // When: production state-machine and local-search exports execute.
    await success.loadNext(); await recovery.loadNext()
    const failedKind = recovery.state.kind
    await recovery.retry()
    const search = searchLoadedArticles(success.state.items, "vue", { categories: new Map([[2, "Frontend"]]), tags: new Map([[9, "Vue"]]) })
    // Then: both artifacts name every expected and actual transition.
    await emit(happyPath ?? "", "t4-feed-happy", {
      feedState: entry(success.state.kind, "ready"),
      requestPages: entry(JSON.stringify(successApi.pages), "[0]"),
      loadedSearchMarker: entry(search.kind, "loaded_results"),
      canonicalUrl: entry(buildSearchUrl("/search", { q: " Vue ", articleTypeId: 2, tagId: 9 }), "/search?q=vue&articleTypeId=2&tagId=9"),
    })
    await emit(failurePath ?? "", "t4-feed-failure", {
      malformedState: entry(failedKind, "error"),
      recoveredState: entry(recovery.state.kind, "ready"),
      retryIntent: entry(JSON.stringify(recoveryApi.pages), "[0,0]"),
      dedupedIds: entry(JSON.stringify(recovery.state.items.map(({ id }) => id)), "[1]"),
    })
  })
})
