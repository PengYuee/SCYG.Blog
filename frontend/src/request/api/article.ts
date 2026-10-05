import type { ArchiveManageArticleData, Article, CreateManageArticleData, DeleteManageArticleData, GetArticleData, GetManageArticleData, ListArticlesData, ListManageArticlesData, PatchManageArticleData, PublishManageArticleData } from "@/request/generated"
import { zArticle, zArticleList } from "@/request/generated/zod.gen"
import type { HttpTransport } from "@/request/transport"
import type { ArticleCreateRequest, ArticleDetail, ArticleListRequest, ArticleUpdateRequest, ArticleVersionTarget } from "@/types/article"
import { ApiParseError, type PageResult, parseBoundary } from "@/types/api"

const ARTICLE_ROUTE: ListArticlesData["url"] = "/api/v1/articles"
const MANAGE_ARTICLE_ROUTE: ListManageArticlesData["url"] = "/api/v1/manage/articles"
type ArticleListQuery = NonNullable<ListArticlesData["query"]>
type ManageArticleListQuery = NonNullable<ListManageArticlesData["query"]>
type ArticleReadListQuery = ArticleListQuery | ManageArticleListQuery

/** 文章 API 适配器契约。 */
export interface ArticleApi {
  readonly list: (request: ArticleListRequest) => Promise<PageResult<ArticleDetail>>
  readonly manageList: (request: ArticleListRequest) => Promise<PageResult<ArticleDetail>>
  readonly detail: (id: GetArticleData["path"]["articleId"]) => Promise<ArticleDetail>
  readonly manageDetail: (id: GetManageArticleData["path"]["articleId"]) => Promise<ArticleDetail>
  readonly create: (request: ArticleCreateRequest) => Promise<ArticleDetail>
  readonly update: (request: ArticleUpdateRequest) => Promise<ArticleDetail>
  readonly deleteManage: (target: ArticleVersionTarget) => Promise<void>
  readonly publish: (target: ArticleVersionTarget) => Promise<ArticleDetail>
  readonly archive: (target: ArticleVersionTarget) => Promise<ArticleDetail>
}

/** 映射文章并校验生成器尚未表达的标签唯一性。 */
function mapArticle(value: Article): ArticleDetail {
  if (new Set(value.tagIds).size !== value.tagIds.length) throw new ApiParseError("article detail", new TypeError("tagIds must contain unique values"))
  return { id: value.id, title: value.title, slug: value.slug, digest: value.digest, markdown: value.content, articleTypeId: value.articleTypeId, articleType: { id: value.articleType.id, name: value.articleType.name, imageUrl: value.articleType.image }, tagIds: value.tagIds, status: value.status, support: value.support, comment: value.comment, visited: value.visited, version: value.version, createdAt: value.createdAt, updatedAt: value.updatedAt }
}

/** 将当前 API 文章映射为领域 Markdown 模型。 */
export function parseArticleDetail(input: unknown): ArticleDetail {
  return mapArticle(parseBoundary(zArticle, input, "article detail"))
}

/** 解析当前 API 文章分页包络。 */
export function parseArticleList(input: unknown): PageResult<ArticleDetail> {
  const value = parseBoundary(zArticleList, input, "article list")
  return { items: value.items.map(mapArticle), pageIndex: value.page.number - 1, pageSize: value.page.size, totalItems: value.page.totalItems, totalPages: value.page.totalPages }
}

