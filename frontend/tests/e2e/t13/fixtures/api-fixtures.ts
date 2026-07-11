import type { Page, Request, Route } from "@playwright/test"

/** T13 可复现文章接口记录。 */
type ApiArticle = {
  readonly id: number
  readonly title: string
  readonly slug: string
  readonly digest: string
  readonly content: string
  readonly article_type_id: number
  readonly tag_ids: readonly number[]
  readonly status: 1
  readonly support: number
  readonly comment: number
  readonly visited: number
  readonly version: number
  readonly created_at: string
  readonly updated_at: string | null
}

/** 公共读取 fixture 的可控失败选项。 */
export type ReadFixtureOptions = {
  readonly listStatus?: number
  readonly detailStatus?: number
  readonly detailMarkdown?: string
  readonly categoryImage?: string | null
}

/** 被生产构建禁止的写请求记录器。 */
export type MutationRecorder = {
  readonly requests: Request[]
}

const apiPattern = /\/(?:api\/)?(?:Article|ArticleType|Tag)\//
const mutationPattern = /\/(?:Create|Update|UpLoad|Delete)/i
const page = (items: readonly unknown[]) => ({ items, page: { number: 1, size: 20, total_items: items.length, total_pages: items.length === 0 ? 0 : 1 } })

/** 构造覆盖首页、列表与详情的确定性中文文章。 */
const article = (id: number, content = "## 可复现正文\n\n这是用于生产端到端验证的安全中文内容。"):
ApiArticle => ({
  id,
  title: id === 101 ? "在 Edge 中验证中文博客的完整阅读体验" : `确定性文章 ${id}`,
  slug: `t13-article-${id}`,
  digest: "稳定摘要用于验证卡片网格、中文排版与资源边界。",
  content,
  article_type_id: ((id - 1) % 3) + 1,
  tag_ids: [((id - 1) % 2) + 1],
  status: 1,
  support: id,
  comment: id % 4,
  visited: 500 - id,
  version: 1,
  created_at: "2026-07-12T00:00:00.000Z",
  updated_at: null,
})

const articles = [101, 102, 103, 104, 105, 106, 107, 108, 109].map((id) => article(id))
const categories = page([
  { id: 1, name: "前端工程", image: "/images/hero-starry.jpg", meun: 1, version: 1, created_at: "2026-07-12T00:00:00.000Z", updated_at: null },
  { id: 2, name: "类型契约", image: null, meun: 2, version: 1, created_at: "2026-07-12T00:00:00.000Z", updated_at: null },
  { id: 3, name: "阅读设计", image: null, meun: 3, version: 1, created_at: "2026-07-12T00:00:00.000Z", updated_at: null },
])
const tags = page([
  { id: 1, name: "Vue 3", version: 1, created_at: "2026-07-12T00:00:00.000Z", updated_at: null },
  { id: 2, name: "TypeScript", version: 1, created_at: "2026-07-12T00:00:00.000Z", updated_at: null },
])

/** 在导航前拦截全部真实公共读取接口。 */
export async function installReadFixtures(pageInstance: Page, options: ReadFixtureOptions = {}): Promise<void> {
  await pageInstance.route(apiPattern, async (route) => {
    const requestUrl = new URL(route.request().url())
    if (requestUrl.pathname.endsWith("/Article/GetArticleList")) {
      await route.fulfill({ status: options.listStatus ?? 200, json: page(articles) })
      return
    }
    if (requestUrl.pathname.endsWith("/Article/GetArticle")) {
      const detail = { ...article(101, options.detailMarkdown), article_type_id: 1 }
      await route.fulfill({ status: options.detailStatus ?? 200, json: detail })
      return
    }
    if (requestUrl.pathname.endsWith("/ArticleType/GetArticleTypeDic")) {
      const items = categories.items.map((item) => item.id === 1 ? { ...item, image: options.categoryImage ?? item.image } : item)
      await route.fulfill({ status: 200, json: { ...categories, items } })
      return
    }
    if (requestUrl.pathname.endsWith("/Tag/GetTagDic")) {
      await route.fulfill({ status: 200, json: tags })
      return
    }
    await route.fallback()
  })
}

/** 记录并拒绝任何意外生产写请求，确保测试不会污染后端。 */
export async function installMutationRecorder(pageInstance: Page): Promise<MutationRecorder> {
  const requests: Request[] = []
  await pageInstance.route(apiPattern, async (route: Route) => {
    const request = route.request()
    if (request.method() !== "GET" || mutationPattern.test(new URL(request.url()).pathname)) {
      requests.push(request)
      await route.abort("blockedbyclient")
      return
    }
    await route.fallback()
  })
  return { requests }
}
