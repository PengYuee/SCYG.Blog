import type { Page, Request, Route } from "@playwright/test"
import type { PublicArticleType, PublicTag } from "../../../../src/request/generated"

type ApiArticle = {
  readonly id: number
  readonly title: string
  readonly slug: string
  readonly digest: string
  readonly content: string
  readonly articleTypeId: number
  readonly articleType: { readonly id: number; readonly name: string; readonly image: string | null }
  readonly tagIds: readonly number[]
  readonly status: 2
  readonly support: number
  readonly comment: number
  readonly visited: number
  readonly version: number
  readonly createdAt: string
  readonly updatedAt: string | null
}

/** 公共读取 fixture 的可控失败选项。 */
export type ReadFixtureOptions = {
  readonly listStatus?: number
  readonly detailStatus?: number
  readonly detailMarkdown?: string
  readonly categoryImage?: string | null
}

/** 意外写请求记录器，用于阻止验收污染后端。 */
export type MutationRecorder = {
  readonly requests: Request[]
}

const apiPattern = /\/api\/v1\//
const mutationRecorders = new WeakMap<Page, MutationRecorder>()
const page = <T>(items: readonly T[]) => ({ items, page: { number: 1, size: 20, totalItems: items.length, totalPages: items.length === 0 ? 0 : 1 } })

/** 构造覆盖首页、列表与详情的确定性中文文章。 */
const article = (id: number, content = "## 可复现正文\n\n这是用于生产端到端验证的安全中文内容。"):
ApiArticle => ({
  id,
  title: id === 101 ? "在 Edge 中验证中文博客的完整阅读体验" : `确定性文章 ${id}`,
  slug: `t13-article-${id}`,
  digest: "稳定摘要用于验证卡片网格、中文排版与资源边界。",
  content,
  articleTypeId: ((id - 1) % 3) + 1,
  articleType: { id: ((id - 1) % 3) + 1, name: "前端工程", image: null },
  tagIds: [((id - 1) % 2) + 1],
  status: 2,
  support: id,
  comment: id % 4,
  visited: 500 - id,
  version: 1,
  createdAt: "2026-07-12T00:00:00.000Z",
  updatedAt: null,
})

const articles = [101, 102, 103, 104, 105, 106, 107, 108, 109].map((id) => article(id))
const categories = page([
  { id: 1, name: "前端工程", image: null, menu: 1, version: 1, createdAt: "2026-07-12T00:00:00.000Z", updatedAt: null },
  { id: 2, name: "类型契约", image: null, menu: 2, version: 1, createdAt: "2026-07-12T00:00:00.000Z", updatedAt: null },
  { id: 3, name: "阅读设计", image: null, menu: 3, version: 1, createdAt: "2026-07-12T00:00:00.000Z", updatedAt: null },
])
const tags = page([
  { id: 1, name: "Vue 3", version: 1, createdAt: "2026-07-12T00:00:00.000Z", updatedAt: null },
  { id: 2, name: "TypeScript", version: 1, createdAt: "2026-07-12T00:00:00.000Z", updatedAt: null },
])

/** 在导航前拦截公共读取及作者字典读取接口。 */
export async function installReadFixtures(pageInstance: Page, options: ReadFixtureOptions = {}): Promise<void> {
  // 先安装共享写阻断底线；较晚注册的专用 mock 优先，非 GET fallback 交给记录器。
  await installMutationRecorder(pageInstance)
  await pageInstance.route(apiPattern, async (route) => {
    if (route.request().method() !== "GET") {
      await route.fallback()
      return
    }
    const requestUrl = new URL(route.request().url())
    if (requestUrl.pathname === "/api/v1/articles") {
      await route.fulfill({ status: options.listStatus ?? 200, json: page(articles) })
      return
    }
    if (requestUrl.pathname.startsWith("/api/v1/articles/")) {
      const detail = { ...article(101, options.detailMarkdown), articleTypeId: 1, articleType: { id: 1, name: "前端工程", image: null } }
      await route.fulfill({ status: options.detailStatus ?? 200, json: detail })
      return
    }
    if (requestUrl.pathname === "/api/v1/article-types") {
      const items = categories.items.map((item): PublicArticleType => ({
        id: item.id,
        name: item.name,
        image: item.id === 1 ? options.categoryImage ?? item.image : item.image,
        articleCount: articles.filter((entry) => entry.articleTypeId === item.id).length,
      }))
      await route.fulfill({ status: 200, json: page(items) })
      return
    }
    if (requestUrl.pathname === "/api/v1/tags") {
      const items = tags.items.map((item): PublicTag => ({ id: item.id, name: item.name, articleCount: articles.filter((entry) => entry.tagIds.includes(item.id)).length }))
      await route.fulfill({ status: 200, json: page(items) })
      return
    }
    if (requestUrl.pathname === "/api/v1/manage/article-types") {
      const items = categories.items.map((item) => item.id === 1 ? { ...item, image: options.categoryImage ?? item.image } : item)
      await route.fulfill({ status: 200, json: { ...categories, items } })
      return
    }
    if (requestUrl.pathname === "/api/v1/manage/tags") {
      await route.fulfill({ status: 200, json: tags })
      return
    }
    await route.fallback()
  })
}

/** 记录并拒绝任何意外生产写请求，确保测试不会污染后端。 */
export async function installMutationRecorder(pageInstance: Page): Promise<MutationRecorder> {
  const existing = mutationRecorders.get(pageInstance)
  if (existing) return existing
  const requests: Request[] = []
  await pageInstance.route(apiPattern, async (route: Route) => {
    const request = route.request()
    if (request.method() !== "GET") {
      requests.push(request)
      await route.abort("blockedbyclient")
      return
    }
    await route.fallback()
  })
  const recorder = { requests }
  mutationRecorders.set(pageInstance, recorder)
  return recorder
}
