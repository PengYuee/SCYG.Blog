import { z } from "zod"
import type { HttpTransport } from "@/request/transport"
import type { ArticleCreateRequest, ArticleDetail, ArticleListRequest, ArticleUpdateRequest, ArticleVersionTarget } from "@/types/article"
import type { PageResult } from "@/types/api"
import { parseBoundary } from "@/types/api"
import { articleSchema, pageSchema } from "./schemas"

const listSchema = z.strictObject({ items: z.array(articleSchema), page: pageSchema })
const ARTICLE_ROUTE = "/api/v1/articles"
const MANAGE_ARTICLE_ROUTE = "/api/v1/manage/articles"

/** 将当前 API 文章映射为领域 Markdown 模型。 */
export function parseArticleDetail(input: unknown): ArticleDetail {
  const value = parseBoundary(articleSchema, input, "article detail")
  return { id: value.id, title: value.title, slug: value.slug, digest: value.digest, markdown: value.content, articleTypeId: value.article_type_id, tagIds: value.tag_ids, status: value.status, support: value.support, comment: value.comment, visited: value.visited, version: value.version, createdAt: value.created_at, updatedAt: value.updated_at }
}

/** 解析当前 API 文章分页包络。 */
export function parseArticleList(input: unknown): PageResult<ArticleDetail> {
  const value = parseBoundary(listSchema, input, "article list")
  return { items: value.items.map(parseArticleDetail), pageIndex: value.page.number - 1, pageSize: value.page.size, totalItems: value.page.total_items, totalPages: value.page.total_pages }
}

/** 文章 API 的类型化适配器。 */
export function createArticleApi(client: HttpTransport) {
  /** 通过指定路径读取文章分页，并统一转换查询参数。 */
  async function listFromRoute(route: string, request: ArticleListRequest): Promise<PageResult<ArticleDetail>> {
    const query = request.q?.trim()
    const response = await client.get(route, { params: {
      page: request.pageModel.pageIndex + 1,
      page_size: request.pageModel.pageSize,
      ...(request.articleTypeId === undefined ? {} : { article_type_id: request.articleTypeId }),
      ...(request.tagId === undefined ? {} : { tag_id: request.tagId }),
      ...(query === undefined || query.length === 0 ? {} : { q: query }),
    } })
    return parseArticleList(response.data)
  }

  return {
    /** 获取公开文章列表。 */
    async list(request: ArticleListRequest) { return listFromRoute(ARTICLE_ROUTE, request) },
    /** 获取管理端文章列表。 */
    async manageList(request: ArticleListRequest) { return listFromRoute(MANAGE_ARTICLE_ROUTE, request) },
    /** 获取公开文章详情。 */
    async detail(id: number) {
      const response = await client.get(`${ARTICLE_ROUTE}/${id}`)
      return parseArticleDetail(response.data)
    },
    /** 获取管理端文章详情，可读取草稿和已归档文章。 */
    async manageDetail(id: number) {
      const response = await client.get(`${MANAGE_ARTICLE_ROUTE}/${id}`)
      return parseArticleDetail(response.data)
    },
    /** 创建文章并解析服务端返回的裸资源。 */
    async create(request: ArticleCreateRequest): Promise<ArticleDetail> {
      const response = await client.post(MANAGE_ARTICLE_ROUTE, {
        title: request.title,
        slug: request.slug,
        digest: request.digest,
        content: request.markdown,
        article_type_id: request.articleTypeId,
        tag_ids: request.tagIds,
        status: request.status,
      })
      return parseArticleDetail(response.data)
    },
    /** 使用强实体标签局部更新文章。 */
    async update(request: ArticleUpdateRequest): Promise<ArticleDetail> {
      const changes: Record<string, unknown> = {}
      if (request.changes.title !== undefined) changes["title"] = request.changes.title
      if (request.changes.slug !== undefined) changes["slug"] = request.changes.slug
      if (request.changes.digest !== undefined) changes["digest"] = request.changes.digest
      if (request.changes.markdown !== undefined) changes["content"] = request.changes.markdown
      if (request.changes.articleTypeId !== undefined) changes["article_type_id"] = request.changes.articleTypeId
      if (request.changes.tagIds !== undefined) changes["tag_ids"] = request.changes.tagIds
      const response = await client.patch(`${MANAGE_ARTICLE_ROUTE}/${request.id}`, changes, { headers: { "If-Match": `"${request.version}"` } })
      return parseArticleDetail(response.data)
    },
    /** 软删除管理端文章。 */
    async deleteManage(target: ArticleVersionTarget): Promise<void> {
      await client.delete(`${MANAGE_ARTICLE_ROUTE}/${target.id}`, { headers: { "If-Match": `"${target.version}"` } })
    },
    /** 发布管理端文章并返回新资源。 */
    async publish(target: ArticleVersionTarget): Promise<ArticleDetail> {
      const response = await client.post(`${MANAGE_ARTICLE_ROUTE}/${target.id}/publish`, undefined, { headers: { "If-Match": `"${target.version}"` } })
      return parseArticleDetail(response.data)
    },
    /** 归档管理端文章并返回新资源。 */
    async archive(target: ArticleVersionTarget): Promise<ArticleDetail> {
      const response = await client.post(`${MANAGE_ARTICLE_ROUTE}/${target.id}/archive`, undefined, { headers: { "If-Match": `"${target.version}"` } })
      return parseArticleDetail(response.data)
    },
  }
}