/** 文章 API 的类型化适配器。 */
export function createArticleApi(client: HttpTransport): ArticleApi {
  function buildListParams(request: ArticleListRequest): ArticleListQuery {
    const query = request.q?.trim()
    return {
      page: request.pageModel.pageIndex + 1,
      pageSize: request.pageModel.pageSize,
      sort: "-createdAt",
      ...(request.articleTypeId === undefined ? {} : { articleTypeId: request.articleTypeId }),
      ...(request.tagId === undefined ? {} : { tagId: request.tagId }),
      ...(query === undefined || query.length === 0 ? {} : { q: query }),
    }
  }

  async function listFromRoute(route: ListArticlesData["url"] | ListManageArticlesData["url"], params: ArticleReadListQuery): Promise<PageResult<ArticleDetail>> {
    const response = await client.get(route, { params })
    return parseArticleList(response.data)
  }
  return {
    async list(request: ArticleListRequest) { return listFromRoute(ARTICLE_ROUTE, buildListParams(request)) },
    async manageList(request: ArticleListRequest) {
      const params: ManageArticleListQuery = buildListParams(request)
      return listFromRoute(MANAGE_ARTICLE_ROUTE, params)
    },
    async detail(id: GetArticleData["path"]["articleId"]) {
      const path: GetArticleData["path"] = { articleId: id }
      const response = await client.get(`${ARTICLE_ROUTE}/${path.articleId}`)
      return parseArticleDetail(response.data)
    },
    async manageDetail(id: GetManageArticleData["path"]["articleId"]) {
      const path: GetManageArticleData["path"] = { articleId: id }
      const response = await client.get(`${MANAGE_ARTICLE_ROUTE}/${path.articleId}`)
      return parseArticleDetail(response.data)
    },
    async create(request: ArticleCreateRequest): Promise<ArticleDetail> {
      const payload: CreateManageArticleData["body"] = { title: request.title, slug: request.slug, digest: request.digest, content: request.markdown, articleTypeId: request.articleTypeId, tagIds: [...request.tagIds], status: request.status }
      const response = await client.post(MANAGE_ARTICLE_ROUTE, payload)
      return parseArticleDetail(response.data)
    },
    async update(request: ArticleUpdateRequest): Promise<ArticleDetail> {
      const changes: PatchManageArticleData["body"] = {}
      if (request.changes.title !== undefined) changes.title = request.changes.title
      if (request.changes.slug !== undefined) changes.slug = request.changes.slug
      if (request.changes.digest !== undefined) changes.digest = request.changes.digest
      if (request.changes.markdown !== undefined) changes.content = request.changes.markdown
      if (request.changes.articleTypeId !== undefined) changes.articleTypeId = request.changes.articleTypeId
      if (request.changes.tagIds !== undefined) changes.tagIds = [...request.changes.tagIds]
      const path: PatchManageArticleData["path"] = { articleId: request.id }
      const headers: PatchManageArticleData["headers"] = { "If-Match": `"${request.version}"` }
      const response = await client.patch(`${MANAGE_ARTICLE_ROUTE}/${path.articleId}`, changes, { headers })
      return parseArticleDetail(response.data)
    },
    async deleteManage(target: ArticleVersionTarget): Promise<void> {
      const path: DeleteManageArticleData["path"] = { articleId: target.id }
      const headers: DeleteManageArticleData["headers"] = { "If-Match": `"${target.version}"` }
      await client.delete(`${MANAGE_ARTICLE_ROUTE}/${path.articleId}`, { headers })
    },
    async publish(target: ArticleVersionTarget): Promise<ArticleDetail> {
      const path: PublishManageArticleData["path"] = { articleId: target.id }
      const headers: PublishManageArticleData["headers"] = { "If-Match": `"${target.version}"` }
      const response = await client.post(`${MANAGE_ARTICLE_ROUTE}/${path.articleId}/publish`, undefined, { headers })
      return parseArticleDetail(response.data)
    },
    async archive(target: ArticleVersionTarget): Promise<ArticleDetail> {
      const path: ArchiveManageArticleData["path"] = { articleId: target.id }
      const headers: ArchiveManageArticleData["headers"] = { "If-Match": `"${target.version}"` }
      const response = await client.post(`${MANAGE_ARTICLE_ROUTE}/${path.articleId}/archive`, undefined, { headers })
      return parseArticleDetail(response.data)
    },
  }
}
